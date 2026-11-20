package service

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type activeRelayEntry struct {
	seq         int64
	requestId   string
	userId      int
	username    string
	tokenId     int
	tokenName   string
	requestPath string
	ip          string
	info        *relaycommon.RelayInfo
}

var (
	activeRelayRequests sync.Map
	activeRelaySeq      atomic.Int64
	activeRelayCount    atomic.Int64
)

// HasActiveRelayRequests reports whether any relay request is currently in flight.
func HasActiveRelayRequests() bool {
	return activeRelayCount.Load() > 0
}

// RegisterActiveRelayRequest registers an in-flight relay request so Usage Logs
// can display it while it is still executing, and returns an idempotent cleanup
// function to call via defer when the request finishes.
func RegisterActiveRelayRequest(c *gin.Context, info *relaycommon.RelayInfo) func() {
	if info == nil {
		return func() {}
	}
	seq := activeRelaySeq.Add(1)
	activeRelayCount.Add(1)
	reqId := info.RequestId
	if reqId == "" && c != nil {
		reqId = c.GetString(common.RequestIdKey)
	}
	if reqId == "" {
		reqId = common.NewRequestId()
	}

	username := ""
	tokenName := ""
	requestPath := ""
	ip := ""
	if c != nil {
		username = c.GetString("username")
		if username == "" {
			username = common.GetContextKeyString(c, constant.ContextKeyUserName)
		}
		tokenName = c.GetString("token_name")
		if c.Request != nil && c.Request.URL != nil {
			requestPath = c.Request.URL.Path
		}
		if info.UserSetting.RecordIpLog {
			ip = c.ClientIP()
		}
	}
	if username == "" && info.UserId > 0 && model.DB != nil {
		username, _ = model.GetUsernameById(info.UserId, false)
	}
	if requestPath == "" && info.RequestURLPath != "" {
		requestPath = info.RequestURLPath
		if idx := strings.Index(requestPath, "?"); idx != -1 {
			requestPath = requestPath[:idx]
		}
	}

	entry := &activeRelayEntry{
		seq:         seq,
		requestId:   reqId,
		userId:      info.UserId,
		username:    username,
		tokenId:     info.TokenId,
		tokenName:   tokenName,
		requestPath: requestPath,
		ip:          ip,
		info:        info,
	}
	activeRelayRequests.Store(seq, entry)

	var once sync.Once
	return func() {
		once.Do(func() {
			activeRelayRequests.Delete(seq)
			activeRelayCount.Add(-1)
		})
	}
}

func matchesActiveLogTextFilter(actual string, pattern string) bool {
	if pattern == "" {
		return true
	}
	if !strings.Contains(pattern, "%") {
		return actual == pattern
	}
	trimmed := strings.Trim(pattern, "%")
	if trimmed == "" {
		return true
	}
	return strings.Contains(strings.ToLower(actual), strings.ToLower(trimmed))
}

// GetActiveRelayLogs returns currently running relay requests formatted as Log
// entries matching the given filter and viewer scope.
func GetActiveRelayLogs(filter model.LogFilter, userId int, isAdmin bool) []*model.Log {
	if filter.Type != model.LogTypeUnknown && filter.Type != model.LogTypeConsume {
		return nil
	}

	now := time.Now()
	type matchedItem struct {
		seq int64
		log *model.Log
	}
	var matched []matchedItem

	activeRelayRequests.Range(func(_, value any) bool {
		entry, ok := value.(*activeRelayEntry)
		if !ok || entry == nil || entry.info == nil {
			return true
		}
		info := entry.info
		if userId > 0 && entry.userId != userId {
			return true
		}

		createdAt := info.StartTime.Unix()
		if createdAt <= 0 {
			createdAt = now.Unix()
		}
		if filter.StartTimestamp != 0 && createdAt < filter.StartTimestamp {
			return true
		}
		if filter.EndTimestamp != 0 && createdAt > filter.EndTimestamp {
			return true
		}

		modelName := info.OriginModelName
		if !matchesActiveLogTextFilter(modelName, filter.ModelName) {
			return true
		}
		if !matchesActiveLogTextFilter(entry.username, filter.Username) {
			return true
		}
		if filter.TokenName != "" && entry.tokenName != filter.TokenName {
			return true
		}
		if filter.RequestId != "" && entry.requestId != filter.RequestId {
			return true
		}

		channelId := info.GetChannelID()
		if filter.Channel != 0 && channelId != filter.Channel {
			return true
		}

		group := info.UsingGroup
		if group == "" {
			group = info.TokenGroup
		}
		if group == "" {
			group = info.UserGroup
		}
		if filter.Group != "" && group != filter.Group {
			return true
		}

		promptTokens := info.GetLivePromptTokens()
		completionTokens := info.GetLiveCompletionTokens()
		isStream := info.GetIsStream()

		elapsedMs := max(now.Sub(info.StartTime).Milliseconds(), 0)
		useTimeSec := int(elapsedMs / 1000)

		otherMap := map[string]any{
			"in_progress": true,
			"elapsed_ms":  elapsedMs,
		}
		if entry.requestPath != "" {
			otherMap["request_path"] = entry.requestPath
		}
		if info.RequestBodySize > 0 {
			otherMap["request_body_size"] = info.RequestBodySize
		}
		if info.HasSendResponse() {
			otherMap["frt"] = float64(max(info.FirstResponseTime.Sub(info.StartTime).Milliseconds(), 0))
		}
		if isStream && completionTokens > 0 && elapsedMs > 0 {
			// Compute real-time average TPS. If first token time is available,
			// prefer streaming generation duration if >= 100ms, otherwise total elapsed.
			genMs := elapsedMs
			if info.HasSendResponse() {
				if sinceFirst := now.Sub(info.FirstResponseTime).Milliseconds(); sinceFirst >= 100 {
					genMs = sinceFirst
				}
			}
			elapsedSec := max(float64(genMs)/1000.0, 0.1)
			liveTps := math.Round((float64(completionTokens)/elapsedSec)*10) / 10
			otherMap["live_tps"] = liveTps
		}
		if info.ReasoningEffort != "" {
			otherMap["reasoning_effort"] = info.ReasoningEffort
		}
		if info.PriceData.ModelRatio > 0 {
			otherMap["model_ratio"] = info.PriceData.ModelRatio
		}
		if info.PriceData.GroupRatioInfo.GroupRatio > 0 {
			otherMap["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
		}
		if info.PriceData.CompletionRatio > 0 {
			otherMap["completion_ratio"] = info.PriceData.CompletionRatio
		}
		if info.PriceData.ModelPrice > 0 {
			otherMap["model_price"] = info.PriceData.ModelPrice
		}

		otherBytes, _ := common.Marshal(otherMap)

		channelName := ""
		if isAdmin && channelId > 0 && model.DB != nil {
			if ch, err := model.CacheGetChannel(channelId); err == nil && ch != nil {
				channelName = ch.Name
			}
		}

		logRow := &model.Log{
			Id:               -int(entry.seq),
			UserId:           entry.userId,
			CreatedAt:        createdAt,
			Type:             model.LogTypeConsume,
			Content:          "",
			Username:         entry.username,
			TokenName:        entry.tokenName,
			ModelName:        modelName,
			Quota:            0,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			UseTime:          useTimeSec,
			IsStream:         isStream,
			ChannelId:        channelId,
			ChannelName:      channelName,
			TokenId:          entry.tokenId,
			Group:            group,
			Ip:               entry.ip,
			RequestId:        entry.requestId,
			Other:            string(otherBytes),
		}
		if !isAdmin {
			logRow.ChannelId = 0
		}
		matched = append(matched, matchedItem{seq: entry.seq, log: logRow})
		return true
	})

	slices.SortFunc(matched, func(a, b matchedItem) int {
		if c := cmp.Compare(b.log.CreatedAt, a.log.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.seq, a.seq)
	})

	result := make([]*model.Log, len(matched))
	for i := range matched {
		result[i] = matched[i].log
	}
	return result
}
