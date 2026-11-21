package service

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
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

const (
	redisActiveRelayHashKey = "newapi:active_relay_requests"
	clusterActiveStaleMs    = 60000
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

type clusterActiveRelaySnapshot struct {
	NodeID            string  `json:"node_id"`
	Seq               int64   `json:"seq"`
	UpdatedAtMs       int64   `json:"updated_at_ms"`
	StartTimeMs       int64   `json:"start_time_ms"`
	FirstResponseMs   int64   `json:"first_response_ms,omitempty"`
	HasFirstResponse  bool    `json:"has_first_response,omitempty"`
	RequestId         string  `json:"request_id"`
	UserId            int     `json:"user_id"`
	Username          string  `json:"username"`
	TokenId           int     `json:"token_id"`
	TokenName         string  `json:"token_name"`
	ModelName         string  `json:"model_name"`
	ChannelId         int     `json:"channel_id"`
	Group             string  `json:"group"`
	Ip                string  `json:"ip,omitempty"`
	RequestPath       string  `json:"request_path,omitempty"`
	RequestBodySize   int64   `json:"request_body_size,omitempty"`
	PromptTokens      int     `json:"prompt_tokens"`
	CompletionTokens  int     `json:"completion_tokens"`
	IsStream          bool    `json:"is_stream"`
	ReasoningEffort   string  `json:"reasoning_effort,omitempty"`
	ModelRatio        float64 `json:"model_ratio,omitempty"`
	GroupRatio        float64 `json:"group_ratio,omitempty"`
	CompletionRatio   float64 `json:"completion_ratio,omitempty"`
	ModelPrice        float64 `json:"model_price,omitempty"`
}

var (
	activeRelayRequests   sync.Map
	activeRelaySeq        atomic.Int64
	activeRelayCount      atomic.Int64
	clusterSyncLoopActive atomic.Bool
	localActiveNodeID     = common.NewRequestId()
)

func clusterFieldKey(nodeID string, seq int64) string {
	return fmt.Sprintf("%s:%d", nodeID, seq)
}

func stableNegativeDisplayID(nodeID string, seq int64) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(nodeID))
	val := int((int64(h.Sum32()&0x3fffffff) ^ (seq & 0x3fffffff)) & 0x3fffffff)
	if val <= 0 {
		val = int(seq & 0x3fffffff)
		if val <= 0 {
			val = 1
		}
	}
	return -val
}

func buildClusterSnapshot(entry *activeRelayEntry, now time.Time) clusterActiveRelaySnapshot {
	info := entry.info
	group := info.UsingGroup
	if group == "" {
		group = info.TokenGroup
	}
	if group == "" {
		group = info.UserGroup
	}
	startMs := info.StartTime.UnixMilli()
	if startMs <= 0 {
		startMs = now.UnixMilli()
	}
	var firstRespMs int64
	hasFirst := info.HasSendResponse()
	if hasFirst {
		firstRespMs = info.FirstResponseTime.UnixMilli()
	}
	return clusterActiveRelaySnapshot{
		NodeID:           localActiveNodeID,
		Seq:              entry.seq,
		UpdatedAtMs:      now.UnixMilli(),
		StartTimeMs:      startMs,
		FirstResponseMs:  firstRespMs,
		HasFirstResponse: hasFirst,
		RequestId:        entry.requestId,
		UserId:           entry.userId,
		Username:         entry.username,
		TokenId:          entry.tokenId,
		TokenName:        entry.tokenName,
		ModelName:        info.OriginModelName,
		ChannelId:        info.GetChannelID(),
		Group:            group,
		Ip:               entry.ip,
		RequestPath:      entry.requestPath,
		RequestBodySize:  info.RequestBodySize,
		PromptTokens:     info.GetLivePromptTokens(),
		CompletionTokens: info.GetLiveCompletionTokens(),
		IsStream:         info.GetIsStream(),
		ReasoningEffort:  info.ReasoningEffort,
		ModelRatio:       info.PriceData.ModelRatio,
		GroupRatio:       info.PriceData.GroupRatioInfo.GroupRatio,
		CompletionRatio:  info.PriceData.CompletionRatio,
		ModelPrice:       info.PriceData.ModelPrice,
	}
}

func syncLocalActiveEntriesToRedis() {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	now := time.Now()
	fields := make(map[string]any)
	activeRelayRequests.Range(func(_, value any) bool {
		entry, ok := value.(*activeRelayEntry)
		if !ok || entry == nil || entry.info == nil {
			return true
		}
		snap := buildClusterSnapshot(entry, now)
		if raw, err := common.Marshal(snap); err == nil {
			fields[clusterFieldKey(localActiveNodeID, entry.seq)] = string(raw)
		}
		return true
	})
	if len(fields) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	pipe := common.RDB.Pipeline()
	pipe.HSet(ctx, redisActiveRelayHashKey, fields)
	pipe.Expire(ctx, redisActiveRelayHashKey, 90*time.Second)
	_, _ = pipe.Exec(ctx)
}

func removeLocalActiveEntryFromRedis(seq int64) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = common.RDB.HDel(ctx, redisActiveRelayHashKey, clusterFieldKey(localActiveNodeID, seq)).Err()
}

func ensureClusterActiveSyncLoop() {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	if !clusterSyncLoopActive.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer clusterSyncLoopActive.Store(false)
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if activeRelayCount.Load() <= 0 || !common.RedisEnabled || common.RDB == nil {
				return
			}
			syncLocalActiveEntriesToRedis()
		}
	}()
}

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
	syncLocalActiveEntriesToRedis()
	ensureClusterActiveSyncLoop()
	model.NotifyLogActivity()

	var once sync.Once
	return func() {
		once.Do(func() {
			activeRelayRequests.Delete(seq)
			activeRelayCount.Add(-1)
			removeLocalActiveEntryFromRedis(seq)
			model.NotifyLogActivity()
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

func formatClusterActiveSnapshot(
	snap clusterActiveRelaySnapshot,
	now time.Time,
	filter model.LogFilter,
	userId int,
	isAdmin bool,
) *model.Log {
	if userId > 0 && snap.UserId != userId {
		return nil
	}

	createdAt := snap.StartTimeMs / 1000
	if createdAt <= 0 {
		createdAt = now.Unix()
	}
	if filter.StartTimestamp != 0 && createdAt < filter.StartTimestamp {
		return nil
	}
	if filter.EndTimestamp != 0 && createdAt > filter.EndTimestamp {
		return nil
	}

	if !matchesActiveLogTextFilter(snap.ModelName, filter.ModelName) {
		return nil
	}
	if !matchesActiveLogTextFilter(snap.Username, filter.Username) {
		return nil
	}
	if filter.TokenName != "" && snap.TokenName != filter.TokenName {
		return nil
	}
	if filter.RequestId != "" && snap.RequestId != filter.RequestId {
		return nil
	}
	if filter.Channel != 0 && snap.ChannelId != filter.Channel {
		return nil
	}
	if filter.Group != "" && snap.Group != filter.Group {
		return nil
	}

	elapsedMs := max(now.UnixMilli()-snap.StartTimeMs, 0)
	useTimeSec := int(elapsedMs / 1000)

	otherMap := map[string]any{
		"in_progress": true,
		"elapsed_ms":  elapsedMs,
	}
	if snap.RequestPath != "" {
		otherMap["request_path"] = snap.RequestPath
	}
	if snap.RequestBodySize > 0 {
		otherMap["request_body_size"] = snap.RequestBodySize
	}
	if snap.HasFirstResponse {
		otherMap["frt"] = float64(max(snap.FirstResponseMs-snap.StartTimeMs, 0))
	}
	if snap.IsStream && snap.CompletionTokens > 0 && elapsedMs > 0 {
		genMs := elapsedMs
		if snap.HasFirstResponse {
			if sinceFirst := now.UnixMilli() - snap.FirstResponseMs; sinceFirst >= 100 {
				genMs = sinceFirst
			}
		}
		elapsedSec := max(float64(genMs)/1000.0, 0.1)
		liveTps := math.Round((float64(snap.CompletionTokens)/elapsedSec)*10) / 10
		otherMap["live_tps"] = liveTps
	}
	if snap.ReasoningEffort != "" {
		otherMap["reasoning_effort"] = snap.ReasoningEffort
	}
	if snap.ModelRatio > 0 {
		otherMap["model_ratio"] = snap.ModelRatio
	}
	if snap.GroupRatio > 0 {
		otherMap["group_ratio"] = snap.GroupRatio
	}
	if snap.CompletionRatio > 0 {
		otherMap["completion_ratio"] = snap.CompletionRatio
	}
	if snap.ModelPrice > 0 {
		otherMap["model_price"] = snap.ModelPrice
	}

	otherBytes, _ := common.Marshal(otherMap)

	channelName := ""
	if isAdmin && snap.ChannelId > 0 && model.DB != nil {
		if ch, err := model.CacheGetChannel(snap.ChannelId); err == nil && ch != nil {
			channelName = ch.Name
		}
	}

	displayID := -int(snap.Seq)
	if snap.NodeID != "" && snap.NodeID != localActiveNodeID {
		displayID = stableNegativeDisplayID(snap.NodeID, snap.Seq)
	}

	logRow := &model.Log{
		Id:               displayID,
		UserId:           snap.UserId,
		CreatedAt:        createdAt,
		Type:             model.LogTypeConsume,
		Content:          "",
		Username:         snap.Username,
		TokenName:        snap.TokenName,
		ModelName:        snap.ModelName,
		Quota:            0,
		PromptTokens:     snap.PromptTokens,
		CompletionTokens: snap.CompletionTokens,
		UseTime:          useTimeSec,
		IsStream:         snap.IsStream,
		ChannelId:        snap.ChannelId,
		ChannelName:      channelName,
		TokenId:          snap.TokenId,
		Group:            snap.Group,
		Ip:               snap.Ip,
		RequestId:        snap.RequestId,
		Other:            string(otherBytes),
	}
	if !isAdmin {
		logRow.ChannelId = 0
	}
	return logRow
}

// GetActiveRelayLogs returns currently running relay requests formatted as Log
// entries matching the given filter and viewer scope, merging local in-flight
// requests with remote nodes' active snapshots when Redis is enabled.
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
	seenRequests := make(map[string]struct{})

	activeRelayRequests.Range(func(_, value any) bool {
		entry, ok := value.(*activeRelayEntry)
		if !ok || entry == nil || entry.info == nil {
			return true
		}
		snap := buildClusterSnapshot(entry, now)
		if snap.RequestId != "" {
			seenRequests[fmt.Sprintf("%d:%s", snap.UserId, snap.RequestId)] = struct{}{}
		}
		if logRow := formatClusterActiveSnapshot(snap, now, filter, userId, isAdmin); logRow != nil {
			matched = append(matched, matchedItem{seq: entry.seq, log: logRow})
		}
		return true
	})

	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		rawEntries, err := common.RDB.HGetAll(ctx, redisActiveRelayHashKey).Result()
		cancel()
		if err == nil && len(rawEntries) > 0 {
			nowMs := now.UnixMilli()
			var staleFields []string
			for field, rawJSON := range rawEntries {
				var snap clusterActiveRelaySnapshot
				if err := common.UnmarshalJsonStr(rawJSON, &snap); err != nil {
					continue
				}
				if nowMs-snap.UpdatedAtMs > clusterActiveStaleMs {
					staleFields = append(staleFields, field)
					continue
				}
				if snap.NodeID == localActiveNodeID {
					continue
				}
				if snap.RequestId != "" {
					reqKey := fmt.Sprintf("%d:%s", snap.UserId, snap.RequestId)
					if _, exists := seenRequests[reqKey]; exists {
						continue
					}
					seenRequests[reqKey] = struct{}{}
				}
				if logRow := formatClusterActiveSnapshot(snap, now, filter, userId, isAdmin); logRow != nil {
					matched = append(matched, matchedItem{seq: snap.Seq, log: logRow})
				}
			}
			if len(staleFields) > 0 {
				cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
				_ = common.RDB.HDel(cleanCtx, redisActiveRelayHashKey, staleFields...).Err()
				cleanCancel()
			}
		}
	}

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
