package relay

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

type outboundJSONOptions struct {
	removeDisabledFields bool
	skipOutbound         bool
	finalizerAdaptor     relaychannel.Adaptor
	marshalErrCode       types.ErrorCode
	debugLogPrefix       string
}

func preparePassThroughRequestBody(c *gin.Context, info *relaycommon.RelayInfo, debugLog bool) (common.ReplayableBody, *types.NewAPIError) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if apiErr := requestplugin.PassThrough(c, info, storage); apiErr != nil {
		return nil, apiErr
	}
	if debugLog && common.DebugEnabled {
		if debugBytes, bErr := storage.Bytes(); bErr == nil {
			logger.LogDebug(c, "requestBody: %s", debugBytes)
		}
	}
	return common.NewReplayableBodyReader(storage), nil
}

func buildOutboundJSONRequestBody(c *gin.Context, info *relaycommon.RelayInfo, converted any, opts outboundJSONOptions) (common.ReplayableBody, io.Closer, *types.NewAPIError) {
	jsonData, err := common.Marshal(converted)
	if err != nil {
		errCode := opts.marshalErrCode
		if errCode == "" {
			errCode = types.ErrorCodeConvertRequestFailed
		}
		return nil, nil, types.NewError(err, errCode, types.ErrOptionWithSkipRetry())
	}
	if opts.removeDisabledFields {
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
	}
	if !opts.skipOutbound {
		var apiErr *types.NewAPIError
		if jsonData, apiErr = requestplugin.Outbound(c, info, jsonData); apiErr != nil {
			return nil, nil, apiErr
		}
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return nil, nil, newAPIErrorFromParamOverride(err)
			}
		}
	}
	if opts.finalizerAdaptor != nil {
		if finalizer, ok := opts.finalizerAdaptor.(relaychannel.ResponsesRequestFinalizer); ok {
			jsonData, err = finalizer.FinalizeResponsesRequest(info, jsonData)
			if err != nil {
				return nil, nil, newConvertRequestFailedError(c, info, err)
			}
		}
	}
	// Egress inspects the final wire body after param overrides and channel finalization.
	if apiErr := requestplugin.Egress(c, info, jsonData); apiErr != nil {
		return nil, nil, apiErr
	}
	if opts.debugLogPrefix != "" {
		logger.LogDebug(c, opts.debugLogPrefix+"%s", jsonData)
	}
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return body, closer, nil
}

func isResponsesEventStreamContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
