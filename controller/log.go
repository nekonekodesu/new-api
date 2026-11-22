package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

func writeLogPageResponse(c *gin.Context, pageInfo *common.PageInfo) {
	payload, err := common.Marshal(gin.H{
		"success": true,
		"message": "",
		"data":    pageInfo,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	sum := sha256.Sum256(payload)
	etag := `W/"` + hex.EncodeToString(sum[:16]) + `"`
	c.Header("Cache-Control", "no-cache")
	c.Header("ETag", etag)
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
}

// logFilterFromQuery reads the log search shared by the log list and the log
// export. Username and channel are read only for administrators.
func logFilterFromQuery(c *gin.Context, admin bool) model.LogFilter {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	afterCreatedAt, _ := strconv.ParseInt(c.Query("after_created_at"), 10, 64)
	afterId, _ := strconv.Atoi(c.Query("after_id"))
	filter := model.LogFilter{
		Type:              logType,
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		AfterCreatedAt:    afterCreatedAt,
		AfterId:           afterId,
		ModelName:         c.Query("model_name"),
		TokenName:         c.Query("token_name"),
		Group:             c.Query("group"),
		RequestId:         c.Query("request_id"),
		UpstreamRequestId: c.Query("upstream_request_id"),
	}
	if admin {
		filter.Username = c.Query("username")
		filter.Channel, _ = strconv.Atoi(c.Query("channel"))
	}
	return filter
}

// limitLogTimeRange returns the start of a log or dashboard search kept within
// the longest log time range. A missing start reaches back the whole range, and
// time after now does not count toward it.
func limitLogTimeRange(startTimestamp int64, endTimestamp int64) (int64, error) {
	maxRange := operation_setting.GetLogSetting().MaxRangeSeconds()
	end := common.GetTimestamp()
	if endTimestamp != 0 {
		end = min(end, endTimestamp)
	}
	if startTimestamp == 0 {
		return end - maxRange, nil
	}
	if end-startTimestamp > maxRange {
		return 0, common.NewMessage("The time range cannot exceed {{days}} days", map[string]any{"days": maxRange / (24 * 60 * 60)})
	}
	return startTimestamp, nil
}

const (
	maxLogLongPollWaitMs      = 15000
	maxLogWatchStreamDuration = 55 * time.Second
)

func WatchAllLogs(c *gin.Context) {
	watchLogActivity(c, 0)
}

func WatchUserLogs(c *gin.Context) {
	userId := c.GetInt("id")
	if userId <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	watchLogActivity(c, userId)
}

func watchLogActivity(c *gin.Context, userId int) {
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	readState := func() (uint64, int) {
		if userId > 0 {
			ver := model.GetClusterUserLogActivityVersion(userId)
			if !common.RedisEnabled || common.RDB == nil {
				if maxID := model.GetLatestPersistedUserLogID(userId); maxID > 0 {
					ver = (ver << 32) | uint64(maxID)
				}
			}
			return ver, service.GetClusterActiveRelayCountForUser(userId)
		}
		ver := model.GetClusterLogActivityVersion()
		if !common.RedisEnabled || common.RDB == nil {
			if maxID := model.GetLatestPersistedLogID(); maxID > 0 {
				ver = (ver << 32) | uint64(maxID)
			}
		}
		return ver, service.GetClusterActiveRelayCount()
	}

	subscribe := func() (<-chan struct{}, uint64) {
		if userId > 0 {
			return model.SubscribeUserLogActivity(userId)
		}
		return model.SubscribeLogActivity()
	}

	writeEvent := func(ver uint64, activeCount int) bool {
		line, err := common.Marshal(gin.H{
			"version":      ver,
			"active_count": activeCount,
		})
		if err != nil {
			return false
		}
		if _, err := c.Writer.Write(append(line, '\n')); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}

	localCh, _ := subscribe()
	lastVer, lastActive := readState()
	if !writeEvent(lastVer, lastActive) {
		return
	}

	ctx := c.Request.Context()
	lifetime := time.NewTimer(maxLogWatchStreamDuration)
	defer lifetime.Stop()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	crossNodeInterval := time.Second
	if common.RedisEnabled && common.RDB != nil {
		crossNodeInterval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(crossNodeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-lifetime.C:
			return
		case <-heartbeat.C:
			if _, err := c.Writer.Write([]byte("\n")); err != nil {
				return
			}
			c.Writer.Flush()
		case <-localCh:
			localCh, _ = subscribe()
			nextVer, nextActive := readState()
			if nextVer != lastVer || nextActive != lastActive {
				lastVer, lastActive = nextVer, nextActive
				if !writeEvent(lastVer, lastActive) {
					return
				}
			}
		case <-ticker.C:
			nextVer, nextActive := readState()
			if nextVer != lastVer || nextActive != lastActive {
				lastVer, lastActive = nextVer, nextActive
				if !writeEvent(lastVer, lastActive) {
					return
				}
			}
		}
	}
}

func parseLogWaitDuration(c *gin.Context) time.Duration {
	waitMs, _ := strconv.Atoi(c.Query("wait_ms"))
	if waitMs <= 0 {
		return 0
	}
	if waitMs > maxLogLongPollWaitMs {
		waitMs = maxLogLongPollWaitMs
	}
	return time.Duration(waitMs) * time.Millisecond
}

func queryDeltaLogsWithBoundedWait(
	ctx context.Context,
	waitDuration time.Duration,
	fetch func() ([]*model.Log, int64, error),
) ([]*model.Log, int64, error) {
	if waitDuration <= 0 {
		return fetch()
	}
	localCh, _ := model.SubscribeLogActivity()
	clusterVer := model.GetClusterLogActivityVersion()

	logs, total, err := fetch()
	if err != nil || len(logs) > 0 {
		return logs, total, err
	}

	deadline := time.NewTimer(waitDuration)
	defer deadline.Stop()

	crossNodeInterval := time.Second
	if common.RedisEnabled && common.RDB != nil {
		crossNodeInterval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(crossNodeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return logs, total, nil
		case <-deadline.C:
			return logs, total, nil
		case <-localCh:
			localCh, _ = model.SubscribeLogActivity()
			clusterVer = model.GetClusterLogActivityVersion()
			logs, total, err = fetch()
			if err != nil || len(logs) > 0 {
				return logs, total, err
			}
		case <-ticker.C:
			if common.RedisEnabled && common.RDB != nil {
				nextVer := model.GetClusterLogActivityVersion()
				if nextVer == clusterVer {
					continue
				}
				clusterVer = nextVer
			}
			logs, total, err = fetch()
			if err != nil || len(logs) > 0 {
				return logs, total, err
			}
		}
	}
}

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	filter := logFilterFromQuery(c, true)
	limit := pageInfo.GetPageSize()
	if filter.AfterCreatedAt > 0 {
		limit = pageInfo.GetPageSize() * 10
	}
	role := c.GetInt("role")
	roleBucket := common.RoleAdminUser
	if role >= common.RoleRootUser {
		roleBucket = common.RoleRootUser
	}

	fetchOnce := func() ([]*model.Log, int64, error) {
		cacheKey := model.BuildLogQueryCacheKey("all", 0, roleBucket, filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
		logs, total, cached := model.GetCachedLogPage(cacheKey)
		if filter.AfterCreatedAt > 0 {
			logs, total, cached = nil, 0, false
		}
		if !cached {
			verBefore := model.GetLogWriteVersion()
			var err error
			logs, total, err = model.GetAllLogs(filter, pageInfo.GetStartIdx(), limit)
			if err != nil {
				return nil, 0, err
			}
			if role < common.RoleRootUser {
				model.FormatAdminLogs(logs)
			} else {
				model.FormatRootLogs(logs)
			}
			if filter.AfterCreatedAt == 0 {
				model.SetCachedLogPage(cacheKey, verBefore, logs, total)
			}
		}
		if pageInfo.GetStartIdx() == 0 {
			activeLogs := service.GetActiveRelayLogs(filter, 0, true)
			if len(activeLogs) > 0 {
				logs = append(activeLogs, logs...)
				total += int64(len(activeLogs))
			}
		}
		return logs, total, nil
	}

	waitDuration := time.Duration(0)
	if filter.AfterCreatedAt > 0 && pageInfo.GetStartIdx() == 0 {
		waitDuration = parseLogWaitDuration(c)
	}
	logs, total, err := queryDeltaLogsWithBoundedWait(c.Request.Context(), waitDuration, fetchOnce)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if filter.AfterCreatedAt > 0 {
		pageInfo.SetTotal(int(total) + pageInfo.GetStartIdx())
	} else {
		pageInfo.SetTotal(int(total))
	}
	pageInfo.SetItems(logs)
	writeLogPageResponse(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	filter := logFilterFromQuery(c, false)
	limit := pageInfo.GetPageSize()
	if filter.AfterCreatedAt > 0 {
		limit = pageInfo.GetPageSize() * 10
	}
	if c.GetInt("role") < common.RoleAdminUser {
		startTimestamp, err := limitLogTimeRange(filter.StartTimestamp, filter.EndTimestamp)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filter.StartTimestamp = startTimestamp
	}
	userId := c.GetInt("id")

	fetchOnce := func() ([]*model.Log, int64, error) {
		cacheKey := model.BuildLogQueryCacheKey("user", userId, common.RoleCommonUser, filter, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
		logs, total, cached := model.GetCachedLogPage(cacheKey)
		if filter.AfterCreatedAt > 0 {
			logs, total, cached = nil, 0, false
		}
		if !cached {
			verBefore := model.GetLogWriteVersion()
			var err error
			logs, total, err = model.GetUserLogs(userId, filter, pageInfo.GetStartIdx(), limit)
			if err != nil {
				return nil, 0, err
			}
			if filter.AfterCreatedAt == 0 {
				model.SetCachedLogPage(cacheKey, verBefore, logs, total)
			}
		}
		if pageInfo.GetStartIdx() == 0 {
			activeLogs := service.GetActiveRelayLogs(filter, userId, false)
			if len(activeLogs) > 0 {
				logs = append(activeLogs, logs...)
				total += int64(len(activeLogs))
			}
		}
		return logs, total, nil
	}

	waitDuration := time.Duration(0)
	if filter.AfterCreatedAt > 0 && pageInfo.GetStartIdx() == 0 {
		waitDuration = parseLogWaitDuration(c)
	}
	logs, total, err := queryDeltaLogsWithBoundedWait(c.Request.Context(), waitDuration, fetchOnce)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if filter.AfterCreatedAt > 0 {
		pageInfo.SetTotal(int(total) + pageInfo.GetStartIdx())
	} else {
		pageInfo.SetTotal(int(total))
	}
	pageInfo.SetItems(logs)
	writeLogPageResponse(c, pageInfo)
	return
}

// ExportAllLogs streams every user's logs matching the search.
func ExportAllLogs(c *gin.Context) {
	exportLogs(c, true)
}

// ExportUserLogs streams the caller's own logs matching the search.
func ExportUserLogs(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser && !operation_setting.GetLogSetting().UserExportEnabled {
		common.ApiErrorT(c, "Log export is disabled")
		return
	}
	exportLogs(c, false)
}

// exportLogs writes one JSON log per line, newest first, and ends with an API
// response object: success with the log count, or the failure. Each user runs
// one export at a time, and every export stays within the longest log time
// range.
func exportLogs(c *gin.Context, all bool) {
	filter := logFilterFromQuery(c, all)
	startTimestamp, err := limitLogTimeRange(filter.StartTimestamp, filter.EndTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	filter.StartTimestamp = startTimestamp

	userId := c.GetInt("id")
	release, ok, err := service.AcquireLogExportLock(userId)
	if err != nil {
		common.SysError(common.LogText("failed to lock log export: %s", err.Error()))
		common.ApiErrorT(c, "Failed to export logs")
		return
	}
	if !ok {
		common.ApiErrorT(c, "Another log export is running. Try again after it finishes.")
		return
	}
	defer release()

	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	count := 0
	writeLog := func(log *model.Log) error {
		line, err := common.Marshal(log)
		if err != nil {
			return err
		}
		if _, err := c.Writer.Write(append(line, '\n')); err != nil {
			return err
		}
		count++
		return nil
	}
	ctx := c.Request.Context()
	if all {
		err = model.StreamAllLogs(ctx, filter, c.GetInt("role"), writeLog)
	} else {
		err = model.StreamUserLogs(ctx, userId, filter, writeLog)
	}
	if ctx.Err() != nil {
		return
	}
	// The status is already sent, so the result object becomes the last line.
	if err != nil {
		common.SysError(common.LogText("failed to export logs: %s", err.Error()))
		common.ApiErrorT(c, "Failed to export logs")
		return
	}
	common.ApiSuccess(c, gin.H{"count": count})
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	common.ApiErrorT(c, "This API is deprecated")
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	common.ApiErrorT(c, "This API is deprecated")
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		common.ApiErrorI18n(c, i18n.MsgTokenInvalid)
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if c.GetInt("role") < common.RoleAdminUser {
		var err error
		if startTimestamp, err = limitLogTimeRange(startTimestamp, endTimestamp); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}
