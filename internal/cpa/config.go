package cpa

import (
	"os"
	"path/filepath"
	"strings"
)

type PluginConfig struct {
	AuthDir       string
	ModelPrefix   string
	BaseURL       string
	PublicBaseURL string
}

func DefaultAuthDir() string {
	return filepath.Join(os.Getenv("APPDATA"), "QwenWorkCN")
}

func DefaultPluginConfig() PluginConfig {
	return PluginConfig{AuthDir: DefaultAuthDir(), ModelPrefix: "qwenwork", PublicBaseURL: "http://127.0.0.1:8317"}
}

func ParsePluginConfig(raw []byte) PluginConfig {
	cfg := DefaultPluginConfig()
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "auth_dir":
			if v != "" {
				cfg.AuthDir = v
			}
		case "model_prefix":
			cfg.ModelPrefix = v
		case "base_url":
			cfg.BaseURL = v
		case "public_base_url":
			if v != "" {
				cfg.PublicBaseURL = v
			}
		}
	}
	return cfg
}
