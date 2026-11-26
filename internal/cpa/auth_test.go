package cpa

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoginStartBuildsDeviceURL(t *testing.T) {
	p := NewPlugin(nil)
	raw, err := p.handleAuthLoginStart(nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	b, _ := json.Marshal(raw)
	var out struct {
		URL   string `json:"URL"`
		State string `json:"State"`
	}
	_ = json.Unmarshal(b, &out)
	t.Logf("url=%s", out.URL)
	for _, part := range []string{"challenge=", "challenge_method=S256", "client_id=" + oauthClientID, "redirect_uri=", "machine_id="} {
		if !strings.Contains(out.URL, part) {
			t.Fatalf("url missing %s", part)
		}
	}
	if !strings.HasPrefix(out.URL, defaultOrigin+"/device/selectAccounts?") {
		t.Fatalf("bad url prefix: %s", out.URL)
	}
	pr, err := p.handleAuthLoginPoll([]byte(`{"State":"nope"}`))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	pb, _ := json.Marshal(pr)
	if !strings.Contains(string(pb), "error") {
		t.Fatalf("expected error status, got %s", pb)
	}
}

func TestParseLocalAuth(t *testing.T) {
	p := NewPlugin(nil)
	raw := []byte(`{"type":"qwenwork","provider":"qwenwork","auth_kind":"local","account_id":"qwenwork-local","auth_dir":"C:\\Users\\WenPaul\\AppData\\Roaming\\QwenWorkCN"}`)
	req, _ := json.Marshal(map[string]any{
		"FileName": "qwenwork-local.json",
		"RawJSON":  base64.StdEncoding.EncodeToString(raw),
	})
	res, err := p.handleAuthParse(req)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, _ := json.Marshal(res)
	t.Logf("%s", string(b))
	if !strings.Contains(string(b), `"Handled":true`) {
		t.Fatalf("not handled: %s", b)
	}
}
