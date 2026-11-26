package cpa

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginabi"
	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"
)

var (
	streamClient = &http.Client{}
	execClient   = &http.Client{Timeout: 5 * time.Minute}
)

type rpcExecutorRequest struct {
	pluginapi.ExecutorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcExecutorStreamResponse struct {
	Headers http.Header `json:"headers,omitempty"`
}

func (p *Plugin) upstream(model string, payload []byte, stream bool, storageJSON []byte) (*http.Response, error) {
	cfg := p.Config()
	clean := CleanModelName(model, cfg.ModelPrefix)
	if !IsKnownModel(clean, "") {
		return nil, pluginabi.NewError("model_not_found", "unknown qwenwork model: "+model, http.StatusNotFound)
	}
	if len(payload) == 0 {
		return nil, pluginabi.NewError("invalid_request", "empty request payload", http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, pluginabi.NewError("invalid_request", "payload is not a JSON object: "+err.Error(), http.StatusBadRequest)
	}
	body["model"] = clean
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	cred, err := credentialFromStorage(storageJSON, cfg)
	if err != nil {
		return nil, pluginabi.NewError("auth_error", "千问办公凭据不可用: "+err.Error(), http.StatusUnauthorized)
	}
	for attempt := 0; attempt < 2; attempt++ {
		sess, err := ensureSessionFor(cred)
		if err != nil {
			return nil, pluginabi.NewError("auth_error", "千问办公登录态不可用: "+err.Error(), http.StatusUnauthorized)
		}
		base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
		if base == "" {
			base = sess.BaseURL
		}
		client := execClient
		if stream {
			client = streamClient
		}
		req, err := http.NewRequestWithContext(p.ctx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("authorization", "Bearer "+sess.AccessToken)
		req.Header.Set("content-type", "application/json")
		req.Header.Set("accept-encoding", "identity")
		if stream {
			req.Header.Set("accept", "text/event-stream")
		} else {
			req.Header.Set("accept", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			invalidateSession(cred)
			continue
		}
		return resp, nil
	}
	return nil, pluginabi.NewError("auth_error", "上游鉴权失败", http.StatusUnauthorized)
}

func (p *Plugin) handleExecute(raw []byte) (any, error) {
	var req pluginapi.ExecutorRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode executor.execute: %w", err)
	}
	payload := req.Payload
	if len(payload) == 0 {
		payload = req.OriginalRequest
	}
	resp, err := p.upstream(req.Model, payload, false, req.StorageJSON)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, pluginabi.NewError("upstream_error", truncate(string(body), 600), resp.StatusCode)
	}
	headers := http.Header{}
	if ct := resp.Header.Get("content-type"); ct != "" {
		headers.Set("content-type", ct)
	}
	return pluginapi.ExecutorResponse{Payload: body, Headers: headers}, nil
}

func (p *Plugin) handleExecuteStream(raw []byte) (any, error) {
	var req rpcExecutorRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode executor.execute_stream: %w", err)
	}
	payload := req.Payload
	if len(payload) == 0 {
		payload = req.OriginalRequest
	}
	resp, err := p.upstream(req.Model, payload, true, req.StorageJSON)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return nil, pluginabi.NewError("upstream_error", truncate(string(body), 600), resp.StatusCode)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		streamID = newUUID()
	}
	go func() {
		defer resp.Body.Close()
		reader := bufio.NewReaderSize(resp.Body, 32*1024)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				if emitErr := p.emitStreamChunk(streamID, line); emitErr != nil {
					_ = p.closeStream(streamID, emitErr)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					_ = p.closeStream(streamID, err)
				} else {
					_ = p.closeStream(streamID, nil)
				}
				return
			}
		}
	}()
	return rpcExecutorStreamResponse{Headers: headers}, nil
}



