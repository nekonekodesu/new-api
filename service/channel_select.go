package service

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func GetChannelConstraints(c *gin.Context) *dto.ChannelConstraints {
	if c == nil {
		return &dto.ChannelConstraints{}
	}
	if existing, ok := common.GetContextKeyType[*dto.ChannelConstraints](c, constant.ContextKeyChannelConstraints); ok && existing != nil {
		return existing
	}
	constraints := &dto.ChannelConstraints{}
	common.SetContextKey(c, constant.ContextKeyChannelConstraints, constraints)
	return constraints
}

func AppendTaskPluginIdentityFilter(c *gin.Context, pluginKey string) {
	if c == nil {
		return
	}
	channelTypes, pluginKeys := pinnedTaskPluginIdentities(c, pluginKey)
	filter := dto.ChannelFilter{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          pluginKey,
		TaskPluginChannelTypes: channelTypes,
		TaskPluginKeys:         pluginKeys,
	}
	if c.Request != nil && c.Request.URL != nil {
		// An ordinary request may run on a plugin channel whose relay routes serve its path.
		filter.RequestPath = c.Request.URL.Path
	}
	GetChannelConstraints(c).AddFilter(filter)
}

type RetryParam struct {
	Ctx          *gin.Context
	TokenGroup   string
	ModelName    string
	RequestPath  string
	Retry        *int
	resetNextTry bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func (p *RetryParam) CanAttempt() bool {
	return p.GetRetry() <= common.RetryTimes || common.GetContextKeyBool(p.Ctx, constant.ContextKeyChannelCooldownBypassed)
}

func (p *RetryParam) RemainingRetries() int {
	remaining := common.RetryTimes - p.GetRetry()
	if common.GetContextKeyBool(p.Ctx, constant.ContextKeyChannelCooldownBypassed) {
		return max(1, remaining)
	}
	return remaining
}

var ErrChannelsCoolingDown = errors.New("every channel is cooling down")

const ginKeyChannelCooldownRoundTried = "channel_cooldown_round_tried"

func usedChannelIDs(c *gin.Context) []int {
	if c == nil {
		return nil
	}
	raw := c.GetStringSlice("use_channel")
	if len(raw) == 0 {
		return nil
	}
	ids := make([]int, 0, len(raw))
	for _, item := range raw {
		if id, err := strconv.Atoi(item); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func cooldownRoundTriedIDs(c *gin.Context) []int {
	if c == nil {
		return nil
	}
	if raw, ok := c.Get(ginKeyChannelCooldownRoundTried); ok {
		if ids, ok := raw.([]int); ok {
			return ids
		}
	}
	return nil
}

func selectSatisfiedChannelPass(
	param *RetryParam,
	filters []dto.ChannelFilter,
	ignoreCooldown bool,
	triedIDs []int,
) (*model.Channel, string, error) {
	selectGroup := param.TokenGroup
	affinityRetryOffset := param.Ctx.GetInt(ginKeyChannelAffinityRetryOffset)

	if param.TokenGroup != "auto" {
		priorityRetry := max(param.GetRetry()-affinityRetryOffset, 0)
		if ignoreCooldown {
			ch, err := model.GetRandomSatisfiedChannelIgnoringCooldown(param.TokenGroup, param.ModelName, priorityRetry, filters, triedIDs...)
			return ch, selectGroup, err
		}
		ch, err := model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, priorityRetry, filters, triedIDs...)
		return ch, selectGroup, err
	}

	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
	autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
	if len(autoGroups) == 0 {
		return nil, selectGroup, errors.New("auto groups is not enabled")
	}

	startGroupIndex := 0
	if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			startGroupIndex = idx
		}
	}
	crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
	selectedAutoGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyAutoGroup)
	if selectedAutoGroup != "" {
		selectGroup = selectedAutoGroup
	}

	for i := startGroupIndex; i < len(autoGroups); i++ {
		autoGroup := autoGroups[i]
		selectGroup = autoGroup
		groupRetry := param.GetRetry()
		// A new group starts from its highest priority without the affinity offset.
		if i > startGroupIndex {
			groupRetry = 0
		}
		priorityRetry := max(groupRetry-affinityRetryOffset, 0)
		logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

		var (
			channel *model.Channel
			err     error
		)
		if ignoreCooldown {
			channel, err = model.GetRandomSatisfiedChannelIgnoringCooldown(autoGroup, param.ModelName, priorityRetry, filters, triedIDs...)
		} else if !crossGroupRetry || len(triedIDs) == 0 || model.HasUntriedChannel(autoGroup, param.ModelName, filters, triedIDs) {
			channel, err = model.GetRandomSatisfiedChannel(autoGroup, param.ModelName, priorityRetry, filters, triedIDs...)
		}
		if err != nil {
			return nil, autoGroup, err
		}
		if channel == nil {
			if !crossGroupRetry && selectedAutoGroup != "" && autoGroup == selectedAutoGroup {
				break
			}
			logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
			param.SetRetry(0)
			affinityRetryOffset = 0
			param.Ctx.Set(ginKeyChannelAffinityRetryOffset, 0)
			continue
		}

		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
		logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

		if !ignoreCooldown && crossGroupRetry && groupRetry >= common.RetryTimes {
			logger.LogDebug(param.Ctx, "Current group %s retries exhausted (groupRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, groupRetry, common.RetryTimes)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			param.SetRetry(0)
			param.ResetRetryNextTry()
			param.Ctx.Set(ginKeyChannelAffinityRetryOffset, 0)
		} else {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
		}
		return channel, autoGroup, nil
	}
	return nil, selectGroup, nil
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	filters := GetChannelConstraints(param.Ctx).Filters
	if common.GetContextKeyBool(param.Ctx, constant.ContextKeyChannelCooldownBypassed) {
		tried := cooldownRoundTriedIDs(param.Ctx)
		channel, selectGroup, err := selectSatisfiedChannelPass(param, filters, true, tried)
		if err != nil {
			return nil, selectGroup, err
		}
		if channel == nil {
			common.SetContextKey(param.Ctx, constant.ContextKeyChannelCooldownBypassed, false)
			return nil, selectGroup, ErrChannelsCoolingDown
		}
		if param.Ctx != nil && !slices.Contains(tried, channel.Id) {
			param.Ctx.Set(ginKeyChannelCooldownRoundTried, append(slices.Clone(tried), channel.Id))
		}
		return channel, selectGroup, nil
	}

	channel, selectGroup, err := selectSatisfiedChannelPass(param, filters, false, usedChannelIDs(param.Ctx))
	if err != nil || channel != nil || !common.RetryCooldownEnabled {
		return channel, selectGroup, err
	}

	// Ordinary routing found no non-cooling candidate. Probe once whether
	// ignoring cooldown yields any candidate; if not, no channel exists at all
	// and the caller receives the normal nil-channel outcome instead of 503.
	if param.TokenGroup == "auto" {
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)
		selectedAutoGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyAutoGroup)
		if crossGroupRetry || selectedAutoGroup == "" {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, 0)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
		} else {
			userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
			if idx := slices.Index(GetRequestAutoGroups(param.Ctx, userGroup), selectedAutoGroup); idx >= 0 {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, idx)
			}
		}
	}
	param.SetRetry(0)
	if param.Ctx != nil {
		param.Ctx.Set(ginKeyChannelAffinityRetryOffset, 0)
	}

	tried := cooldownRoundTriedIDs(param.Ctx)
	channel, selectGroup, err = selectSatisfiedChannelPass(param, filters, true, tried)
	if err != nil || channel == nil {
		return nil, selectGroup, err
	}
	common.SetContextKey(param.Ctx, constant.ContextKeyChannelCooldownBypassed, true)
	if param.Ctx != nil && !slices.Contains(tried, channel.Id) {
		param.Ctx.Set(ginKeyChannelCooldownRoundTried, append(slices.Clone(tried), channel.Id))
	}
	return channel, selectGroup, nil
}

func pinnedTaskPluginIdentities(c *gin.Context, expected string) ([]int, []string) {
	if c == nil || expected == "" {
		return nil, nil
	}
	// A shared host endpoint and a native route several plugins declare both
	// admit a channel of any candidate.
	var generation *jsplugin.RoutingGeneration
	var candidates []*jsplugin.LoadedPlugin
	if value, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint); exists {
		if pinned, ok := value.(jsplugin.PinnedEndpoint); ok && len(pinned.Candidates) > 1 {
			generation = pinned.Generation
			for _, candidate := range pinned.Candidates {
				candidates = append(candidates, candidate.Plugin)
			}
		}
	} else if value, exists := c.Get(jsplugin.ContextKeyPinnedRoute); exists {
		if pinned, ok := value.(jsplugin.PinnedRoute); ok && len(pinned.Candidates) > 1 {
			generation = pinned.Generation
			for _, candidate := range pinned.Candidates {
				candidates = append(candidates, candidate.Plugin)
			}
		}
	}
	if generation != nil {
		expectedFound := false
		channelTypes := make([]int, 0, len(candidates))
		pluginKeys := make([]string, 0, len(candidates))
		seen := make(map[int]struct{}, len(candidates))
		for _, candidate := range candidates {
			if candidate == nil {
				continue
			}
			if candidate.Meta.Key == expected {
				expectedFound = true
			}
			pluginKeys = append(pluginKeys, candidate.Meta.Key)
			for _, channelType := range candidate.Meta.ChannelTypes {
				if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
					continue
				}
				if _, duplicate := seen[channelType]; duplicate {
					continue
				}
				if plugin, indexed := generation.GetByChannelType(channelType); indexed && plugin == candidate {
					seen[channelType] = struct{}{}
					channelTypes = append(channelTypes, channelType)
				}
			}
		}
		if expectedFound {
			return channelTypes, pluginKeys
		}
	}
	value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, ok := value.(jsplugin.PinnedPlugin)
	if !exists || !ok || pinned.Generation == nil || pinned.Plugin == nil || pinned.Plugin.Meta.Key != expected {
		return nil, nil
	}
	channelTypes := make([]int, 0, len(pinned.Plugin.Meta.ChannelTypes))
	for _, channelType := range pinned.Plugin.Meta.ChannelTypes {
		if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
			continue
		}
		channelTypes = append(channelTypes, channelType)
	}
	return channelTypes, []string{expected}
}

// ChannelSelectError explains why SelectChannelForRequest found no channel.
// Callers render it for their transport: the HTTP distributor localizes
// MessageID with its own helpers and the Responses WebSocket relay wraps it in
// a NewAPIError. Message is set instead of MessageID when the text is a fixed
// error code that clients match on.
type ChannelSelectError struct {
	StatusCode int
	Code       types.ErrorCode
	MessageID  string
	Params     map[string]any
	Message    string
	// FilterKind and Channel identify a candidate rejected by request filters.
	FilterKind dto.ChannelFilterKind
	Channel    *model.Channel
	// NoAvailableChannel marks the "no channel for this group and model"
	// outcome so the distributor can name the claiming task plugin.
	NoAvailableChannel bool
}

// SelectChannelForRequest resolves the channel for one attempt with the rules
// shared by the HTTP distributor and the Responses WebSocket relay: a pinned
// channel wins, then session affinity (first attempt only), then a random
// eligible channel; every candidate must satisfy the request's channel
// filters. The group the channel was chosen from is returned for auto-group
// callers. The caller still applies SetupContextForSelectedChannel.
func SelectChannelForRequest(c *gin.Context, modelName string, retry *RetryParam) (*model.Channel, string, *ChannelSelectError) {
	constraints := GetChannelConstraints(c)
	if pin, found, overridden := constraints.ResolvedPin(); found {
		for _, lost := range overridden {
			logger.LogWarn(c, common.LogText(
				"channel pin overridden: winning_source=%s winning_channel_id=%d overridden_source=%s overridden_channel_id=%d",
				pin.Source, pin.ChannelId, lost.Source, lost.ChannelId,
			))
		}
		channel, err := model.CacheGetChannel(pin.ChannelId)
		if err != nil {
			return nil, "", pinnedChannelUnavailable(pin, http.StatusBadRequest, i18n.MsgDistributorInvalidChannelId)
		}
		if channel.Status != common.ChannelStatusEnabled {
			return nil, "", pinnedChannelUnavailable(pin, http.StatusForbidden, i18n.MsgDistributorChannelDisabled)
		}
		if ok, kind := model.ChannelSatisfiesFilters(channel, modelName, constraints.Filters); !ok {
			return nil, "", &ChannelSelectError{
				StatusCode: http.StatusBadRequest, Code: types.ErrorCode(kind), MessageID: i18n.MsgDistributorNoAvailableChannel,
				Params:     map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelName},
				FilterKind: kind, Channel: channel,
			}
		}
		return channel, "", nil
	}

	usingGroup := retry.TokenGroup
	var channel *model.Channel
	var selectGroup string
	autoGroupIdx := 0
	if lastGroupIndex, exists := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := lastGroupIndex.(int); ok {
			autoGroupIdx = idx
		}
	}
	isInitialAttempt := retry.GetRetry() == 0 &&
		autoGroupIdx == 0 &&
		!common.GetContextKeyBool(c, constant.ContextKeyChannelCooldownBypassed) &&
		len(usedChannelIDs(c)) == 0 &&
		len(cooldownRoundTriedIDs(c)) == 0
	if isInitialAttempt {
		if preferredChannelID, found := GetPreferredChannelByAffinity(c, modelName, usingGroup); found {
			strictSession := RequestPolicy(c).SessionMode == "strict"
			affinityUsable := false
			preferred, err := model.CacheGetChannel(preferredChannelID)
			affinitySatisfied := false
			preferredCooling := false
			if err == nil && preferred != nil && preferred.Status == common.ChannelStatusEnabled {
				affinitySatisfied, _ = model.ChannelSatisfiesFilters(preferred, modelName, constraints.Filters)
				if affinitySatisfied && !strictSession && model.IsChannelModelCooling(preferred.Id, modelName) {
					preferredCooling = true
					affinitySatisfied = false
				}
			}
			if affinitySatisfied {
				if usingGroup == "auto" {
					userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
					for _, g := range GetRequestAutoGroups(c, userGroup) {
						if model.IsChannelEnabledForGroupModel(g, modelName, preferred.Id) {
							selectGroup = g
							common.SetContextKey(c, constant.ContextKeyAutoGroup, g)
							channel = preferred
							affinityUsable = true
							MarkChannelAffinityUsed(c, g, preferred.Id)
							break
						}
					}
				} else if model.IsChannelEnabledForGroupModel(usingGroup, modelName, preferred.Id) {
					channel = preferred
					selectGroup = usingGroup
					affinityUsable = true
					MarkChannelAffinityUsed(c, usingGroup, preferred.Id)
				}
			}
			if !affinityUsable && !preferredCooling && !ShouldKeepChannelAffinityOnChannelDisabled() {
				ClearCurrentChannelAffinityCache(c)
			}
			if !affinityUsable && strictSession {
				return nil, "", &ChannelSelectError{StatusCode: http.StatusServiceUnavailable, Message: "strict_session_binding_unavailable"}
			}
		}
	}

	if channel == nil {
		var err error
		channel, selectGroup, err = CacheGetRandomSatisfiedChannel(retry)
		if errors.Is(err, ErrChannelsCoolingDown) {
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeGetChannelFailed, MessageID: i18n.MsgChannelsCoolingDown,
				Params: map[string]any{"Group": selectGroup, "Model": modelName},
			}
		}
		if err != nil {
			showGroup := usingGroup
			if usingGroup == "auto" {
				showGroup = fmt.Sprintf("auto(%s)", selectGroup)
			}
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorGetChannelFailed,
				Params: map[string]any{"Group": showGroup, "Model": modelName, "Error": err.Error()},
			}
		}
		if channel == nil {
			return nil, selectGroup, &ChannelSelectError{
				StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
				Params: map[string]any{"Group": usingGroup, "Model": modelName}, NoAvailableChannel: true,
			}
		}
	}
	if ok, kind := model.ChannelSatisfiesFilters(channel, modelName, constraints.Filters); !ok {
		return nil, selectGroup, &ChannelSelectError{
			StatusCode: http.StatusServiceUnavailable, Code: types.ErrorCodeModelNotFound, MessageID: i18n.MsgDistributorNoAvailableChannel,
			Params:     map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelName},
			FilterKind: kind, Channel: channel, NoAvailableChannel: true,
		}
	}
	return channel, selectGroup, nil
}

// Origin-task pins report a fixed code so task polling can tell a retired
// channel from a malformed request.
func pinnedChannelUnavailable(pin dto.ChannelPin, statusCode int, messageID string) *ChannelSelectError {
	if pin.Source == dto.PinSourceOriginTask {
		return &ChannelSelectError{StatusCode: http.StatusBadRequest, Code: "origin_task_channel_disabled", Message: "origin_task_channel_disabled"}
	}
	return &ChannelSelectError{StatusCode: statusCode, MessageID: messageID}
}

// AppendUsedChannel records an attempted channel in the request's channel
// trail, which the retry log and the consume log's admin_info both read.
func AppendUsedChannel(c *gin.Context, channelID int) {
	c.Set("use_channel", append(c.GetStringSlice("use_channel"), fmt.Sprintf("%d", channelID)))
}
