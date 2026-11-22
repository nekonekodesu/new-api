package relay

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	claudechannel "github.com/QuantumNous/new-api/relay/channel/claude"
	geminichannel "github.com/QuantumNous/new-api/relay/channel/gemini"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const learnedProtocolRouteTTL = 30 * time.Minute

type protocolRouteKey struct {
	ChannelID    int
	Model        string
	ClientFormat types.RelayFormat
}

type learnedProtocolEntry struct {
	TargetFormat types.RelayFormat
	ExpiresAt    int64
}

var learnedProtocolRoutes sync.Map

func buildProtocolRouteKey(info *relaycommon.RelayInfo) (protocolRouteKey, bool) {
	if info == nil || info.ChannelMeta == nil || info.ChannelId <= 0 || info.RelayFormat == "" {
		return protocolRouteKey{}, false
	}
	model := strings.ToLower(strings.TrimSpace(info.UpstreamModelName))
	if model == "" {
		model = strings.ToLower(strings.TrimSpace(info.OriginModelName))
	}
	if model == "" {
		return protocolRouteKey{}, false
	}
	return protocolRouteKey{
		ChannelID:    info.ChannelId,
		Model:        model,
		ClientFormat: info.RelayFormat,
	}, true
}

// ApplyLearnedProtocolFormat sets info.ForceTargetRelayFormat when a prior
// request on the same channel and model discovered that the upstream requires
// a converted protocol format.
func ApplyLearnedProtocolFormat(info *relaycommon.RelayInfo) {
	key, ok := buildProtocolRouteKey(info)
	if !ok {
		return
	}
	val, found := learnedProtocolRoutes.Load(key)
	if !found {
		return
	}
	entry, ok := val.(learnedProtocolEntry)
	if !ok || time.Now().Unix() > entry.ExpiresAt {
		learnedProtocolRoutes.Delete(key)
		return
	}
	info.ForceTargetRelayFormat = entry.TargetFormat
}

// RememberLearnedTargetFormat records a working converted protocol format for
// the channel, model, and client format.
func RememberLearnedTargetFormat(info *relaycommon.RelayInfo, targetFormat types.RelayFormat) {
	if targetFormat == "" {
		return
	}
	key, ok := buildProtocolRouteKey(info)
	if !ok {
		return
	}
	learnedProtocolRoutes.Store(key, learnedProtocolEntry{
		TargetFormat: targetFormat,
		ExpiresAt:    time.Now().Add(learnedProtocolRouteTTL).Unix(),
	})
}

func isProtocolFallbackEligibleStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity,
		http.StatusInternalServerError,
		http.StatusNotImplemented,
		http.StatusBadGateway,
		http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func supportsProtocolFallbackChannel(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	switch info.ApiType {
	case constant.APITypeNewAPI,
		constant.APITypeSub2API,
		constant.APITypeOpenAI,
		constant.APITypeOpenRouter,
		constant.APITypeXinference,
		constant.APITypeDeepSeek,
		constant.APITypeAli,
		constant.APITypeVolcEngine,
		constant.APITypeZhipuV4,
		constant.APITypeSiliconFlow,
		constant.APITypeMoonshot,
		constant.APITypeMiniMax,
		constant.APITypeOllama,
		constant.APITypeBaiduV2,
		constant.APITypeXai:
		return true
	default:
		return false
	}
}

func fallbackTargetFormats(info *relaycommon.RelayInfo, triedFormat types.RelayFormat) []types.RelayFormat {
	if !supportsProtocolFallbackChannel(info) {
		return nil
	}
	if triedFormat == "" {
		triedFormat = info.GetFinalRequestRelayFormat()
	}
	if triedFormat == "" {
		triedFormat = info.RelayFormat
	}

	var candidates []types.RelayFormat
	addCandidate := func(f types.RelayFormat) {
		if f == "" || f == triedFormat || slices.Contains(candidates, f) {
			return
		}
		candidates = append(candidates, f)
	}

	// OpenAI Chat (/v1/chat/completions) is the universal common denominator.
	if triedFormat != types.RelayFormatOpenAI {
		addCandidate(types.RelayFormatOpenAI)
	}

	isMultiProtocolGateway := info.ApiType == constant.APITypeNewAPI || info.ApiType == constant.APITypeSub2API
	if isMultiProtocolGateway {
		if info.TargetsClaudeModel() {
			addCandidate(types.RelayFormatClaude)
		}
		if info.TargetsGeminiModel() {
			addCandidate(types.RelayFormatGemini)
		}
		if info.TargetsNativeResponsesCapableModel() {
			addCandidate(types.RelayFormatOpenAIResponses)
		}
		addCandidate(info.RelayFormat)
	} else if info.ApiType == constant.APITypeOpenAI && info.TargetsNativeResponsesCapableModel() {
		addCandidate(types.RelayFormatOpenAIResponses)
	}

	return candidates
}

// TryProtocolFallbackOnError attempts alternate protocol conversions on the
// same channel when an upstream rejects the initial wire format before any
// response bytes are sent to the client.
func TryProtocolFallbackOnError(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	adaptor channel.Adaptor,
	sourceRequest any,
	httpResp *http.Response,
	statusCodeMappingStr string,
) (*dto.Usage, *types.NewAPIError, bool) {
	if c == nil || info == nil || adaptor == nil || sourceRequest == nil || httpResp == nil {
		return nil, nil, false
	}
	if c.Writer != nil && c.Writer.Written() {
		return nil, nil, false
	}
	if !isProtocolFallbackEligibleStatus(httpResp.StatusCode) {
		return nil, nil, false
	}
	triedFormat := info.GetFinalRequestRelayFormat()
	candidates := fallbackTargetFormats(info, triedFormat)
	if len(candidates) == 0 {
		return nil, nil, false
	}

	// Close the failed primary response body before retrying with conversion.
	service.CloseResponseBodyGracefully(httpResp)

	var lastErr *types.NewAPIError
	for _, targetFormat := range candidates {
		usage, apiErr := executeProtocolFallbackAttempt(c, info, adaptor, sourceRequest, targetFormat, statusCodeMappingStr)
		if apiErr == nil {
			RememberLearnedTargetFormat(info, targetFormat)
			return usage, nil, true
		}
		lastErr = apiErr
		if c.Writer != nil && c.Writer.Written() {
			return nil, lastErr, true
		}
	}
	return nil, lastErr, true
}

func executeProtocolFallbackAttempt(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	adaptor channel.Adaptor,
	sourceRequest any,
	targetFormat types.RelayFormat,
	statusCodeMappingStr string,
) (*dto.Usage, *types.NewAPIError) {
	savedRelayFormat := info.RelayFormat
	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	savedForceTarget := info.ForceTargetRelayFormat

	info.ForceTargetRelayFormat = targetFormat
	info.FinalRequestRelayFormat = targetFormat
	info.RequestConversionChain = nil
	info.InitRequestConversionChain()
	info.ClaudeToChatStreamState = nil
	info.ChatToGeminiStreamState = nil
	info.ClaudeConvertInfo = nil
	info.SendResponseCount = 0

	defer func() {
		info.RelayFormat = savedRelayFormat
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
		info.ForceTargetRelayFormat = savedForceTarget
	}()

	if targetFormat == types.RelayFormatOpenAIResponses {
		return textRequestViaResponses(c, info, adaptor, sourceRequest)
	}

	var (
		convertedRequest any
		targetMode       int
		targetPath       string
	)

	switch targetFormat {
	case types.RelayFormatOpenAI:
		result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAI, sourceRequest)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		openaiReq, ok := result.Value.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, types.NewError(fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if info.UpstreamModelName != "" {
			openaiReq.Model = info.UpstreamModelName
		}
		if info.SupportStreamOptions && info.IsStream {
			openaiReq.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
		}
		convertedRequest, err = adaptor.ConvertOpenAIRequest(c, info, openaiReq)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		targetMode = relayconstant.RelayModeChatCompletions
		targetPath = "/v1/chat/completions"

	case types.RelayFormatClaude:
		result, err := service.ConvertRequest(c, info, types.RelayFormatClaude, sourceRequest)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		claudeReq, ok := result.Value.(*dto.ClaudeRequest)
		if !ok {
			return nil, types.NewError(fmt.Errorf("expected Anthropic Messages request, got %T", result.Value), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if info.UpstreamModelName != "" {
			claudeReq.Model = info.UpstreamModelName
		}
		claudeAdaptor := &claudechannel.Adaptor{}
		convertedRequest, err = claudeAdaptor.ConvertClaudeRequest(c, info, claudeReq)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		targetMode = relayconstant.RelayModeUnknown
		targetPath = "/v1/messages"

	case types.RelayFormatGemini:
		result, err := service.ConvertRequest(c, info, types.RelayFormatGemini, sourceRequest)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		geminiReq, ok := result.Value.(*dto.GeminiChatRequest)
		if !ok {
			return nil, types.NewError(fmt.Errorf("expected Gemini generateContent request, got %T", result.Value), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		geminiAdaptor := &geminichannel.Adaptor{}
		convertedRequest, err = geminiAdaptor.ConvertGeminiRequest(c, info, geminiReq)
		if err != nil {
			return nil, newConvertRequestFailedError(c, info, err)
		}
		targetMode = relayconstant.RelayModeGemini
		action := "generateContent"
		if info.IsStream {
			action = "streamGenerateContent?alt=sse"
		}
		targetPath = fmt.Sprintf("/v1beta/models/%s:%s", info.UpstreamModelName, action)

	default:
		return nil, types.NewError(fmt.Errorf("unsupported fallback target format %s", targetFormat), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	info.FinalRequestRelayFormat = targetFormat
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	var apiErr *types.NewAPIError
	if jsonData, apiErr = requestplugin.Outbound(c, info, jsonData); apiErr != nil {
		return nil, apiErr
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}
	if apiErr = requestplugin.Egress(c, info, jsonData); apiErr != nil {
		return nil, apiErr
	}

	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()

	// Temporarily align RelayFormat/RelayMode/RequestURLPath with the target
	// wire format so the channel adaptor builds the target endpoint URL and headers.
	info.RelayFormat = targetFormat
	info.RelayMode = targetMode
	info.RequestURLPath = targetPath
	resp, reqErr := adaptor.DoRequest(c, info, body)
	info.RelayFormat = savedRelayFormat
	info.RelayMode = savedRelayMode
	info.RequestURLPath = savedRequestURLPath

	if reqErr != nil {
		return nil, types.NewOpenAIError(reqErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	httpResp := resp.(*http.Response)
	info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
	if httpResp.StatusCode != http.StatusOK {
		newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}

	var usage any
	switch targetFormat {
	case types.RelayFormatOpenAI:
		if savedRelayFormat == types.RelayFormatOpenAIResponses {
			if info.IsStream {
				usage, apiErr = openaichannel.OaiChatToResponsesStreamHandler(c, info, httpResp)
			} else {
				usage, apiErr = openaichannel.OaiChatToResponsesHandler(c, info, httpResp)
			}
		} else {
			if info.IsStream {
				usage, apiErr = openaichannel.OaiStreamHandler(c, info, httpResp)
			} else {
				usage, apiErr = openaichannel.OpenaiHandler(c, info, httpResp)
			}
		}
	case types.RelayFormatClaude:
		claudeAdaptor := &claudechannel.Adaptor{}
		usage, apiErr = claudeAdaptor.DoResponse(c, httpResp, info)
	case types.RelayFormatGemini:
		geminiAdaptor := &geminichannel.Adaptor{}
		usage, apiErr = geminiAdaptor.DoResponse(c, httpResp, info)
	}

	if apiErr != nil {
		service.ResetStatusCode(apiErr, statusCodeMappingStr)
		return nil, apiErr
	}
	usageDto, _ := usage.(*dto.Usage)
	return usageDto, nil
}
