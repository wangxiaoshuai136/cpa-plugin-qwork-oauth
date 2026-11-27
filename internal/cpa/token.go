//go:build windows

package cpa

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	defaultOrigin  = "https://gateway.qwenwork.cn"
	oauthClientID  = "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb"
	oauthRedirect  = "qwenwork-cn://"
	clientVersion  = "2.1.0"
	releaseVersion = "2.1.0-26111906"
	clientBuild    = "26111906"
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

// credentialFile is the on-disk shape shared with the host auth store.
type credentialFile struct {
	Type             string `json:"type"`
	Provider         string `json:"provider"`
	OAuthProvider    string `json:"oauth_provider"`
	AuthKind         string `json:"auth_kind"`
	AccountID        string `json:"account_id"`
	Disabled         bool   `json:"disabled"`
	UpdatedAt        string `json:"updated_at"`
	Token            string `json:"token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresAt        string `json:"expires_at"`
	RefreshExpiresAt string `json:"refresh_token_expires_at"`
	MachineID        string `json:"machine_id"`
	UID              string `json:"uid"`
	Label            string `json:"label"`
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	KeySource        string `json:"key_source"`
	Note             string `json:"note"`
	AuthDir          string `json:"auth_dir"`
	Origin           string `json:"origin"`
}

type credential struct {
	token        string
	refreshToken string
	authDir      string
	origin       string
	machineID    string
	oauth        bool
}

func credentialFromStorage(raw []byte, cfg PluginConfig) (*credential, error) {
	var f credentialFile
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &f)
	}
	cred := &credential{
		token:        strings.TrimSpace(f.Token),
		refreshToken: strings.TrimSpace(f.RefreshToken),
		authDir:      strings.TrimSpace(f.AuthDir),
		origin:       strings.TrimRight(strings.TrimSpace(f.Origin), "/"),
		machineID:    strings.TrimSpace(f.MachineID),
		oauth:        f.AuthKind == "oauth",
	}
	if cred.authDir == "" {
		cred.authDir = cfg.AuthDir
	}
	if cred.origin == "" {
		cred.origin = defaultOrigin
	}
	if cred.token == "" {
		doc, err := readAuth(cred.authDir)
		if err != nil {
			return nil, err
		}
		cred.token = doc.Token
		cred.refreshToken = doc.RefreshToken
		if doc.Endpoints.APIOrigin != "" {
			cred.origin = strings.TrimRight(doc.Endpoints.APIOrigin, "/")
		}
	}
	return cred, nil
}

func (c *credential) key() string {
	sum := sha1.Sum([]byte(c.token))
	return hex.EncodeToString(sum[:8])
}

type sessionTicket struct {
	AccessToken string
	BaseURL     string
	ExpiresAt   time.Time
}

var (
	sessMu    sync.Mutex
	sessCache = map[string]*sessionTicket{}
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func ensureSessionFor(cred *credential) (*sessionTicket, error) {
	key := cred.key()
	sessMu.Lock()
	defer sessMu.Unlock()
	if t := sessCache[key]; t != nil && time.Now().Before(t.ExpiresAt.Add(-60*time.Second)) {
		return t, nil
	}
	return mintLocked(cred, key)
}

func mintLocked(cred *credential, key string) (*sessionTicket, error) {
	req, err := http.NewRequest(http.MethodPost, cred.origin+"/api/desktop-sessions", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+cred.token)
	req.Header.Set("content-type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("desktop-sessions HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		BaseURL     string `json:"base_url"`
		ExpiresAtMS int64  `json:"expires_at_ms"`
		TTLMS       int64  `json:"ttl_ms"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse desktop-sessions: %w", err)
	}
	if out.AccessToken == "" || out.BaseURL == "" {
		return nil, errors.New("desktop-sessions returned empty token/base_url")
	}
	exp := time.UnixMilli(out.ExpiresAtMS)
	if out.ExpiresAtMS == 0 {
		exp = time.Now().Add(time.Duration(out.TTLMS) * time.Millisecond)
	}
	t := &sessionTicket{AccessToken: out.AccessToken, BaseURL: strings.TrimRight(out.BaseURL, "/"), ExpiresAt: exp}
	sessCache[key] = t
	return t, nil
}

func invalidateSession(cred *credential) {
	sessMu.Lock()
	delete(sessCache, cred.key())
	sessMu.Unlock()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func clientHeaders() map[string]string {
	return map[string]string{
		"X-QwenWork-Version":         clientVersion,
		"X-QwenWork-Release-Version": releaseVersion,
		"X-QwenWork-Build":           clientBuild,
		"X-QwenWork-Platform":        "win",
		"X-QwenWork-Arch":            "x64",
	}
}


