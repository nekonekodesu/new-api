package common

import (
	"strings"

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
		return info.RelayFormat != types.RelayFormatClaude && info.RelayFormat != types.RelayFormatGemini
	case constant.APITypeAnthropic:
		return info.RelayFormat == types.RelayFormatClaude
	case constant.APITypeGemini:
		return info.RelayFormat == types.RelayFormatGemini
	case constant.APITypeNewAPI:
		if info.TargetsClaudeModel() {
			return info.RelayFormat == types.RelayFormatClaude
		}
		return true
	default:
		// Custom adaptors route their own native protocols.
		return true
	}
}

func (info *RelayInfo) TargetsClaudeModel() bool {
	if info == nil {
		return false
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = model[slash+1:]
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude")
}
