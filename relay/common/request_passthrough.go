package common

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// ShouldPassThroughBody permits raw bodies only when the selected adaptor
// accepts the client's wire format. Otherwise conversion must run even when
// passthrough was enabled globally or on the channel.
func (info *RelayInfo) ShouldPassThroughBody() bool {
	if info == nil {
		return false
	}
	if info.ForceTargetRelayFormat != "" && info.ForceTargetRelayFormat != info.RelayFormat {
		return false
	}
	enabled := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	if info.ChannelMeta == nil {
		return enabled
	}
	if !enabled && !info.ChannelSetting.PassThroughBodyEnabled {
		return false
	}
	if info.RelayFormat == "" {
		return true
	}
	switch info.ApiType {
	case constant.APITypeOpenAI, constant.APITypeOpenRouter, constant.APITypeXinference:
		if info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini {
			return false
		}
		if info.RelayFormat == types.RelayFormatOpenAIResponses {
			return info.ApiType == constant.APITypeOpenAI && info.TargetsNativeResponsesCapableModel()
		}
		if info.TargetsResponsesOnlyModel() {
			return false
		}
		return true
	case constant.APITypeAnthropic:
		return info.RelayFormat == types.RelayFormatClaude
	case constant.APITypeGemini:
		return info.RelayFormat == types.RelayFormatGemini
	case constant.APITypeAws, constant.APITypeCodex:
		return false
	case constant.APITypeVertexAi:
		if info.TargetsClaudeModel() {
			return false
		}
		return info.RelayFormat == types.RelayFormatGemini
	case constant.APITypeNewAPI, constant.APITypeSub2API:
		if info.TargetsClaudeModel() {
			return info.RelayFormat == types.RelayFormatClaude
		}
		if info.TargetsGeminiModel() {
			return info.RelayFormat == types.RelayFormatGemini || info.RelayFormat == types.RelayFormatOpenAI
		}
		if info.TargetsResponsesOnlyModel() {
			return info.RelayFormat == types.RelayFormatOpenAIResponses
		}
		if info.RelayFormat == types.RelayFormatGemini {
			return false
		}
		if info.RelayFormat == types.RelayFormatOpenAIResponses {
			return info.TargetsNativeResponsesCapableModel()
		}
		return true
	case constant.APITypeDeepSeek,
		constant.APITypeAli,
		constant.APITypeVolcEngine,
		constant.APITypeZhipuV4,
		constant.APITypeSiliconFlow,
		constant.APITypeMoonshot,
		constant.APITypeMiniMax,
		constant.APITypeOllama,
		constant.APITypeBaiduV2,
		constant.APITypeXai,
		constant.APITypeCloudflare:
		return info.RelayFormat != types.RelayFormatGemini
	default:
		return info.RelayFormat == types.RelayFormatOpenAI
	}
}

func (info *RelayInfo) normalizedTargetModel() string {
	if info == nil {
		return ""
	}
	model := ""
	if info.ChannelMeta != nil {
		model = info.UpstreamModelName
	}
	if model == "" {
		model = info.OriginModelName
	}
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = model[slash+1:]
	}
	return strings.ToLower(strings.TrimSpace(model))
}

func (info *RelayInfo) TargetsClaudeModel() bool {
	model := info.normalizedTargetModel()
	return strings.HasPrefix(model, "claude")
}

func (info *RelayInfo) TargetsGeminiModel() bool {
	model := info.normalizedTargetModel()
	return strings.HasPrefix(model, "gemini") ||
		strings.HasPrefix(model, "learnlm") ||
		strings.HasPrefix(model, "gemma-3")
}

func (info *RelayInfo) TargetsResponsesOnlyModel() bool {
	model := info.normalizedTargetModel()
	return common.IsOpenAIResponseOnlyModel(model)
}

func (info *RelayInfo) TargetsNativeResponsesCapableModel() bool {
	model := info.normalizedTargetModel()
	if model == "" {
		return false
	}
	if common.IsOpenAIResponseOnlyModel(model) {
		return true
	}
	return strings.HasPrefix(model, "gpt-5") ||
		strings.HasPrefix(model, "gpt-4.1") ||
		strings.HasPrefix(model, "gpt-4o") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") ||
		strings.HasPrefix(model, "chatgpt-") ||
		strings.HasPrefix(model, "computer-use")
}

func (info *RelayInfo) PreferredTargetRelayFormat() types.RelayFormat {
	if info == nil {
		return types.RelayFormatOpenAI
	}
	if info.ForceTargetRelayFormat != "" {
		return info.ForceTargetRelayFormat
	}
	if info.TargetsClaudeModel() {
		return types.RelayFormatClaude
	}
	if info.TargetsGeminiModel() {
		return types.RelayFormatGemini
	}
	if info.TargetsResponsesOnlyModel() {
		return types.RelayFormatOpenAIResponses
	}
	if info.RelayFormat == types.RelayFormatOpenAIResponses && info.TargetsNativeResponsesCapableModel() {
		return types.RelayFormatOpenAIResponses
	}
	return types.RelayFormatOpenAI
}
