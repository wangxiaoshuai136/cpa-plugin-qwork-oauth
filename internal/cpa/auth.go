package cpa

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginabi"
	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"
)

const deviceFlowTTL = 5 * time.Minute

type loginState struct {
	verifier  string
	challenge string
	machineID string
	createdAt time.Time
}

var (
	loginMu sync.Mutex
	logins  = map[string]*loginState{}
)

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func newPKCE() (verifier, challenge string) {
	var b [48]byte
	_, _ = rand.Read(b[:])
	verifier = base64.RawURLEncoding.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

func sweepLogins() {
	loginMu.Lock()
	defer loginMu.Unlock()
	now := time.Now()
	for k, v := range logins {
		if now.Sub(v.createdAt) > deviceFlowTTL {
			delete(logins, k)
		}
	}
}

func (p *Plugin) handleAuthLoginStart(raw []byte) (any, error) {
	sweepLogins()
	verifier, challenge := newPKCE()
	nonce := newUUID()
	machineID := newUUID()

	loginMu.Lock()
	logins[nonce] = &loginState{verifier: verifier, challenge: challenge, machineID: machineID, createdAt: time.Now()}
	loginMu.Unlock()

	q := url.Values{}
	q.Set("challenge", challenge)
	q.Set("challenge_method", "S256")
	q.Set("nonce", nonce)
	q.Set("machine_id", machineID)
	q.Set("client_id", oauthClientID)
	q.Set("redirect_uri", oauthRedirect)

	deviceURL := defaultOrigin + "/device/selectAccounts?" + q.Encode()
	var extra struct {
		Browser string `json:"browser"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &extra)
	}
	pref := strings.ToLower(strings.TrimSpace(extra.Browser))
	if pref == "" {
		pref = "default"
	}
	openURL := deviceURL
	if pref != "none" && openIsolatedBrowser(pref, deviceURL) {
		openURL = strings.TrimRight(p.Config().PublicBaseURL, "/") + "/v0/resource/plugins/qwenwork/login"
	}
	return pluginapi.AuthLoginStartResponse{
		Provider:  ProviderID,
		URL:       openURL,
		State:     nonce,
		ExpiresAt: time.Now().Add(deviceFlowTTL),
		Metadata:  map[string]any{"machine_id": machineID},
	}, nil
}

func (p *Plugin) handleAuthLoginPoll(raw []byte) (any, error) {
	var req pluginapi.AuthLoginPollRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode auth.login.poll: %w", err)
	}
	loginMu.Lock()
	st := logins[req.State]
	loginMu.Unlock()
	if st == nil {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录已失效，请重新发起"}, nil
	}
	if time.Since(st.createdAt) > deviceFlowTTL {
		loginMu.Lock()
		delete(logins, req.State)
		loginMu.Unlock()
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录超时（5 分钟），请重新发起"}, nil
	}

	q := url.Values{}
	q.Set("nonce", req.State)
	q.Set("verifier", st.verifier)
	q.Set("challenge_method", "S256")
	httpReq, err := http.NewRequest(http.MethodGet, defaultOrigin+"/api/v1/deviceToken/poll?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("accept", "application/json")
	httpReq.Header.Set("x-request-id", newUUID())
	httpReq.Header.Set("user-agent", "QwenWork/"+clientVersion)
	for k, v := range clientHeaders() {
		httpReq.Header.Set(k, v)
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: "网络错误，继续等待: " + err.Error()}, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: "等待浏览器完成登录"}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: fmt.Sprintf("取票失败 HTTP %d: %s", resp.StatusCode, truncate(string(body), 160))}, nil
	}
	var out struct {
		Token            string `json:"token"`
		DeviceToken      string `json:"device_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresAt        string `json:"expires_at"`
		RefreshExpiresAt string `json:"refresh_token_expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "解析取票响应失败: " + err.Error()}, nil
	}
	token := out.Token
	if token == "" {
		token = out.DeviceToken
	}
	if token == "" {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: "等待浏览器完成登录"}, nil
	}
	if out.RefreshToken == "" {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "取票响应缺少 refresh_token"}, nil
	}
	uid, label := fetchUserIdentity(token)
	suffix := tokenSuffix(token)
	if uid != "" {
		suffix = shortHash(uid)
	}
	accountID := "qwenwork-" + suffix
	displayLabel := firstNonEmpty(label, "千问办公")
	cred := credentialFile{
		Type:             ProviderID,
		Provider:         ProviderID,
		OAuthProvider:    ProviderID,
		AuthKind:         "oauth",
		AccountID:        accountID,
		UpdatedAt:        time.Now().Format(time.RFC3339),
		Token:            token,
		RefreshToken:     out.RefreshToken,
		ExpiresAt:        out.ExpiresAt,
		RefreshExpiresAt: out.RefreshExpiresAt,
		MachineID:        st.machineID,
		UID:              uid,
		Label:            displayLabel,
		Name:             "千问办公 OAuth（" + displayLabel + "）",
		DisplayName:      displayLabel,
		KeySource:        "oauth",
		Origin:           defaultOrigin,
	}
	rawStorage, _ := json.Marshal(cred)
	auth := pluginapi.AuthData{
		Provider:         ProviderID,
		ID:               accountID,
		FileName:         accountID + ".json",
		Label:            displayLabel,
		StorageJSON:      rawStorage,
		Metadata:         map[string]any{"uid": uid, "machine_id": st.machineID},
		Attributes:       map[string]string{"auth_provider": ProviderID},
		NextRefreshAfter: nextRefresh(out.ExpiresAt),
	}
	loginMu.Lock()
	delete(logins, req.State)
	loginMu.Unlock()
	return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusSuccess, Message: "登录成功", Auth: auth}, nil
}

func (p *Plugin) handleAuthParse(raw []byte) (any, error) {
	var req pluginapi.AuthParseRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode auth.parse: %w", err)
	}
	if len(req.RawJSON) == 0 {
		return pluginapi.AuthParseResponse{Handled: false}, nil
	}
	var doc credentialFile
	if err := json.Unmarshal(req.RawJSON, &doc); err != nil {
		return pluginapi.AuthParseResponse{Handled: false}, nil
	}
	fileName := req.FileName
	if fileName == "" {
		fileName = "qwenwork-auth.json"
	}
	if doc.Type != ProviderID && doc.Provider != ProviderID && !strings.HasPrefix(fileName, ProviderID+"-") {
		return pluginapi.AuthParseResponse{Handled: false}, nil
	}
	id := doc.AccountID
	if id == "" {
		if doc.UID != "" {
			id = "qwenwork-" + shortHash(doc.UID)
		} else if doc.Token != "" {
			id = "qwenwork-" + tokenSuffix(doc.Token)
		} else if doc.AuthDir != "" {
			id = "qwenwork-" + shortHash(strings.ToLower(doc.AuthDir))
		} else {
			id = "qwenwork-local"
		}
	}
	label := firstNonEmpty(doc.DisplayName, doc.Label, "千问办公")
	if fileName == "" || !strings.HasSuffix(fileName, ".json") {
		fileName = id + ".json"
	}
	auth := pluginapi.AuthData{
		Provider:         ProviderID,
		ID:               id,
		FileName:         fileName,
		Label:            label,
		Prefix:           "",
		StorageJSON:      req.RawJSON,
		Metadata:         map[string]any{"uid": doc.UID, "auth_dir": doc.AuthDir, "auth_kind": doc.AuthKind},
		Attributes:       map[string]string{"auth_provider": ProviderID},
		NextRefreshAfter: nextRefresh(doc.ExpiresAt),
	}
	return pluginapi.AuthParseResponse{Handled: true, Auth: auth}, nil
}

func (p *Plugin) handleAuthRefresh(raw []byte) (any, error) {
	var req pluginapi.AuthRefreshRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode auth.refresh: %w", err)
	}
	var doc credentialFile
	_ = json.Unmarshal(req.StorageJSON, &doc)
	if doc.AuthKind != "oauth" || doc.RefreshToken == "" {
		doc.UpdatedAt = time.Now().Format(time.RFC3339)
		rawStorage, _ := json.Marshal(doc)
		auth := pluginapi.AuthData{Provider: ProviderID, ID: req.AuthID, Label: firstNonEmpty(doc.Label, "千问办公"), StorageJSON: rawStorage, Attributes: map[string]string{"auth_provider": ProviderID}, NextRefreshAfter: time.Now().Add(24 * time.Hour)}
		return pluginapi.AuthRefreshResponse{Auth: auth, NextRefreshAfter: time.Now().Add(24 * time.Hour)}, nil
	}

	origin := strings.TrimRight(firstNonEmpty(doc.Origin, defaultOrigin), "/")
	body, _ := json.Marshal(map[string]any{
		"refresh_token": doc.RefreshToken,
		"client_id":     oauthClientID,
		"redirect_uri":  oauthRedirect,
	})
	httpReq, err := http.NewRequest(http.MethodPost, origin+"/api/v1/deviceToken/refresh", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, pluginabi.NewError("refresh_failed", fmt.Sprintf("刷新失败 HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200)), http.StatusBadGateway)
	}
	var out struct {
		Token            string `json:"token"`
		DeviceToken      string `json:"device_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresAt        string `json:"expires_at"`
		RefreshExpiresAt string `json:"refresh_token_expires_at"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}
	token := firstNonEmpty(out.Token, out.DeviceToken)
	if token == "" {
		return nil, pluginabi.NewError("refresh_failed", "刷新响应缺少 token", http.StatusBadGateway)
	}
	if out.RefreshToken != "" {
		doc.RefreshToken = out.RefreshToken
	}
	doc.Token = token
	doc.ExpiresAt = out.ExpiresAt
	if out.RefreshExpiresAt != "" {
		doc.RefreshExpiresAt = out.RefreshExpiresAt
	}
	doc.UpdatedAt = time.Now().Format(time.RFC3339)
	rawStorage, _ := json.Marshal(doc)
	next := nextRefresh(doc.ExpiresAt)
	auth := pluginapi.AuthData{Provider: ProviderID, ID: req.AuthID, Label: firstNonEmpty(doc.Label, "千问办公"), StorageJSON: rawStorage, Attributes: map[string]string{"auth_provider": ProviderID}, NextRefreshAfter: next}
	return pluginapi.AuthRefreshResponse{Auth: auth, NextRefreshAfter: next}, nil
}

func unsupportedLogin() error {
	return pluginabi.NewError("unsupported", "不支持的登录方式", http.StatusBadRequest)
}

func fetchUserIdentity(token string) (uid, label string) {
	req, err := http.NewRequest(http.MethodGet, defaultOrigin+"/api/v1/adapter/user/account-context?include=user,plan,quota", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Data struct {
			User struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", ""
	}
	return out.Data.User.ID, firstNonEmpty(out.Data.User.Name, out.Data.User.Username)
}

func nextRefresh(expiresAt string) time.Time {
	if t, err := time.Parse(time.RFC3339, expiresAt); err == nil {
		if next := t.Add(-10 * time.Minute); next.After(time.Now()) {
			return next
		}
	}
	return time.Now().Add(50 * time.Minute)
}

func tokenSuffix(token string) string {
	sum := sha1.Sum([]byte(token))
	return hex.EncodeToString(sum[:4])
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:4])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}



