package cpa

import "github.com/wenbaolu6666-design/cpa-plugin-qwenwork/internal/pluginapi"

type modelDef struct {
	ID       string
	Name     string
	Desc     string
	Vision   bool
	Reasoning bool
}

var modelDefs = []modelDef{
	{"flash", "标准", "极速全能，秒杀日常任务", true, true},
	{"pro", "高级", "性能均衡，擅长复杂任务", false, true},
	{"qwen4-max", "Qwen4-Max", "强大出众，适合大型任务", true, true},
	{"qwen3.8-max-preview", "Qwen3.8-Max", "全维旗舰，驾驭高难任务", true, true},
	{"qwen3.8-flash", "Qwen3.8-Flash", "轻量高效，适合高频任务", true, true},
	{"minimax-m3.1-flash", "MiniMax-M3.1-Flash", "多模态，擅长办公、Agent 与编程", true, true},
	{"glm-5.4", "GLM-5.4", "擅长编程，适合多步骤和视觉任务", false, true},
	{"ling-3.1-flash", "Ling-3.1-Flash", "长程推理，擅长办公与编程", false, true},
	{"deepseek-flash", "DeepSeek-V4.1-Flash", "视觉推理，擅长编程与智能体任务", true, true},
	{"deepseek-v4-pro", "Deepseek-V4-Pro", "深度推理，擅长数理与编程", false, true},
	{"glm-5.3", "GLM-5.3", "百万上下文，擅长工程与 Agent", false, true},
	{"glm-5.3-flash", "GLM-5.3-Flash", "轻量多模态，擅长视觉编程", true, true},
	{"kimi-k3.1-pro", "Kimi-K3.1-Pro", "长上下文，擅长前端与深研", true, true},
	{"step-5-preview", "Step-5-Preview", "多模态，擅长 CLI、深研与编程", true, true},
}

func StaticModels(prefix string) []pluginapi.ModelInfo {
	out := make([]pluginapi.ModelInfo, 0, len(modelDefs))
	for _, m := range modelDefs {
		id := m.ID
		if prefix != "" {
			id = prefix + "/" + m.ID
		}
		mods := []string{"text"}
		if m.Vision {
			mods = append(mods, "image")
		}
		out = append(out, pluginapi.ModelInfo{
			ID:                         id,
			Object:                     "model",
			OwnedBy:                    ProviderID,
			Type:                       "chat",
			DisplayName:                m.Name,
			Name:                       m.ID,
			Description:                m.Desc,
			InputTokenLimit:            1000000,
			OutputTokenLimit:           32768,
			ContextLength:              1000000,
			MaxCompletionTokens:        32768,
			SupportedGenerationMethods: []string{"chat"},
			SupportedParameters:        []string{"temperature", "top_p", "max_tokens", "stream", "tools", "tool_choice"},
			SupportedInputModalities:   mods,
			SupportedOutputModalities:  []string{"text"},
		})
	}
	return out
}

func IsKnownModel(id string, prefix string) bool {
	clean := id
	if prefix != "" && len(id) > len(prefix)+1 && id[:len(prefix)+1] == prefix+"/" {
		clean = id[len(prefix)+1:]
	}
	for _, m := range modelDefs {
		if m.ID == clean {
			return true
		}
	}
	return false
}

func CleanModelName(id string, prefix string) string {
	if prefix != "" && len(id) > len(prefix)+1 && id[:len(prefix)+1] == prefix+"/" {
		return id[len(prefix)+1:]
	}
	return id
}
