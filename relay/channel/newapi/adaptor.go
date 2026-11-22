package newapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	openaiAdaptor openai.Adaptor
	claudeAdaptor claude.Adaptor
	geminiAdaptor gemini.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.openaiAdaptor.Init(info)
	a.claudeAdaptor.Init(info)
	a.geminiAdaptor.Init(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeAlphaSearch {
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, "/v1/alpha/search", info.ChannelType), nil
	}
	finalFormat := info.GetFinalRequestRelayFormat()
	if info.FinalRequestRelayFormat == "" && info.ForceTargetRelayFormat != "" {
		finalFormat = info.ForceTargetRelayFormat
	}
	if finalFormat == types.RelayFormatClaude && info.RelayFormat != types.RelayFormatClaude {
		return a.claudeAdaptor.GetRequestURL(info)
	}
	if finalFormat == types.RelayFormatGemini && (info.RelayFormat != types.RelayFormatGemini || (info.IsModelMapped && !info.ShouldPassThroughBody())) {
		return a.geminiAdaptor.GetRequestURL(info)
	}
	if finalFormat == types.RelayFormatOpenAIResponses && info.RelayFormat != types.RelayFormatOpenAIResponses {
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, "/v1/responses", info.ChannelType), nil
	}
	if finalFormat == types.RelayFormatOpenAI && info.RelayFormat != types.RelayFormatOpenAI {
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, "/v1/chat/completions", info.ChannelType), nil
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)

	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatClaude:
		claude.CommonClaudeHeadersOperation(c, req, info)
		req.Set("x-api-key", info.ApiKey)
		if req.Get("anthropic-version") == "" {
			anthropicVersion := c.Request.Header.Get("anthropic-version")
			if anthropicVersion == "" {
				anthropicVersion = "2023-06-01"
			}
			req.Set("anthropic-version", anthropicVersion)
		}
	case types.RelayFormatGemini:
		req.Set("x-goog-api-key", info.ApiKey)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	target := types.RelayFormatOpenAI
	if info != nil && info.ForceTargetRelayFormat != "" {
		target = info.ForceTargetRelayFormat
	} else if info.TargetsClaudeModel() {
		target = types.RelayFormatClaude
	} else if info.TargetsResponsesOnlyModel() {
		target = types.RelayFormatOpenAIResponses
	}
	switch target {
	case types.RelayFormatClaude:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatClaude
		}
		return a.claudeAdaptor.ConvertOpenAIRequest(c, info, request)
	case types.RelayFormatGemini:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatGemini
		}
		return a.geminiAdaptor.ConvertOpenAIRequest(c, info, request)
	case types.RelayFormatOpenAIResponses:
		result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, request)
		if err != nil {
			return nil, err
		}
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
		}
		return result.Value, nil
	default:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatOpenAI
		}
		return request, nil
	}
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	var target types.RelayFormat = types.RelayFormatOpenAIResponses
	if info != nil && info.ForceTargetRelayFormat != "" {
		target = info.ForceTargetRelayFormat
	} else if info.TargetsClaudeModel() {
		target = types.RelayFormatClaude
	} else if info.TargetsGeminiModel() {
		target = types.RelayFormatGemini
	}
	switch target {
	case types.RelayFormatClaude:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatClaude
		}
		return a.claudeAdaptor.ConvertOpenAIResponsesRequest(c, info, request)
	case types.RelayFormatGemini:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatGemini
		}
		return a.geminiAdaptor.ConvertOpenAIResponsesRequest(c, info, request)
	case types.RelayFormatOpenAI:
		return openai.ConvertResponsesToOpenAIWithAdaptor(c, info, request, a.openaiAdaptor.ConvertOpenAIRequest)
	default:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
		}
		return request, nil
	}
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	var target types.RelayFormat = types.RelayFormatClaude
	if info != nil && info.ForceTargetRelayFormat != "" {
		target = info.ForceTargetRelayFormat
	} else if info.TargetsGeminiModel() {
		target = types.RelayFormatGemini
	} else if info.TargetsResponsesOnlyModel() {
		target = types.RelayFormatOpenAIResponses
	}
	switch target {
	case types.RelayFormatOpenAI:
		return openai.ConvertClaudeToOpenAIWithAdaptor(c, info, request, a.openaiAdaptor.ConvertOpenAIRequest)
	case types.RelayFormatGemini:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatGemini
		}
		return a.geminiAdaptor.ConvertClaudeRequest(c, info, request)
	case types.RelayFormatOpenAIResponses:
		result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, request)
		if err != nil {
			return nil, err
		}
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
		}
		return result.Value, nil
	default:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatClaude
		}
		return a.claudeAdaptor.ConvertClaudeRequest(c, info, request)
	}
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	var target types.RelayFormat = types.RelayFormatGemini
	if info != nil && info.ForceTargetRelayFormat != "" {
		target = info.ForceTargetRelayFormat
	} else if info.TargetsClaudeModel() {
		target = types.RelayFormatClaude
	} else if info.TargetsResponsesOnlyModel() {
		target = types.RelayFormatOpenAIResponses
	} else if info != nil && !info.TargetsGeminiModel() && (info.UpstreamModelName != "" || info.OriginModelName != "") {
		target = types.RelayFormatOpenAI
	}
	switch target {
	case types.RelayFormatClaude:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatClaude
		}
		return a.claudeAdaptor.ConvertGeminiRequest(c, info, request)
	case types.RelayFormatOpenAI:
		return openai.ConvertGeminiToOpenAIWithAdaptor(c, info, request, a.openaiAdaptor.ConvertOpenAIRequest)
	case types.RelayFormatOpenAIResponses:
		result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, request)
		if err != nil {
			return nil, err
		}
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
		}
		return result.Value, nil
	default:
		if info != nil {
			info.FinalRequestRelayFormat = types.RelayFormatGemini
		}
		return a.geminiAdaptor.ConvertGeminiRequest(c, info, request)
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return a.openaiAdaptor.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatClaude:
		return a.claudeAdaptor.DoResponse(c, resp, info)
	case types.RelayFormatGemini:
		return a.geminiAdaptor.DoResponse(c, resp, info)
	case types.RelayFormatOpenAIResponses:
		if info.RelayFormat != types.RelayFormatOpenAIResponses {
			if info.IsStream {
				return openai.OaiResponsesToChatStreamHandler(c, info, resp)
			}
			return openai.OaiResponsesToChatHandler(c, info, resp)
		}
		return a.openaiAdaptor.DoResponse(c, resp, info)
	default:
		return a.openaiAdaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
