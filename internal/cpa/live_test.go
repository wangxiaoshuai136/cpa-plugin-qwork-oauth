package cpa

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestLiveTicketAndChat(t *testing.T) {
	if os.Getenv("QWENWORK_LIVE") == "" {
		t.Skip("set QWENWORK_LIVE=1 to run live test")
	}
	cfg := DefaultPluginConfig()
	cred, err := credentialFromStorage(nil, cfg)
	if err != nil {
		t.Fatalf("credential: %v", err)
	}
	sess, err := ensureSessionFor(cred)
	if err != nil {
		t.Fatalf("ensureSessionFor: %v", err)
	}
	t.Logf("base=%s tokenLen=%d expires=%s", sess.BaseURL, len(sess.AccessToken), sess.ExpiresAt.Format("15:04:05"))

	p := NewPlugin(nil)
	p.config = cfg
	resp, err := p.upstream("flash", []byte(`{"model":"flash","messages":[{"role":"user","content":"只回答ok"}],"max_tokens":8}`), false, nil)
	if err != nil {
		t.Fatalf("upstream: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	t.Logf("status=%d body=%s", resp.StatusCode, truncate(string(body), 160))
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil || len(out.Choices) == 0 {
		t.Fatalf("bad response: %v", err)
	}
	_ = context.Background()
}

