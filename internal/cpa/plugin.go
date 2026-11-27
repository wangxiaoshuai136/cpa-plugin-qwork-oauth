package cpa

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginabi"
	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"
)

const (
	ProviderID = "qwenwork"
	Version    = "0.2.0"
)

type HostCallFunc func(method string, payload []byte) ([]byte, error)

type rpcRegistration struct {
	SchemaVersion uint32             `json:"schema_version"`
	Metadata      pluginapi.Metadata `json:"metadata"`
	Capabilities  rpcCapabilities    `json:"capabilities"`
}

type rpcCapabilities struct {
	ModelProvider         bool                         `json:"model_provider"`
	AuthProvider          bool                         `json:"auth_provider"`
	Scheduler             bool                         `json:"scheduler"`
	ModelRouter           bool                         `json:"model_router"`
	Executor              bool                         `json:"executor"`
	ExecutorModelScope    pluginapi.ExecutorModelScope `json:"executor_model_scope,omitempty"`
	ExecutorInputFormats  []string                     `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string                     `json:"executor_output_formats,omitempty"`
	ManagementAPI         bool                         `json:"management_api"`
	QuotaProvider         bool                         `json:"quota_provider"`
}

type rpcIdentifierResponse struct {
	Identifier          string                        `json:"identifier"`
	Identifiers         []string                      `json:"identifiers,omitempty"`
	CredentialProviders []string                      `json:"credential_providers,omitempty"`
	Entries             []pluginapi.AuthProviderEntry `json:"entries,omitempty"`
}

type Plugin struct {
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	config   PluginConfig
	hostCall HostCallFunc
}

func NewPlugin(hostCall HostCallFunc) *Plugin {
	ctx, cancel := context.WithCancel(context.Background())
	return &Plugin{ctx: ctx, cancel: cancel, config: DefaultPluginConfig(), hostCall: hostCall}
}

func (p *Plugin) SetHostCall(fn HostCallFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hostCall = fn
}

func (p *Plugin) callHost(method string, req any) ([]byte, error) {
	p.mu.RLock()
	fn := p.hostCall
	p.mu.RUnlock()
	if fn == nil {
		return nil, fmt.Errorf("hostCall is nil")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return fn(method, payload)
}

func (p *Plugin) emitStreamChunk(streamID string, payload []byte) error {
	_, err := p.callHost(pluginabi.MethodHostStreamEmit, map[string]any{"stream_id": streamID, "payload": payload})
	return err
}

func (p *Plugin) closeStream(streamID string, streamErr error) error {
	req := map[string]any{"stream_id": streamID}
	if streamErr != nil {
		req["error"] = streamErr.Error()
	}
	_, err := p.callHost(pluginabi.MethodHostStreamClose, req)
	return err
}

func (p *Plugin) Config() PluginConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *Plugin) HandleMethod(method string, raw []byte) (any, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return p.handleRegister(raw)
	case pluginabi.MethodPluginQuiesce:
		return map[string]any{"quiesced": true}, nil
	case pluginabi.MethodPluginShutdown:
		p.cancel()
		return map[string]any{"shutdown": true}, nil
	case pluginabi.MethodAuthIdentifier:
		return rpcIdentifierResponse{Identifier: ProviderID, Identifiers: []string{ProviderID}, CredentialProviders: []string{ProviderID}, Entries: []pluginapi.AuthProviderEntry{{Identifier: ProviderID, Name: "QWork"}}}, nil
	case pluginabi.MethodAuthParse:
		return p.handleAuthParse(raw)
	case pluginabi.MethodAuthRefresh:
		return p.handleAuthRefresh(raw)
	case pluginabi.MethodAuthLoginStart:
		return p.handleAuthLoginStart(raw)
	case pluginabi.MethodAuthLoginPoll:
		return p.handleAuthLoginPoll(raw)
	case pluginabi.MethodModelStatic, pluginabi.MethodModelForAuth:
		return p.handleModelStatic(raw)
	case pluginabi.MethodExecutorIdentifier:
		return rpcIdentifierResponse{Identifier: ProviderID, Identifiers: []string{ProviderID}}, nil
	case pluginabi.MethodExecutorExecute:
		return p.handleExecute(raw)
	case pluginabi.MethodExecutorExecuteStream:
		return p.handleExecuteStream(raw)
	case pluginabi.MethodExecutorCountTokens:
		return map[string]any{"tokens": 0}, nil
	case pluginabi.MethodQuotaIdentifier:
		return p.handleQuotaIdentifier()
	case pluginabi.MethodQuotaDescribe:
		return p.handleQuotaDescribe()
	case pluginabi.MethodQuotaFetch:
		return p.handleQuotaFetch(raw)
	case pluginabi.MethodManagementRegister:
		return p.handleManagementRegister(raw)
	case pluginabi.MethodManagementHandle:
		return p.handleManagementHandle(raw)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (p *Plugin) handleRegister(raw []byte) (any, error) {
	if len(raw) > 0 {
		var in struct {
			ConfigYAML []byte `json:"config_yaml"`
		}
		if err := json.Unmarshal(raw, &in); err == nil && len(in.ConfigYAML) > 0 {
			cfg := ParsePluginConfig(in.ConfigYAML)
			p.mu.Lock()
			p.config = cfg
			p.mu.Unlock()
		}
	}

	return rpcRegistration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "QWork OAuth",
			Version:          Version,
			Author:           "王小帅",
			GitHubRepository: "https://github.com/wangxiaoshuai136/cpa-plugin-qwork-oauth",
			Logo:             "https://raw.githubusercontent.com/wangxiaoshuai136/cpa-plugin-qwork-oauth/main/assets/icon.png?v=2",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "auth_dir", Type: pluginapi.ConfigFieldTypeString, Default: DefaultAuthDir(), Description: "千问办公客户端登录态目录（含 auth-v2.dat 与 Local State）"},
				{Name: "model_prefix", Type: pluginapi.ConfigFieldTypeString, Default: "", Description: "模型名前缀，例如 qwenwork"},
				{Name: "base_url", Type: pluginapi.ConfigFieldTypeString, Default: "", Description: "覆盖 MaaS 上游地址，留空使用换票返回值"},
			},
		},
		Capabilities: rpcCapabilities{
			ModelProvider:         true,
			AuthProvider:          true,
			Executor:              true,
			ExecutorModelScope:    pluginapi.ExecutorModelScopeStatic,
			ExecutorInputFormats:  []string{"chat-completions"},
			ExecutorOutputFormats: []string{"chat-completions"},
			QuotaProvider:         true,
			ManagementAPI:         true,
		},
	}, nil
}

func (p *Plugin) handleModelStatic(raw []byte) (any, error) {
	prefix := strings.Trim(strings.TrimSpace(p.Config().ModelPrefix), "/")
	return pluginapi.ModelResponse{Provider: ProviderID, Models: StaticModels(prefix)}, nil
}












