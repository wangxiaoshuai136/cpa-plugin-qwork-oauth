package cpa

import (
	"encoding/json"
	"net/http"

	"github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"
)

// handleManagementRegister only exposes static browser assets. Anything that
// reads credentials or calls upstream must live behind /v0/management/, which
// the host authenticates with the management key.
func (p *Plugin) handleManagementRegister(raw []byte) (any, error) {
	return pluginapi.ManagementRegistrationResponse{
		Resources: []pluginapi.ResourceRoute{
			{Path: "/icon.png", Description: "QWork icon"},
		},
	}, nil
}

func (p *Plugin) handleManagementHandle(raw []byte) (any, error) {
	var req pluginapi.ManagementRequest
	_ = json.Unmarshal(raw, &req)
	headers := http.Header{}
	headers.Set("Content-Type", "image/png")
	headers.Set("Cache-Control", "no-cache")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: headers, Body: iconBytes()}, nil
}
