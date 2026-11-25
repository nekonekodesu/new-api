package relay

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// PrepareResponsesRequest applies the same model, conversion and channel rules
// for HTTP and WebSocket requests. The caller closes closer after the attempt;
// passthrough bodies remain owned by the incoming request's BodyStorage.
// The returned adaptor retains route/conversion state for DoRequest/DoResponse.
func PrepareResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) (relaychannel.Adaptor, common.ReplayableBody, io.Closer, *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		!common.SupportsResponsesCompact(info.ChannelType, info.ApiType) {
		return nil, nil, nil, types.NewErrorWithStatusCode(
			fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	request, err := common.DeepCopy(req)
	if err != nil {
		return nil, nil, nil, types.NewError(fmt.Errorf("failed to copy responses request: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return nil, nil, nil, newConvertRequestFailedError(c, info, err)
	}
	ApplyLearnedProtocolFormat(info)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, nil, nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	if info.ShouldPassThroughBody() {
		body, apiErr := preparePassThroughRequestBody(c, info, false)
		if apiErr != nil {
			return nil, nil, nil, apiErr
		}
		return adaptor, body, io.NopCloser(body), nil
	}

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
	if err != nil {
		return nil, nil, nil, newConvertRequestFailedError(c, info, err)
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	body, closer, apiErr := buildOutboundJSONRequestBody(c, info, convertedRequest, outboundJSONOptions{
		removeDisabledFields: true,
		finalizerAdaptor:     adaptor,
		debugLogPrefix:       "requestBody: ",
	})
	if apiErr != nil {
		return nil, nil, nil, apiErr
	}
	return adaptor, body, closer, nil
}
