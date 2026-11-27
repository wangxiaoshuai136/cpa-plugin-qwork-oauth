//go:build windows

package cpa

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	size uint32
	data *byte
}

var (
	modCrypt32    = syscall.NewLazyDLL("crypt32.dll")
	modKernel32   = syscall.NewLazyDLL("kernel32.dll")
	procUnprotect = modCrypt32.NewProc("CryptUnprotectData")
	procLocalFree = modKernel32.NewProc("LocalFree")
)

func dpapiUnprotect(in []byte) ([]byte, error) {
	if len(in) == 0 {
		return nil, errors.New("empty dpapi blob")
	}
	var din, dout dataBlob
	din.size = uint32(len(in))
	din.data = &in[0]
	r, _, callErr := procUnprotect.Call(
		uintptr(unsafe.Pointer(&din)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&dout)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData failed: %v", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(dout.data)))
	out := make([]byte, dout.size)
	copy(out, unsafe.Slice(dout.data, dout.size))
	return out, nil
}

type authDoc struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
	Endpoints    struct {
		APIOrigin string `json:"apiOrigin"`
	} `json:"endpoints"`
}

func readAuth(dir string) (*authDoc, error) {
	lsRaw, err := os.ReadFile(filepath.Join(dir, "Local State"))
	if err != nil {
		return nil, fmt.Errorf("read Local State: %w", err)
	}
	var ls struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(lsRaw, &ls); err != nil {
		return nil, fmt.Errorf("parse Local State: %w", err)
	}
	ek, err := base64.StdEncoding.DecodeString(ls.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted_key: %w", err)
	}
	if len(ek) < 6 || string(ek[:5]) != "DPAPI" {
		return nil, errors.New("encrypted_key is not DPAPI format")
	}
	master, err := dpapiUnprotect(ek[5:])
	if err != nil {
		return nil, err
	}
	blob, err := os.ReadFile(filepath.Join(dir, "auth-v2.dat"))
	if err != nil {
		return nil, fmt.Errorf("read auth-v2.dat: %w", err)
	}
	if len(blob) < 31 || string(blob[:3]) != "v10" {
		return nil, errors.New("auth-v2.dat is not v10 format")
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, blob[3:15], blob[15:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt auth-v2.dat: %w", err)
	}
	var doc authDoc
	if err := json.Unmarshal(plain, &doc); err != nil {
		return nil, fmt.Errorf("parse auth-v2.dat: %w", err)
	}
	if strings.TrimSpace(doc.Token) == "" {
		return nil, errors.New("auth-v2.dat has no token")
	}
	if doc.Endpoints.APIOrigin == "" {
		doc.Endpoints.APIOrigin = defaultOrigin
	}
	return &doc, nil
}
