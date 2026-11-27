package cpa

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultOrigin  = "https://gateway.qwenwork.cn"
	oauthClientID  = "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb"
	oauthRedirect  = "qwenwork-cn://"
	clientVersion  = "2.1.0"
	releaseVersion = "2.1.0-26111906"
	clientBuild    = "26111906"
)

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
