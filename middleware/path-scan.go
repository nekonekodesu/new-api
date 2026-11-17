package middleware

import (
	"cmp"
	"context"
	"net/http"
	"net/netip"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

const (
	pathScanSweepInterval         = 30 * time.Second
	pathScanRedisSyncInterval     = 2 * time.Second
	pathScanRedisOperationTimeout = 500 * time.Millisecond
	pathScanMaxTrackedAttempts    = 50000
	pathScanMaxTrackedBans        = 50000

	redisPathScanBansHashKey   = "path_scan:bans"
	redisPathScanAttemptPrefix = "path_scan:attempt:"
)

type SecurityBan struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	BannedAt  time.Time `json:"banned_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pathScanAttempt struct {
	count       int
	windowStart time.Time
}

var pathScanState = struct {
	sync.Mutex
	attempts      map[string]pathScanAttempt
	bans          map[string]SecurityBan
	lastSweep     time.Time
	lastRedisSync time.Time
}{
	attempts: make(map[string]pathScanAttempt),
	bans:     make(map[string]SecurityBan),
}

func PathScanProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		setting := system_setting.GetPathScanSetting()
		if !setting.Enabled {
			c.Next()
			return
		}

		reqPath := c.Request.URL.Path
		protectedMgmt := isProtectedManagementAPIPath(reqPath)

		ip := normalizeBanIP(common.GetClientIP(c))
		now := time.Now()
		window := time.Duration(setting.WindowSeconds) * time.Second

		maybeSyncPathScanBansFromRedis(c.Request.Context(), now, false)

		if ip != "" {
			pathScanState.Lock()
			maybeSweepPathScanStateLocked(now, window)
			ban, banned := pathScanState.bans[ip]
			if banned && !now.Before(ban.ExpiresAt) {
				delete(pathScanState.bans, ip)
				banned = false
			}
			pathScanState.Unlock()
			if banned && !protectedMgmt {
				setPathScanNoStoreHeaders(c)
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}

		if !protectedMgmt && system_setting.MatchDirectPathScan(reqPath) {
			setPathScanNoStoreHeaders(c)
			if ip != "" {
				recordPathScanBan(
					c.Request.Context(),
					ip,
					"Directly block paths",
					now,
					system_setting.PathScanBanDuration(setting),
				)
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		// Gin resolves route parameters before global middleware runs. An empty
		// route pattern (or catch-all wildcard pattern) identifies probe paths
		// that would otherwise enter the SPA fallback or wildcard handlers.
		if !protectedMgmt && isCountableScanRoute(c.FullPath()) && system_setting.MatchCountedPathScan(reqPath) {
			setPathScanNoStoreHeaders(c)
			shouldBan := setting.MaxRequests <= 1
			if ip != "" && !shouldBan {
				shouldBan = incrementPathScanAttempt(c.Request.Context(), ip, now, window, setting.WindowSeconds, setting.MaxRequests)
			}
			if shouldBan {
				if ip != "" {
					recordPathScanBan(
						c.Request.Context(),
						ip,
						"Dictionary path scanning",
						now,
						system_setting.PathScanBanDuration(setting),
					)
				}
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Next()
	}
}

func setPathScanNoStoreHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
}

func isCountableScanRoute(fullPath string) bool {
	return fullPath == "" || strings.Contains(fullPath, "*")
}

func normalizeBanIP(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if addr, err := netip.ParseAddr(trimmed); err == nil {
		return addr.Unmap().WithZone("").String()
	}
	return trimmed
}

func incrementPathScanAttempt(ctx context.Context, ip string, now time.Time, window time.Duration, windowSeconds int, maxRequests int) bool {
	if common.RedisEnabled && common.RDB != nil && windowSeconds > 0 && maxRequests > 1 {
		redisCtx, cancel := context.WithTimeout(ctx, pathScanRedisOperationTimeout)
		_, count, _, err := redisFixedWindowTake(
			redisCtx,
			redisPathScanAttemptPrefix+ip,
			maxRequests,
			int64(windowSeconds),
		)
		cancel()
		if err == nil {
			return int(count) >= maxRequests
		}
	}

	pathScanState.Lock()
	defer pathScanState.Unlock()
	maybeSweepPathScanStateLocked(now, window)
	attempt := pathScanState.attempts[ip]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= window {
		attempt = pathScanAttempt{windowStart: now}
	}
	attempt.count++
	if len(pathScanState.attempts) >= pathScanMaxTrackedAttempts {
		if _, exists := pathScanState.attempts[ip]; !exists {
			evictOneAttemptLocked()
		}
	}
	pathScanState.attempts[ip] = attempt
	return attempt.count >= maxRequests
}

func recordPathScanBan(ctx context.Context, ip string, reason string, now time.Time, duration time.Duration) {
	ban := SecurityBan{
		IP:        ip,
		Reason:    reason,
		BannedAt:  now,
		ExpiresAt: now.Add(duration),
	}

	pathScanState.Lock()
	delete(pathScanState.attempts, ip)
	if len(pathScanState.bans) >= pathScanMaxTrackedBans {
		if _, exists := pathScanState.bans[ip]; !exists {
			evictOneBanLocked()
		}
	}
	pathScanState.bans[ip] = ban
	pathScanState.Unlock()

	if common.RedisEnabled && common.RDB != nil {
		if payload, err := common.Marshal(ban); err == nil {
			redisCtx, cancel := context.WithTimeout(ctx, pathScanRedisOperationTimeout)
			_ = common.RDB.HSet(redisCtx, redisPathScanBansHashKey, ip, string(payload)).Err()
			_ = common.RDB.Del(redisCtx, redisPathScanAttemptPrefix+ip).Err()
			cancel()
		}
	}
}

func maybeSyncPathScanBansFromRedis(ctx context.Context, now time.Time, force bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	pathScanState.Lock()
	if !force && !pathScanState.lastRedisSync.IsZero() && now.Sub(pathScanState.lastRedisSync) < pathScanRedisSyncInterval {
		pathScanState.Unlock()
		return
	}
	pathScanState.lastRedisSync = now
	pathScanState.Unlock()

	redisCtx, cancel := context.WithTimeout(ctx, pathScanRedisOperationTimeout)
	entries, err := common.RDB.HGetAll(redisCtx, redisPathScanBansHashKey).Result()
	cancel()
	if err != nil {
		return
	}

	var expiredIPs []string
	remoteBans := make(map[string]SecurityBan, len(entries))
	for ip, raw := range entries {
		var ban SecurityBan
		if err := common.UnmarshalJsonStr(raw, &ban); err != nil {
			expiredIPs = append(expiredIPs, ip)
			continue
		}
		if !now.Before(ban.ExpiresAt) {
			expiredIPs = append(expiredIPs, ip)
			continue
		}
		if ban.IP == "" {
			ban.IP = ip
		}
		remoteBans[ban.IP] = ban
	}

	if len(expiredIPs) > 0 {
		cleanCtx, cleanCancel := context.WithTimeout(ctx, pathScanRedisOperationTimeout)
		_ = common.RDB.HDel(cleanCtx, redisPathScanBansHashKey, expiredIPs...).Err()
		cleanCancel()
	}

	pathScanState.Lock()
	pathScanState.bans = remoteBans
	pathScanState.Unlock()
}

func pathMatchesAny(rawPath string, rules []string) bool {
	return system_setting.MatchPathScanRules(rawPath, rules)
}

// isProtectedManagementAPIPath prevents operators from locking themselves out
// of system configuration, login, or ban management if an overly broad rule
// such as "/*" or "/api*" is saved in DirectPaths.
func isProtectedManagementAPIPath(rawPath string) bool {
	if rawPath == "" {
		return false
	}
	if !strings.HasPrefix(rawPath, "/") {
		rawPath = "/" + rawPath
	}
	cleanLower := strings.ToLower(path.Clean(rawPath))
	switch cleanLower {
	case "/api/option", "/api/status", "/api/setup",
		"/api/user/login", "/api/user/logout", "/api/user/self":
		return true
	}
	return strings.HasPrefix(cleanLower, "/api/option/") ||
		strings.HasPrefix(cleanLower, "/api/network-security/") ||
		strings.HasPrefix(cleanLower, "/api/user/login/") ||
		strings.HasPrefix(cleanLower, "/api/user/auth/") ||
		strings.HasPrefix(cleanLower, "/api/user/passkey/")
}

func maybeSweepPathScanStateLocked(now time.Time, window time.Duration) {
	if !pathScanState.lastSweep.IsZero() && now.Sub(pathScanState.lastSweep) < pathScanSweepInterval {
		return
	}
	pathScanState.lastSweep = now
	for ip, ban := range pathScanState.bans {
		if !now.Before(ban.ExpiresAt) {
			delete(pathScanState.bans, ip)
		}
	}
	for ip, attempt := range pathScanState.attempts {
		if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= window {
			delete(pathScanState.attempts, ip)
		}
	}
}

func evictOneAttemptLocked() {
	var oldestIP string
	var oldestStart time.Time
	for ip, attempt := range pathScanState.attempts {
		if oldestIP == "" || attempt.windowStart.Before(oldestStart) {
			oldestIP = ip
			oldestStart = attempt.windowStart
		}
	}
	if oldestIP != "" {
		delete(pathScanState.attempts, oldestIP)
	}
}

func evictOneBanLocked() {
	var earliestIP string
	var earliestExpiry time.Time
	for ip, ban := range pathScanState.bans {
		if earliestIP == "" || ban.ExpiresAt.Before(earliestExpiry) {
			earliestIP = ip
			earliestExpiry = ban.ExpiresAt
		}
	}
	if earliestIP != "" {
		delete(pathScanState.bans, earliestIP)
	}
}

func GetActivePathScanBans() []SecurityBan {
	now := time.Now()
	maybeSyncPathScanBansFromRedis(context.Background(), now, true)

	pathScanState.Lock()
	active := make([]SecurityBan, 0, len(pathScanState.bans))
	for ip, ban := range pathScanState.bans {
		if !now.Before(ban.ExpiresAt) {
			delete(pathScanState.bans, ip)
			continue
		}
		active = append(active, ban)
	}
	pathScanState.Unlock()

	slices.SortFunc(active, func(a, b SecurityBan) int {
		if c := b.BannedAt.Compare(a.BannedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.IP, b.IP)
	})
	return active
}

// DeletePathScanBan removes a specific IP ban (or clears all active bans when ip is empty)
// and resets tracked scan attempt counters for the affected IP(s).
func DeletePathScanBan(ip string) int {
	trimmed := normalizeBanIP(ip)
	maybeSyncPathScanBansFromRedis(context.Background(), time.Now(), true)

	pathScanState.Lock()
	removed := 0
	if trimmed == "" {
		removed = len(pathScanState.bans)
		clear(pathScanState.bans)
		clear(pathScanState.attempts)
	} else {
		if _, exists := pathScanState.bans[trimmed]; exists {
			delete(pathScanState.bans, trimmed)
			removed = 1
		}
		delete(pathScanState.attempts, trimmed)
	}
	pathScanState.Unlock()

	if common.RedisEnabled && common.RDB != nil {
		redisCtx, cancel := context.WithTimeout(context.Background(), pathScanRedisOperationTimeout)
		if trimmed == "" {
			_ = common.RDB.Del(redisCtx, redisPathScanBansHashKey).Err()
		} else {
			_ = common.RDB.HDel(redisCtx, redisPathScanBansHashKey, trimmed).Err()
			_ = common.RDB.Del(redisCtx, redisPathScanAttemptPrefix+trimmed).Err()
		}
		cancel()
	}
	return removed
}
