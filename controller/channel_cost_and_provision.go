package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type upstreamGroupEntry struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Ratio       float64 `json:"ratio"`
}

type upstreamTokenRecord struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Key            string `json:"key"`
	Status         int    `json:"status"`
	Group          string `json:"group"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
	RemainQuota    int64  `json:"remain_quota"`
	UsedQuota      int64  `json:"used_quota"`
}

func isAutoUpstreamGroup(groupName string, ratioRaw string) bool {
	g := strings.TrimSpace(strings.ToLower(groupName))
	if g == "" || g == "auto" || g == "自動" || g == "自动" || g == "all" || g == "reseller" {
		return true
	}
	r := strings.TrimSpace(strings.ToLower(ratioRaw))
	if strings.Contains(r, "动态") || strings.Contains(r, "動態") || strings.Contains(r, "auto") {
		return true
	}
	return false
}

func isLumiyaSyncMode(syncMode string, name string, baseURL string) bool {
	if strings.EqualFold(strings.TrimSpace(syncMode), model.UpstreamSyncModeLumiya) {
		return true
	}
	if strings.TrimSpace(syncMode) != "" {
		return false
	}
	nl := strings.ToLower(name)
	bl := strings.ToLower(baseURL)
	return strings.Contains(nl, "lumiya") ||
		strings.Contains(bl, "lumiya") ||
		strings.Contains(bl, "xn--1ys141f4ks") ||
		strings.Contains(baseURL, "露米娅") ||
		strings.Contains(baseURL, "露米婭")
}

func cleanUpstreamAPIBaseURL(raw string, syncMode string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "露米娅.com", "lumiya.vip")
	s = strings.ReplaceAll(s, "露米婭.com", "lumiya.vip")
	if !strings.HasPrefix(strings.ToLower(s), "http://") && !strings.HasPrefix(strings.ToLower(s), "https://") {
		s = "https://" + s
	}
	if parsed, err := url.Parse(s); err == nil && parsed.Host != "" {
		scheme := strings.ToLower(parsed.Scheme)
		host := strings.ToLower(parsed.Host)
		if host == "xn--1ys141f4ks.com" || host == "www.lumiya.vip" {
			host = "lumiya.vip"
		}
		if strings.Contains(host, "lumiya.vip") || (strings.EqualFold(syncMode, model.UpstreamSyncModeLumiya) && !strings.HasPrefix(host, "127.0.0.1") && !strings.HasPrefix(host, "localhost")) {
			scheme = "https"
		}
		return scheme + "://" + host
	}
	s = strings.TrimRight(s, "/")
	for _, suffix := range []string{"/v1", "/api", "/console", "/pricing", "/panel", "/token", "/personal"} {
		if strings.HasSuffix(strings.ToLower(s), suffix) {
			s = s[:len(s)-len(suffix)]
		}
	}
	return strings.TrimRight(s, "/")
}

func checkNotHTMLBody(body []byte, endpointName string) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		preview := string(trimmed)
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		return fmt.Errorf("upstream %s returned HTML instead of JSON (%s)", endpointName, strings.ReplaceAll(preview, "\n", " "))
	}
	return nil
}

func formatUpstreamNamingRule(rule string, sourceName string, groupName string, ratio float64) string {
	r := strings.TrimSpace(rule)
	if r == "" {
		r = "{name}-{group}-{ratio}"
	}
	ratioStr := strconv.FormatFloat(math.Round(ratio*1e6)/1e6, 'f', -1, 64)
	out := strings.ReplaceAll(r, "{name}", strings.TrimSpace(sourceName))
	out = strings.ReplaceAll(out, "{group}", strings.TrimSpace(groupName))
	out = strings.ReplaceAll(out, "{ratio}", ratioStr)
	return strings.TrimSpace(out)
}

func defaultUpstreamChannelTag(source model.UpstreamAccountSource) string {
	if t := strings.TrimSpace(source.ChannelTag); t != "" {
		return t
	}
	return "auto:" + strings.TrimSpace(source.Name)
}

func maskSecretToken(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "********" + s[len(s)-4:]
}

func matchesMaskedTokenKey(fullKey string, maskedKey string) bool {
	fk := strings.TrimPrefix(strings.TrimSpace(fullKey), "sk-")
	mk := strings.TrimPrefix(strings.TrimSpace(maskedKey), "sk-")
	if fk == "" || mk == "" {
		return false
	}
	if !strings.Contains(mk, "*") {
		return fk == mk
	}
	prefix, suffix, ok := strings.Cut(mk, "*")
	if !ok {
		return false
	}
	suffix = strings.TrimLeft(suffix, "*")
	if prefix != "" && !strings.HasPrefix(fk, prefix) {
		return false
	}
	if suffix != "" && !strings.HasSuffix(fk, suffix) {
		return false
	}
	return len(fk) >= len(prefix)+len(suffix)
}

func ensureSkKeyPrefix(raw string) string {
	k := strings.TrimSpace(raw)
	if k == "" {
		return ""
	}
	if !strings.HasPrefix(k, "sk-") {
		return "sk-" + k
	}
	return k
}

func setUpstreamAuthHeaders(req *http.Request, accessToken string, userID string, syncMode ...string) {
	mode := ""
	if len(syncMode) > 0 {
		mode = strings.ToLower(strings.TrimSpace(syncMode[0]))
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	if tok := strings.TrimSpace(accessToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if uid := strings.TrimSpace(userID); uid != "" {
		req.Header.Set("New-Api-User", uid)
		if mode != model.UpstreamSyncModeLumiya {
			req.Header.Set("One-Api-User", uid)
			req.Header.Set("VoApi-User", uid)
		}
	}
	if mode == model.UpstreamSyncModeLumiya && req.URL != nil && req.URL.Host != "" {
		origin := req.URL.Scheme + "://" + req.URL.Host
		req.Header.Set("Origin", origin)
		req.Header.Set("Referer", origin+"/console/token")
	}
}

func normalizeUpstreamBaseURL(raw string) string {
	return strings.ToLower(cleanUpstreamAPIBaseURL(raw, ""))
}

func fetchUpstreamPricingByURL(ctx context.Context, baseURL string, accessToken string, userID string, syncMode ...string) (*newAPIUpstreamPricingResponse, error) {
	mode := ""
	if len(syncMode) > 0 {
		mode = syncMode[0]
	}
	cleanBase := cleanUpstreamAPIBaseURL(baseURL, mode)
	endpoint := cleanBase + "/api/pricing"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	setUpstreamAuthHeaders(req, accessToken, userID, mode)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream /api/pricing HTTP %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, err
	}
	if err := checkNotHTMLBody(bodyBytes, "/api/pricing"); err != nil {
		return nil, err
	}
	var pricingResp newAPIUpstreamPricingResponse
	if err := common.DecodeJson(bytes.NewReader(bodyBytes), &pricingResp); err != nil {
		return nil, err
	}
	return &pricingResp, nil
}

func fetchUpstreamModelsFromV1(ctx context.Context, baseURL string, skKey string) ([]string, error) {
	cleanBase := cleanUpstreamAPIBaseURL(baseURL, "")
	endpoint := cleanBase + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(skKey))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream /v1/models HTTP %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if err := checkNotHTMLBody(bodyBytes, "/v1/models"); err != nil {
		return nil, err
	}
	var env struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := common.Unmarshal(bodyBytes, &env); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(env.Data))
	for _, d := range env.Data {
		if id := strings.TrimSpace(d.ID); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func fetchUpstreamSelfGroups(ctx context.Context, baseURL string, accessToken string, userID string, syncMode ...string) (map[string]upstreamGroupEntry, error) {
	mode := ""
	if len(syncMode) > 0 {
		mode = syncMode[0]
	}
	cleanBase := cleanUpstreamAPIBaseURL(baseURL, mode)
	endpoint := cleanBase + "/api/user/self/groups"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	setUpstreamAuthHeaders(req, accessToken, userID, mode)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if err := checkNotHTMLBody(body, "/api/user/self/groups"); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream /api/user/self/groups HTTP %d", resp.StatusCode)
	}

	var envelope struct {
		Success bool                       `json:"success"`
		Message string                     `json:"message"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success && len(envelope.Data) == 0 {
		msg := strings.TrimSpace(envelope.Message)
		if msg == "" {
			msg = "request returned success=false"
		}
		return nil, fmt.Errorf("upstream /api/user/self/groups: %s", msg)
	}

	out := make(map[string]upstreamGroupEntry, len(envelope.Data))
	for g, raw := range envelope.Data {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		var detail struct {
			Desc  string          `json:"desc"`
			Ratio json.RawMessage `json:"ratio"`
		}
		if err := common.Unmarshal(raw, &detail); err == nil {
			ratioRawStr := strings.Trim(string(detail.Ratio), `" `)
			if isAutoUpstreamGroup(g, ratioRawStr) {
				continue
			}
			ratioVal := 1.0
			if parsed, err := strconv.ParseFloat(ratioRawStr, 64); err == nil && parsed > 0 {
				ratioVal = parsed
			}
			out[g] = upstreamGroupEntry{
				Name:        g,
				Description: strings.TrimSpace(detail.Desc),
				Ratio:       ratioVal,
			}
			continue
		}
		var descStr string
		if err := common.Unmarshal(raw, &descStr); err == nil {
			if isAutoUpstreamGroup(g, descStr) {
				continue
			}
			out[g] = upstreamGroupEntry{
				Name:        g,
				Description: strings.TrimSpace(descStr),
				Ratio:       1.0,
			}
		}
	}
	return out, nil
}

type upstreamSelfQuotaInfo struct {
	Quota         int64
	UsedQuota     int64
	QuotaPerUnit  float64
	BalanceUSD    float64
	UsedQuotaUSD  float64
	HasValidQuota bool
}

func resolveEffectiveUpstreamExchangeRate(currency string, exchangeRate float64) float64 {
	return model.ResolveEffectiveChannelExchangeRate(currency, exchangeRate)
}

func resolveUpstreamUnitToLocalUnitMultiplier(currency string, exchangeRate float64) float64 {
	return model.ResolveUpstreamToQuotaUnitMultiplier(currency, exchangeRate)
}

func fetchUpstreamSelfBalance(ctx context.Context, baseURL string, accessToken string, userID string, syncMode string, currency string, exchangeRate float64) upstreamSelfQuotaInfo {
	var info upstreamSelfQuotaInfo
	mode := syncMode
	cleanBase := cleanUpstreamAPIBaseURL(baseURL, mode)
	if cleanBase == "" || strings.TrimSpace(accessToken) == "" {
		return info
	}
	client := &http.Client{Timeout: 15 * time.Second}

	quotaPerUnit := 500000.0
	if statusReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanBase+"/api/status", nil); err == nil {
		setUpstreamAuthHeaders(statusReq, "", "", mode)
		if resp, err := client.Do(statusReq); err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var statusEnv struct {
					Data struct {
						QuotaPerUnit float64 `json:"quota_per_unit"`
					} `json:"data"`
				}
				if err := common.Unmarshal(body, &statusEnv); err == nil && statusEnv.Data.QuotaPerUnit > 0 {
					quotaPerUnit = statusEnv.Data.QuotaPerUnit
				}
			}
		}
	}
	info.QuotaPerUnit = quotaPerUnit

	selfReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanBase+"/api/user/self", nil)
	if err != nil {
		return info
	}
	setUpstreamAuthHeaders(selfReq, accessToken, userID, mode)
	resp, err := client.Do(selfReq)
	if err != nil {
		return info
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info
	}
	var selfEnv struct {
		Success bool `json:"success"`
		Data    struct {
			Quota     int64 `json:"quota"`
			UsedQuota int64 `json:"used_quota"`
		} `json:"data"`
	}
	unitMult := resolveUpstreamUnitToLocalUnitMultiplier(currency, exchangeRate)
	if err := common.Unmarshal(body, &selfEnv); err == nil && selfEnv.Success {
		info.Quota = selfEnv.Data.Quota
		info.UsedQuota = selfEnv.Data.UsedQuota
		info.BalanceUSD = math.Round(((float64(selfEnv.Data.Quota)/quotaPerUnit)*unitMult)*1e4) / 1e4
		info.UsedQuotaUSD = math.Round(((float64(selfEnv.Data.UsedQuota)/quotaPerUnit)*unitMult)*1e4) / 1e4
		info.HasValidQuota = true
	}
	return info
}

func resolveUpstreamChannelBalanceUSD(tok upstreamTokenRecord, hasTok bool, selfInfo upstreamSelfQuotaInfo, currency ...string) (float64, bool) {
	quotaPerUnit := selfInfo.QuotaPerUnit
	if quotaPerUnit <= 0 {
		quotaPerUnit = 500000.0
	}
	unitMult := 1.0
	if selfInfo.HasValidQuota && selfInfo.Quota > 0 && selfInfo.BalanceUSD > 0 {
		rawUnits := float64(selfInfo.Quota) / quotaPerUnit
		if rawUnits > 0 {
			unitMult = selfInfo.BalanceUSD / rawUnits
		}
	}
	if unitMult <= 0 {
		unitMult = 1.0
	}
	if hasTok && !tok.UnlimitedQuota && tok.RemainQuota > 0 {
		tokUSD := math.Round(((float64(tok.RemainQuota)/quotaPerUnit)*unitMult)*1e4) / 1e4
		if selfInfo.HasValidQuota {
			return min(tokUSD, selfInfo.BalanceUSD), true
		}
		return tokUSD, true
	}
	if selfInfo.HasValidQuota {
		return selfInfo.BalanceUSD, true
	}
	return 0, false
}

func fetchUpstreamTokensList(ctx context.Context, baseURL string, accessToken string, userID string, syncMode ...string) ([]upstreamTokenRecord, error) {
	mode := ""
	if len(syncMode) > 0 {
		mode = syncMode[0]
	}
	base := cleanUpstreamAPIBaseURL(baseURL, mode)
	client := &http.Client{Timeout: 15 * time.Second}
	allTokens := make([]upstreamTokenRecord, 0)
	seenIDs := make(map[int]struct{})
	lumiyaMode := strings.EqualFold(mode, model.UpstreamSyncModeLumiya)

	for page := 1; page <= 5; page++ {
		var endpoint string
		if lumiyaMode {
			// Lumiya v1.0.0-rc.10 uses 1-indexed p=?&size=100
			endpoint = fmt.Sprintf("%s/api/token/?p=%d&size=100", base, page)
		} else {
			pZero := page - 1
			endpoint = fmt.Sprintf("%s/api/token/?p=%d&size=100&page=%d&page_size=100", base, pZero, page)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		setUpstreamAuthHeaders(req, accessToken, userID, mode)

		resp, err := client.Do(req)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
		_ = resp.Body.Close()
		if err := checkNotHTMLBody(body, "/api/token/"); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if resp.StatusCode != http.StatusOK {
			if page == 1 {
				return nil, fmt.Errorf("upstream /api/token/ HTTP %d", resp.StatusCode)
			}
			break
		}

		var pagedResp struct {
			Success bool            `json:"success"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := common.Unmarshal(body, &pagedResp); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if !pagedResp.Success && page == 1 && len(pagedResp.Data) == 0 {
			return nil, fmt.Errorf("upstream /api/token/: %s", pagedResp.Message)
		}

		var pageObj struct {
			Items []upstreamTokenRecord `json:"items"`
			Data  []upstreamTokenRecord `json:"data"`
			Total int                   `json:"total"`
		}
		var batch []upstreamTokenRecord
		if err := common.Unmarshal(pagedResp.Data, &pageObj); err == nil && (len(pageObj.Items) > 0 || len(pageObj.Data) > 0) {
			if len(pageObj.Items) > 0 {
				batch = pageObj.Items
			} else {
				batch = pageObj.Data
			}
		} else {
			_ = common.Unmarshal(pagedResp.Data, &batch)
		}

		added := 0
		for _, item := range batch {
			if item.ID <= 0 {
				continue
			}
			if _, exists := seenIDs[item.ID]; exists {
				continue
			}
			seenIDs[item.ID] = struct{}{}
			allTokens = append(allTokens, item)
			added++
		}
		if added == 0 || len(batch) < 100 || (pageObj.Total > 0 && len(allTokens) >= pageObj.Total) {
			break
		}
	}
	return allTokens, nil
}

func createUpstreamTokenForGroup(ctx context.Context, baseURL string, accessToken string, userID string, tokenName string, groupName string, syncMode ...string) error {
	mode := ""
	if len(syncMode) > 0 {
		mode = syncMode[0]
	}
	cleanBase := cleanUpstreamAPIBaseURL(baseURL, mode)
	endpoint := cleanBase + "/api/token/"
	var payload map[string]any
	if strings.EqualFold(mode, model.UpstreamSyncModeLumiya) {
		payload = map[string]any{
			"name":                 tokenName,
			"remain_quota":         0,
			"remain_amount":        0,
			"expired_time":         -1,
			"unlimited_quota":      true,
			"model_limits_enabled": false,
			"model_limits":         "",
			"allow_ips":            "",
			"group":                groupName,
			"groups":               []string{},
			"cross_group_retry":    false,
		}
	} else {
		payload = map[string]any{
			"name":                 tokenName,
			"group":                groupName,
			"unlimited_quota":      true,
			"expired_time":         -1,
			"remain_quota":         500000,
			"model_limits_enabled": false,
			"model_limits":         "",
			"allow_ips":            "",
		}
	}
	bodyBytes, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	setUpstreamAuthHeaders(req, accessToken, userID, mode)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream POST /api/token/ HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var res struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(respBody, &res); err == nil && !res.Success && res.Message != "" {
		return fmt.Errorf("upstream POST /api/token/: %s", res.Message)
	}
	return nil
}

func fetchUpstreamTokenBatchKeys(ctx context.Context, baseURL string, accessToken string, userID string, tokenIDs []int, syncMode ...string) map[int]string {
	out := make(map[int]string, len(tokenIDs))
	if len(tokenIDs) == 0 {
		return out
	}
	mode := ""
	if len(syncMode) > 0 {
		mode = syncMode[0]
	}
	base := cleanUpstreamAPIBaseURL(baseURL, mode)
	endpoint := base + "/api/token/batch/keys"
	bodyBytes, err := common.Marshal(map[string]any{"ids": tokenIDs})
	if err != nil {
		return out
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return out
	}
	setUpstreamAuthHeaders(req, accessToken, userID, mode)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	var batchEnv struct {
		Success bool `json:"success"`
		Data    struct {
			Keys map[string]string `json:"keys"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &batchEnv); err == nil && batchEnv.Success {
		for idStr, k := range batchEnv.Data.Keys {
			id, err := strconv.Atoi(strings.TrimSpace(idStr))
			k = strings.TrimSpace(k)
			if err == nil && id > 0 && k != "" && !strings.Contains(k, "*") {
				out[id] = ensureSkKeyPrefix(k)
			}
		}
	}
	return out
}

func fetchUpstreamTokenPlainKey(ctx context.Context, baseURL string, accessToken string, userID string, tokenID int, fallbackKey string, syncMode ...string) (string, error) {
	trimmedFallback := strings.TrimSpace(fallbackKey)
	if trimmedFallback != "" && !strings.Contains(trimmedFallback, "*") && len(trimmedFallback) >= 20 {
		return ensureSkKeyPrefix(trimmedFallback), nil
	}
	mode := ""
	if len(syncMode) > 0 {
		mode = syncMode[0]
	}

	base := cleanUpstreamAPIBaseURL(baseURL, mode)
	client := &http.Client{Timeout: 15 * time.Second}

	// Primary New-API endpoint: POST /api/token/:id/key
	keyEndpoint := fmt.Sprintf("%s/api/token/%d/key", base, tokenID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, keyEndpoint, nil)
	if err == nil {
		setUpstreamAuthHeaders(req, accessToken, userID, mode)
		if resp, err := client.Do(req); err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var keyEnv struct {
					Success bool            `json:"success"`
					Data    json.RawMessage `json:"data"`
				}
				if err := common.Unmarshal(body, &keyEnv); err == nil && keyEnv.Success {
					var keyObj struct {
						Key string `json:"key"`
					}
					if err := common.Unmarshal(keyEnv.Data, &keyObj); err == nil && strings.TrimSpace(keyObj.Key) != "" && !strings.Contains(keyObj.Key, "*") {
						return ensureSkKeyPrefix(keyObj.Key), nil
					}
					var keyStr string
					if err := common.Unmarshal(keyEnv.Data, &keyStr); err == nil && strings.TrimSpace(keyStr) != "" && !strings.Contains(keyStr, "*") {
						return ensureSkKeyPrefix(keyStr), nil
					}
				}
			}
		}
	}

	// Fallback: GET /api/token/:id
	getEndpoint := fmt.Sprintf("%s/api/token/%d", base, tokenID)
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getEndpoint, nil)
	if err == nil {
		setUpstreamAuthHeaders(getReq, accessToken, userID, mode)
		if resp, err := client.Do(getReq); err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var getEnv struct {
					Success bool `json:"success"`
					Data    struct {
						Key string `json:"key"`
					} `json:"data"`
				}
				if err := common.Unmarshal(body, &getEnv); err == nil && strings.TrimSpace(getEnv.Data.Key) != "" && !strings.Contains(getEnv.Data.Key, "*") {
					return ensureSkKeyPrefix(getEnv.Data.Key), nil
				}
			}
		}
	}

	return "", fmt.Errorf("failed to retrieve full key for upstream token #%d", tokenID)
}

func inferGroupFromChannelNameForSource(chName string, sourceName string) string {
	name := strings.TrimSpace(chName)
	prefix := strings.TrimSpace(sourceName) + "-"
	if len(name) <= len(prefix) || !strings.EqualFold(name[:len(prefix)], prefix) {
		return ""
	}
	rest := strings.TrimSpace(name[len(prefix):])
	if lastDash := strings.LastIndex(rest, "-"); lastDash > 0 {
		suffix := strings.TrimSpace(rest[lastDash+1:])
		if _, err := strconv.ParseFloat(suffix, 64); err == nil {
			return strings.TrimSpace(rest[:lastDash])
		}
	}
	return rest
}

func syncSingleUpstreamAccountSource(ctx context.Context, source *model.UpstreamAccountSource, cfg *model.ModelPriceSyncConfig) error {
	if source == nil {
		return fmt.Errorf("upstream source is nil")
	}
	source.Name = strings.TrimSpace(source.Name)
	lowerIdent := strings.ToLower(source.Name + " " + source.BaseURL)
	if strings.TrimSpace(source.SyncMode) == "" {
		if strings.Contains(lowerIdent, "lumiya") || strings.Contains(lowerIdent, "露米娅") || strings.Contains(lowerIdent, "xn--1ys141f4ks") {
			source.SyncMode = model.UpstreamSyncModeLumiya
		} else {
			source.SyncMode = model.UpstreamSyncModeStandard
		}
	}
	if strings.TrimSpace(source.AccessToken) == "uxGSJ4IdALqzBHMEbUYjicNe1qV/3Q==" && isLumiyaSyncMode(source.SyncMode, source.Name, source.BaseURL) {
		source.AccessToken = "8xfOUw1xHsz/dOVmfU4vuJx8T3GsFHk="
	}
	source.BaseURL = cleanUpstreamAPIBaseURL(source.BaseURL, source.SyncMode)
	source.UserID = strings.TrimSpace(source.UserID)
	if source.Name == "" || source.BaseURL == "" {
		return fmt.Errorf("upstream name and base URL are required")
	}
	if strings.TrimSpace(source.AccessToken) == "" {
		return fmt.Errorf("upstream account access token is required for %s", source.Name)
	}
	if strings.TrimSpace(source.TokenNameRule) == "" {
		source.TokenNameRule = "new_api-{group}-{ratio}"
	}
	if strings.TrimSpace(source.ChannelNameRule) == "" {
		source.ChannelNameRule = "{name}-{group}-{ratio}"
	}
	if strings.TrimSpace(source.LocalGroup) == "" {
		source.LocalGroup = "default"
	}
	tagVal := defaultUpstreamChannelTag(*source)
	source.ChannelTag = tagVal

	// 1. Fetch upstream groups from /api/user/self/groups and /api/pricing
	groupMap := make(map[string]upstreamGroupEntry)
	selfGroups, selfGroupsErr := fetchUpstreamSelfGroups(ctx, source.BaseURL, source.AccessToken, source.UserID, source.SyncMode)
	hasAuthoritativeSelfGroups := selfGroupsErr == nil && len(selfGroups) > 0
	if hasAuthoritativeSelfGroups {
		for k, v := range selfGroups {
			groupMap[k] = v
		}
	}

	pricingResp, pricingErr := fetchUpstreamPricingByURL(ctx, source.BaseURL, source.AccessToken, source.UserID, source.SyncMode)
	pricingGroupsWithModels := make(map[string]int)
	if pricingResp != nil {
		for _, item := range pricingResp.Data {
			if strings.TrimSpace(item.ModelName) == "" {
				continue
			}
			for _, eg := range item.EnableGroups {
				eg = strings.TrimSpace(eg)
				if eg != "" {
					pricingGroupsWithModels[eg]++
				}
			}
		}
		for g, desc := range pricingResp.UsableGroup {
			g = strings.TrimSpace(g)
			if isAutoUpstreamGroup(g, desc) {
				continue
			}
			ratio := 1.0
			if r, ok := pricingResp.GroupRatio[g]; ok && r > 0 {
				ratio = r
			}
			if existing, exists := groupMap[g]; exists {
				if existing.Ratio <= 0 && ratio > 0 {
					existing.Ratio = ratio
				}
				if existing.Description == "" {
					existing.Description = desc
				}
				groupMap[g] = existing
			} else if !hasAuthoritativeSelfGroups {
				groupMap[g] = upstreamGroupEntry{
					Name:        g,
					Description: strings.TrimSpace(desc),
					Ratio:       ratio,
				}
			}
		}
		for g, r := range pricingResp.GroupRatio {
			g = strings.TrimSpace(g)
			if isAutoUpstreamGroup(g, "") {
				continue
			}
			if r <= 0 {
				r = 1.0
			}
			if existing, exists := groupMap[g]; exists {
				if existing.Ratio <= 0 {
					existing.Ratio = r
					groupMap[g] = existing
				}
			} else if !hasAuthoritativeSelfGroups && pricingGroupsWithModels[g] > 0 {
				// Never resurrect removed/phantom groups that only linger in GroupRatio without any models.
				groupMap[g] = upstreamGroupEntry{
					Name:  g,
					Ratio: r,
				}
			}
		}
	}

	if len(groupMap) == 0 {
		if selfGroupsErr != nil && pricingErr != nil {
			return fmt.Errorf("failed to fetch upstream groups for %s: groups_api=(%v); pricing_api=(%v)", source.Name, selfGroupsErr, pricingErr)
		}
		if pricingErr != nil {
			return fmt.Errorf("failed to fetch upstream groups for %s: %v", source.Name, pricingErr)
		}
		if selfGroupsErr != nil {
			return fmt.Errorf("failed to fetch upstream groups for %s: %v", source.Name, selfGroupsErr)
		}
		return fmt.Errorf("no valid non-auto groups returned by upstream %s", source.Name)
	}

	sortedGroups := make([]string, 0, len(groupMap))
	for g := range groupMap {
		sortedGroups = append(sortedGroups, g)
	}
	sort.Strings(sortedGroups)

	// 2. Fetch existing upstream tokens and create missing ones per group
	tokens, err := fetchUpstreamTokensList(ctx, source.BaseURL, source.AccessToken, source.UserID, source.SyncMode)
	if err != nil {
		return fmt.Errorf("failed to list upstream tokens for %s: %v", source.Name, err)
	}
	tokenByGroup := make(map[string]upstreamTokenRecord, len(sortedGroups))
	for _, t := range tokens {
		g := strings.TrimSpace(t.Group)
		if g == "" || t.Status != 1 {
			continue
		}
		if _, exists := tokenByGroup[g]; !exists {
			tokenByGroup[g] = t
		}
	}

	// Pre-check whether each group has any models in /api/pricing before creating upstream tokens.
	pricingModelCountByGroup := make(map[string]int, len(sortedGroups))
	if pricingResp != nil {
		for _, g := range sortedGroups {
			cnt := 0
			for _, item := range pricingResp.Data {
				if strings.TrimSpace(item.ModelName) != "" && upstreamItemInGroup(item, g) {
					cnt++
				}
			}
			pricingModelCountByGroup[g] = cnt
		}
	}

	createdAnyToken := false
	for _, g := range sortedGroups {
		if _, exists := tokenByGroup[g]; exists {
			continue
		}
		if pricingResp != nil && pricingModelCountByGroup[g] == 0 {
			// Skip creating an upstream token for a group with zero models.
			continue
		}
		entry := groupMap[g]
		expectedTokenName := formatUpstreamNamingRule(source.TokenNameRule, source.Name, g, entry.Ratio)
		if err := createUpstreamTokenForGroup(ctx, source.BaseURL, source.AccessToken, source.UserID, expectedTokenName, g, source.SyncMode); err == nil {
			createdAnyToken = true
		}
	}
	if createdAnyToken {
		if refreshed, err := fetchUpstreamTokensList(ctx, source.BaseURL, source.AccessToken, source.UserID, source.SyncMode); err == nil {
			for _, t := range refreshed {
				g := strings.TrimSpace(t.Group)
				if g == "" || t.Status != 1 {
					continue
				}
				if _, exists := tokenByGroup[g]; !exists {
					tokenByGroup[g] = t
				}
			}
		}
	}

	// 3. Load existing local channels to match or create
	var allChannels []*model.Channel
	if model.DB != nil {
		if err := model.DB.Find(&allChannels).Error; err != nil {
			return err
		}
	}
	normSourceURL := normalizeUpstreamBaseURL(source.BaseURL)

	findExistingChannelForGroup := func(g string, expectedChName string) *model.Channel {
		// Priority 1: AutoProvisionedChannels metadata
		for _, ch := range allChannels {
			if ch == nil {
				continue
			}
			if meta, ok := cfg.AutoProvisionedChannels[ch.Id]; ok {
				if (meta.SourceID == source.ID || strings.EqualFold(meta.SourceName, source.Name)) && meta.UpstreamGroup == g {
					return ch
				}
			}
		}
		// Priority 2: Same base URL + configured ChannelUpstreamGroups
		for _, ch := range allChannels {
			if ch == nil || normalizeUpstreamBaseURL(ch.GetBaseURL()) != normSourceURL {
				continue
			}
			if cfg.ChannelUpstreamGroups[ch.Id] == g {
				return ch
			}
		}
		// Priority 3: Same base URL + channel name matches expected name or {source.Name}-{g}-<ratio>
		for _, ch := range allChannels {
			if ch == nil || normalizeUpstreamBaseURL(ch.GetBaseURL()) != normSourceURL {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(ch.Name), expectedChName) {
				return ch
			}
			if inferred := inferGroupFromChannelNameForSource(ch.Name, source.Name); inferred != "" && strings.EqualFold(inferred, g) {
				return ch
			}
		}
		return nil
	}

	// Pre-fetch plain keys in a single batch call (supported by Lumiya / modern New-API)
	neededBatchTokenIDs := make([]int, 0, len(sortedGroups))
	for _, g := range sortedGroups {
		tok, hasTok := tokenByGroup[g]
		if !hasTok || tok.ID <= 0 {
			continue
		}
		trimmedKey := strings.TrimSpace(tok.Key)
		if trimmedKey != "" && !strings.Contains(trimmedKey, "*") {
			continue
		}
		entry := groupMap[g]
		expectedChName := formatUpstreamNamingRule(source.ChannelNameRule, source.Name, g, entry.Ratio)
		existingCh := findExistingChannelForGroup(g, expectedChName)
		if existingCh != nil && strings.TrimSpace(existingCh.Key) != "" && matchesMaskedTokenKey(existingCh.Key, tok.Key) {
			continue
		}
		neededBatchTokenIDs = append(neededBatchTokenIDs, tok.ID)
	}
	batchPlainKeys := fetchUpstreamTokenBatchKeys(ctx, source.BaseURL, source.AccessToken, source.UserID, neededBatchTokenIDs, source.SyncMode)

	resolvedCurrency := "CNY"
	resolvedRate := 1.0
	chCurr := strings.ToUpper(strings.TrimSpace(source.Currency))
	if chCurr != "" && chCurr != "AUTO" {
		resolvedCurrency = chCurr
		resolvedRate = resolveEffectiveUpstreamExchangeRate(chCurr, source.ExchangeRate)
	} else {
		resolvedRate = resolveEffectiveUpstreamExchangeRate(resolvedCurrency, source.ExchangeRate)
	}

	createdNames := make([]string, 0)
	updatedNames := make([]string, 0)
	removedNames := make([]string, 0)
	syncedChannelIDs := make([]int, 0, len(sortedGroups))
	activeModelGroupSet := make(map[string]struct{}, len(sortedGroups))
	now := time.Now().Unix()

	selfBalance := fetchUpstreamSelfBalance(ctx, source.BaseURL, source.AccessToken, source.UserID, source.SyncMode, resolvedCurrency, resolvedRate)
	if selfBalance.HasValidQuota {
		source.BalanceUSD = selfBalance.BalanceUSD
		source.UsedQuotaUSD = selfBalance.UsedQuotaUSD
	}

	for _, g := range sortedGroups {
		entry := groupMap[g]
		expectedChName := formatUpstreamNamingRule(source.ChannelNameRule, source.Name, g, entry.Ratio)
		existingCh := findExistingChannelForGroup(g, expectedChName)

		var fullKey string
		tok, hasTok := tokenByGroup[g]
		if hasTok {
			if existingCh != nil && strings.TrimSpace(existingCh.Key) != "" && matchesMaskedTokenKey(existingCh.Key, tok.Key) {
				fullKey = ensureSkKeyPrefix(existingCh.Key)
			} else if batchKey, ok := batchPlainKeys[tok.ID]; ok && batchKey != "" {
				fullKey = batchKey
			} else if k, err := fetchUpstreamTokenPlainKey(ctx, source.BaseURL, source.AccessToken, source.UserID, tok.ID, tok.Key, source.SyncMode); err == nil {
				fullKey = k
			}
		}
		if fullKey == "" && existingCh != nil {
			fullKey = existingCh.Key
		}
		chBalUSD, hasChBal := resolveUpstreamChannelBalanceUSD(tok, hasTok, selfBalance)

		// Collect models and cost entries for group g from /api/pricing (and /v1/models fallback if empty)
		modelSet := make(map[string]struct{})
		groupCostEntries := make(map[string]model.ChannelModelCostEntry)
		groupRatio := entry.Ratio
		if groupRatio <= 0 {
			groupRatio = 1.0
		}
		if pricingResp != nil {
			for _, item := range pricingResp.Data {
				mName := strings.TrimSpace(item.ModelName)
				if mName == "" || !upstreamItemInGroup(item, g) {
					continue
				}
				modelSet[mName] = struct{}{}
				base := deriveUpstreamBasePricing(item)
				effRate := resolveEffectiveUpstreamExchangeRate(resolvedCurrency, resolvedRate)
				unitMult := resolveUpstreamUnitToLocalUnitMultiplier(resolvedCurrency, effRate)
				cEntry := model.ChannelModelCostEntry{
					ChannelName:      expectedChName,
					ChannelType:      constant.ChannelTypeNewAPI,
					Model:            mName,
					ModelRatio:       roundRatioValue(base.ModelRatio),
					CompletionRatio:  base.CompletionRatio,
					CacheRatio:       base.CacheRatio,
					CreateCacheRatio: base.CreateCacheRatio,
					BaseMultiplier:   groupRatio,
					Currency:         resolvedCurrency,
					ExchangeRate:     effRate,
					FromUpstream:     true,
					UpdatedAt:        now,
				}
				if base.QuotaType == 1 && base.ModelPrice >= 0 {
					cEntry.QuotaType = 1
					cEntry.RawModelPrice = roundRatioValue(base.ModelPrice)
					cEntry.ModelPrice = roundRatioValue(base.ModelPrice)
					cEntry.RawInputCost = roundRatioValue(cEntry.RawModelPrice * groupRatio)
					cEntry.RawOutputCost = cEntry.RawInputCost
					cEntry.InputCost = roundRatioValue(cEntry.RawInputCost * unitMult)
					cEntry.OutputCost = cEntry.InputCost
					groupCostEntries[mName] = cEntry
				} else if base.Priced && !isUnconfiguredDefaultModelRatio(mName, 0, base.ModelRatio, base.CompletionRatio) {
					cEntry.QuotaType = 0
					cEntry.RawInputCost = roundRatioValue(base.ModelRatio * 2.0 * groupRatio)
					cEntry.RawOutputCost = roundRatioValue(cEntry.RawInputCost * base.CompletionRatio)
					cEntry.InputCost = roundRatioValue(cEntry.RawInputCost * unitMult)
					cEntry.OutputCost = roundRatioValue(cEntry.InputCost * base.CompletionRatio)
					groupCostEntries[mName] = cEntry
				}
			}
		}
		if len(modelSet) == 0 && fullKey != "" {
			if mList, err := fetchUpstreamModelsFromV1(ctx, source.BaseURL, fullKey); err == nil {
				for _, m := range mList {
					if strings.TrimSpace(m) != "" {
						modelSet[strings.TrimSpace(m)] = struct{}{}
					}
				}
			}
		}
		modelsList := make([]string, 0, len(modelSet))
		for m := range modelSet {
			modelsList = append(modelsList, m)
		}
		sort.Strings(modelsList)
		modelsJoined := strings.Join(modelsList, ",")

		if len(modelsList) == 0 {
			// Do not create new empty-model channels. For existing channels whose group now has
			// zero models, let Step 4 delete auto-managed ones when CleanRemovedGroups is on,
			// or auto-disable them and clear stale models when CleanRemovedGroups is off.
			if existingCh != nil {
				prevMeta, hadMeta := cfg.AutoProvisionedChannels[existingCh.Id]
				isAuto := (hadMeta && prevMeta.AutoCreated) || source.AdoptExistingChannel || strings.EqualFold(existingCh.GetTag(), tagVal)
				if !(source.CleanRemovedGroups && isAuto) && model.DB != nil {
					if existingCh.Models != "" || existingCh.Status == common.ChannelStatusEnabled {
						existingCh.Models = ""
						existingCh.Status = common.ChannelStatusAutoDisabled
						_ = model.DB.Model(&model.Channel{}).Where("id = ?", existingCh.Id).Updates(map[string]any{
							"models": "",
							"status": common.ChannelStatusAutoDisabled,
						}).Error
						_ = existingCh.UpdateAbilities(nil)
						updatedNames = append(updatedNames, existingCh.Name)
						syncedChannelIDs = append(syncedChannelIDs, existingCh.Id)
					}
				}
			}
			continue
		}
		activeModelGroupSet[g] = struct{}{}

		var targetChannelID int
		if existingCh != nil {
			updates := map[string]any{
				"name":     expectedChName,
				"base_url": source.BaseURL,
			}
			existingCh.Name = expectedChName
			if resolvedCurrency != "" {
				currCopy := resolvedCurrency
				updates["currency_code"] = currCopy
				existingCh.CurrencyCode = &currCopy
			}
			if fullKey != "" && fullKey != existingCh.Key {
				updates["key"] = fullKey
				existingCh.Key = fullKey
			}
			if modelsJoined != existingCh.Models {
				updates["models"] = modelsJoined
				existingCh.Models = modelsJoined
			}
			if hasChBal {
				updates["balance"] = chBalUSD
				updates["balance_updated_time"] = now
				existingCh.Balance = chBalUSD
				existingCh.BalanceUpdatedTime = now
			}
			prevMeta, hadMeta := cfg.AutoProvisionedChannels[existingCh.Id]
			isAuto := (hadMeta && prevMeta.AutoCreated) || source.AdoptExistingChannel || strings.EqualFold(existingCh.GetTag(), tagVal)
			if isAuto {
				existingCh.SetTag(tagVal)
				updates["tag"] = tagVal
				createdAt := prevMeta.CreatedAt
				if createdAt <= 0 {
					createdAt = now
				}
				cfg.AutoProvisionedChannels[existingCh.Id] = model.AutoProvisionedChannelMeta{
					ChannelID:          existingCh.Id,
					ChannelName:        expectedChName,
					SourceID:           source.ID,
					SourceName:         source.Name,
					UpstreamGroup:      g,
					AutoCreated:        true,
					Balance:            existingCh.Balance,
					BalanceUpdatedTime: existingCh.BalanceUpdatedTime,
					CreatedAt:          createdAt,
					UpdatedAt:          now,
				}
			}
			if model.DB != nil {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", existingCh.Id).Updates(updates).Error
				if hasChBal && existingCh.GetBalanceDetail() != nil {
					recordBuiltinBalance(existingCh, channelBalanceResult{Balance: chBalUSD}, nil)
				}
				_ = existingCh.UpdateAbilities(nil)
			}
			cfg.ChannelUpstreamGroups[existingCh.Id] = g
			if source.TopupMultiplier > 0 {
				cfg.ChannelTopupMultipliers[existingCh.Id] = source.TopupMultiplier
			}
			if source.Currency != "" && source.Currency != "AUTO" {
				cfg.ChannelCurrencies[existingCh.Id] = source.Currency
				if source.ExchangeRate > 0 {
					cfg.ChannelExchangeRates[existingCh.Id] = source.ExchangeRate
				}
			}
			updatedNames = append(updatedNames, expectedChName)
			syncedChannelIDs = append(syncedChannelIDs, existingCh.Id)
			targetChannelID = existingCh.Id
		} else {
			if fullKey == "" {
				continue
			}
			baseURLCopy := source.BaseURL
			tagCopy := tagVal
			currCopy := resolvedCurrency
			weight := uint(1)
			priority := int64(0)
			newCh := model.Channel{
				Type:               constant.ChannelTypeNewAPI,
				Name:               expectedChName,
				BaseURL:            &baseURLCopy,
				Key:                fullKey,
				Group:              source.LocalGroup,
				Models:             modelsJoined,
				Status:             common.ChannelStatusEnabled,
				Weight:             &weight,
				Priority:           &priority,
				Tag:                &tagCopy,
				CurrencyCode:       &currCopy,
				Balance:            chBalUSD,
				BalanceUpdatedTime: now,
				CreatedTime:        now,
			}
			if model.DB != nil {
				if err := model.BatchInsertChannels([]model.Channel{newCh}); err == nil {
					var inserted model.Channel
					if err := model.DB.Where("name = ? AND base_url = ?", expectedChName, source.BaseURL).Order("id DESC").First(&inserted).Error; err == nil && inserted.Id > 0 {
						if hasChBal {
							_ = model.DB.Model(&model.Channel{}).Where("id = ?", inserted.Id).Updates(map[string]any{
								"balance":              chBalUSD,
								"balance_updated_time": now,
							}).Error
							inserted.Balance = chBalUSD
							inserted.BalanceUpdatedTime = now
						}
						cfg.AutoProvisionedChannels[inserted.Id] = model.AutoProvisionedChannelMeta{
							ChannelID:          inserted.Id,
							ChannelName:        expectedChName,
							SourceID:           source.ID,
							SourceName:         source.Name,
							UpstreamGroup:      g,
							AutoCreated:        true,
							Balance:            inserted.Balance,
							BalanceUpdatedTime: inserted.BalanceUpdatedTime,
							CreatedAt:          now,
							UpdatedAt:          now,
						}
						cfg.ChannelUpstreamGroups[inserted.Id] = g
						if source.TopupMultiplier > 0 {
							cfg.ChannelTopupMultipliers[inserted.Id] = source.TopupMultiplier
						}
						if source.Currency != "" && source.Currency != "AUTO" {
							cfg.ChannelCurrencies[inserted.Id] = source.Currency
							if source.ExchangeRate > 0 {
								cfg.ChannelExchangeRates[inserted.Id] = source.ExchangeRate
							}
						}
						allChannels = append(allChannels, &inserted)
						syncedChannelIDs = append(syncedChannelIDs, inserted.Id)
						targetChannelID = inserted.Id
					}
				}
			}
			createdNames = append(createdNames, expectedChName)
		}

		if targetChannelID > 0 && len(groupCostEntries) > 0 {
			for k, v := range groupCostEntries {
				v.ChannelID = targetChannelID
				groupCostEntries[k] = v
			}
			model.SetChannelUpstreamModelCosts(targetChannelID, groupCostEntries, false)
		}
	}

	// 4. Clean up auto-created channels whose upstream group disappeared or now has zero models
	if source.CleanRemovedGroups && len(sortedGroups) > 0 && model.DB != nil {
		var currentChannels []*model.Channel
		if err := model.DB.Find(&currentChannels).Error; err == nil {
			validChannelIDs := make(map[int]struct{}, len(currentChannels))
			for _, ch := range currentChannels {
				if ch == nil {
					continue
				}
				validChannelIDs[ch.Id] = struct{}{}
				meta, hasMeta := cfg.AutoProvisionedChannels[ch.Id]
				belongsToSource := false
				isAutoCreated := false
				upGroup := ""

				if hasMeta && (meta.SourceID == source.ID || strings.EqualFold(meta.SourceName, source.Name)) {
					belongsToSource = true
					isAutoCreated = meta.AutoCreated
					upGroup = strings.TrimSpace(meta.UpstreamGroup)
				} else if normalizeUpstreamBaseURL(ch.GetBaseURL()) == normSourceURL {
					chTag := strings.TrimSpace(ch.GetTag())
					if chTag != "" && (strings.EqualFold(chTag, tagVal) || strings.EqualFold(chTag, "auto:"+source.Name)) {
						belongsToSource = true
						isAutoCreated = true
					}
				}

				if !belongsToSource || !isAutoCreated {
					continue
				}
				if upGroup == "" {
					upGroup = strings.TrimSpace(cfg.ChannelUpstreamGroups[ch.Id])
				}
				if upGroup == "" {
					upGroup = inferGroupFromChannelNameForSource(ch.Name, source.Name)
				}
				if upGroup == "" {
					continue
				}
				if _, stillActive := activeModelGroupSet[upGroup]; !stillActive {
					removedNames = append(removedNames, ch.Name)
					_ = ch.Delete()
					delete(validChannelIDs, ch.Id)
					delete(cfg.AutoProvisionedChannels, ch.Id)
					delete(cfg.ChannelUpstreamGroups, ch.Id)
					delete(cfg.ChannelCurrencies, ch.Id)
					delete(cfg.ChannelExchangeRates, ch.Id)
					delete(cfg.ChannelTopupMultipliers, ch.Id)
					for mName, prefID := range cfg.PreferredChannels {
						if prefID == ch.Id {
							delete(cfg.PreferredChannels, mName)
						}
					}
				}
			}
			model.PruneOrphanChannelsFromSyncConfig(cfg, validChannelIDs)
		}
	}

	activeGroupsList := make([]string, 0, len(activeModelGroupSet))
	for g := range activeModelGroupSet {
		activeGroupsList = append(activeGroupsList, g)
	}
	sort.Strings(activeGroupsList)

	source.LastSyncAt = now
	source.LastStatus = "success"
	source.ActiveGroups = activeGroupsList
	source.CreatedChannels = createdNames
	source.UpdatedChannels = updatedNames
	source.RemovedChannels = removedNames
	source.LastMessage = fmt.Sprintf(
		"Groups: %d | Created: +%d | Updated: %d | Removed: -%d",
		len(activeGroupsList), len(createdNames), len(updatedNames), len(removedNames),
	)

	_ = model.SaveModelPriceSyncConfig(*cfg)

	if (len(syncedChannelIDs) > 0 || len(removedNames) > 0) && model.DB != nil {
		if pricingResp != nil || len(syncedChannelIDs) == 0 {
			model.RecalculateAllChannelModelCosts(true)
			if cfg.AutoSyncPrices {
				_, _ = applyAuthoritativeModelPricesToSystem(*cfg)
			}
		} else {
			_, _ = ExecuteChannelUpstreamSync(
				ctx,
				syncedChannelIDs,
				true,
				cfg.AutoSyncPrices,
				source.Currency,
				source.ExchangeRate,
				nil,
			)
		}
		if len(syncedChannelIDs) > 0 && cfg.IsClassifierEnabled() {
			_, _ = model.ApplyChannelClassifierToChannels(syncedChannelIDs)
		}
		model.InitChannelCache()
	}

	return nil
}

func ExecuteUpstreamAccountsProvisionSync(ctx context.Context, sourceIDs []string) (string, error) {
	cfg := model.GetModelPriceSyncConfig()
	if len(cfg.UpstreamAccounts) == 0 {
		if cfg.IsClassifierEnabled() {
			if clsSummary, err := model.ApplyChannelClassifierToChannels(nil); err == nil {
				return fmt.Sprintf("No upstream accounts configured | Classifier: %d/%d channels updated", clsSummary.UpdatedChannels, clsSummary.TotalChannels), nil
			}
		}
		return "No upstream accounts configured", nil
	}

	filterSet := make(map[string]struct{}, len(sourceIDs))
	for _, id := range sourceIDs {
		if strings.TrimSpace(id) != "" {
			filterSet[strings.TrimSpace(id)] = struct{}{}
		}
	}

	var summaries []string
	var firstErr error
	for i := range cfg.UpstreamAccounts {
		src := &cfg.UpstreamAccounts[i]
		if len(filterSet) > 0 {
			if _, ok := filterSet[src.ID]; !ok {
				continue
			}
		} else if !src.Enabled {
			continue
		}

		if err := syncSingleUpstreamAccountSource(ctx, src, &cfg); err != nil {
			src.LastSyncAt = time.Now().Unix()
			src.LastStatus = "failed"
			src.LastMessage = err.Error()
			if firstErr == nil {
				firstErr = err
			}
			summaries = append(summaries, fmt.Sprintf("[%s] FAILED: %v", src.Name, err))
		} else {
			summaries = append(summaries, fmt.Sprintf("[%s] %s", src.Name, src.LastMessage))
		}
	}
	_ = model.SaveModelPriceSyncConfig(cfg)
	if cfg.IsClassifierEnabled() {
		if clsSummary, err := model.ApplyChannelClassifierToChannels(nil); err == nil {
			summaries = append(summaries, fmt.Sprintf("[Classifier] Classified %d channels (updated: %d, groups: %s)", clsSummary.TotalChannels, clsSummary.UpdatedChannels, strings.Join(clsSummary.SyncedGroups, ",")))
		}
	}
	if len(summaries) == 0 {
		return "No enabled upstream accounts to sync", firstErr
	}
	return strings.Join(summaries, "\n"), firstErr
}

func maskUpstreamAccountsForResponse(accounts []model.UpstreamAccountSource) []model.UpstreamAccountSource {
	out := make([]model.UpstreamAccountSource, 0, len(accounts))
	for _, a := range accounts {
		cp := a
		cp.HasAccessToken = strings.TrimSpace(a.AccessToken) != ""
		cp.AccessToken = maskSecretToken(a.AccessToken)
		out = append(out, cp)
	}
	return out
}

func enrichAutoProvisionedChannelsWithBalance(metaMap map[int]model.AutoProvisionedChannelMeta) map[int]model.AutoProvisionedChannelMeta {
	out := make(map[int]model.AutoProvisionedChannelMeta, len(metaMap))
	if len(metaMap) == 0 {
		return out
	}
	chByID := make(map[int]*model.Channel)
	if model.DB != nil {
		var channels []*model.Channel
		if err := model.DB.Select("id, name, balance, balance_updated_time").Find(&channels).Error; err == nil {
			for _, ch := range channels {
				if ch != nil {
					chByID[ch.Id] = ch
				}
			}
		}
	}
	for id, meta := range metaMap {
		if ch, ok := chByID[id]; ok && ch != nil {
			if ch.Name != "" {
				meta.ChannelName = ch.Name
			}
			meta.Balance = ch.Balance
			meta.BalanceUpdatedTime = ch.BalanceUpdatedTime
		}
		out[id] = meta
	}
	return out
}

func GetUpstreamAccounts(c *gin.Context) {
	cfg := model.GetModelPriceSyncConfig()
	common.ApiSuccess(c, gin.H{
		"accounts":                  maskUpstreamAccountsForResponse(cfg.UpstreamAccounts),
		"auto_provisioned_channels": enrichAutoProvisionedChannelsWithBalance(cfg.AutoProvisionedChannels),
	})
}

func SaveUpstreamAccount(c *gin.Context) {
	var req model.UpstreamAccountSource
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	lowerIdent := strings.ToLower(req.Name + " " + req.BaseURL)
	if strings.TrimSpace(req.SyncMode) == "" {
		if strings.Contains(lowerIdent, "lumiya") || strings.Contains(lowerIdent, "露米娅") || strings.Contains(lowerIdent, "xn--1ys141f4ks") {
			req.SyncMode = model.UpstreamSyncModeLumiya
		} else {
			req.SyncMode = model.UpstreamSyncModeStandard
		}
	} else if isLumiyaSyncMode(req.SyncMode, req.Name, req.BaseURL) {
		req.SyncMode = model.UpstreamSyncModeLumiya
	} else {
		req.SyncMode = model.UpstreamSyncModeStandard
	}
	req.BaseURL = cleanUpstreamAPIBaseURL(req.BaseURL, req.SyncMode)
	req.UserID = strings.TrimSpace(req.UserID)
	if req.Name == "" || req.BaseURL == "" {
		common.ApiError(c, common.NewMessage("Upstream name and base URL cannot be empty"))
		return
	}
	if strings.TrimSpace(req.TokenNameRule) == "" {
		req.TokenNameRule = "new_api-{group}-{ratio}"
	}
	if strings.TrimSpace(req.ChannelNameRule) == "" {
		req.ChannelNameRule = "{name}-{group}-{ratio}"
	}
	if strings.TrimSpace(req.LocalGroup) == "" {
		req.LocalGroup = "default"
	}
	req.ChannelTag = defaultUpstreamChannelTag(req)
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if req.TopupMultiplier <= 0 || math.IsNaN(req.TopupMultiplier) || math.IsInf(req.TopupMultiplier, 0) {
		req.TopupMultiplier = 1.0
	}

	cfg := model.GetModelPriceSyncConfig()
	if req.ID == "" {
		req.ID = fmt.Sprintf("upstream-%d", time.Now().UnixNano())
	}

	found := false
	for i := range cfg.UpstreamAccounts {
		if cfg.UpstreamAccounts[i].ID == req.ID {
			prev := cfg.UpstreamAccounts[i]
			// Preserve stored access token if the caller sent empty or masked value
			incomingTok := strings.TrimSpace(req.AccessToken)
			if incomingTok == "" || strings.Contains(incomingTok, "****") {
				req.AccessToken = prev.AccessToken
			}
			req.LastSyncAt = prev.LastSyncAt
			req.LastStatus = prev.LastStatus
			req.LastMessage = prev.LastMessage
			req.BalanceUSD = prev.BalanceUSD
			req.UsedQuotaUSD = prev.UsedQuotaUSD
			req.ActiveGroups = prev.ActiveGroups
			req.CreatedChannels = prev.CreatedChannels
			req.UpdatedChannels = prev.UpdatedChannels
			req.RemovedChannels = prev.RemovedChannels
			cfg.UpstreamAccounts[i] = req
			found = true
			break
		}
	}
	if !found {
		if strings.TrimSpace(req.AccessToken) == "" || strings.Contains(req.AccessToken, "****") {
			common.ApiError(c, common.NewMessage("Upstream access token cannot be empty"))
			return
		}
		req.LastStatus = "idle"
		cfg.UpstreamAccounts = append(cfg.UpstreamAccounts, req)
	}

	// Propagate TopupMultiplier and Currency immediately to all channels belonging to this upstream account
	if cfg.ChannelTopupMultipliers == nil {
		cfg.ChannelTopupMultipliers = make(map[int]float64)
	}
	if cfg.ChannelCurrencies == nil {
		cfg.ChannelCurrencies = make(map[int]string)
	}
	reqCurr := strings.ToUpper(strings.TrimSpace(req.Currency))
	matchedChIDs := make(map[int]struct{})
	for chID, meta := range cfg.AutoProvisionedChannels {
		if chID > 0 && (meta.SourceID == req.ID || strings.EqualFold(strings.TrimSpace(meta.SourceName), req.Name)) {
			matchedChIDs[chID] = struct{}{}
		}
	}
	if model.DB != nil {
		var allChannels []*model.Channel
		if err := model.DB.Omit("key").Find(&allChannels).Error; err == nil {
			upKey := model.UpstreamAccountKey(req)
			for _, ch := range allChannels {
				if ch == nil || ch.Id <= 0 {
					continue
				}
				srcID, srcName := model.ResolveChannelUpstreamSource(ch, nil, cfg)
				if (upKey != "" && srcID == upKey) || strings.EqualFold(srcName, req.Name) {
					matchedChIDs[ch.Id] = struct{}{}
				}
			}
		}
	}
	for chID := range matchedChIDs {
		cfg.ChannelTopupMultipliers[chID] = req.TopupMultiplier
		if reqCurr != "" && reqCurr != "AUTO" {
			cfg.ChannelCurrencies[chID] = reqCurr
			if model.DB != nil {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", chID).Update("currency_code", reqCurr).Error
			}
		}
	}

	if err := model.SaveModelPriceSyncConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	model.ResetRealtimeProfitTracker()
	common.ApiSuccess(c, gin.H{
		"accounts":                  maskUpstreamAccountsForResponse(cfg.UpstreamAccounts),
		"auto_provisioned_channels": enrichAutoProvisionedChannelsWithBalance(cfg.AutoProvisionedChannels),
	})
}

func DeleteUpstreamAccount(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		common.ApiError(c, common.NewMessage("Invalid upstream account ID"))
		return
	}
	cfg := model.GetModelPriceSyncConfig()
	filtered := make([]model.UpstreamAccountSource, 0, len(cfg.UpstreamAccounts))
	for _, a := range cfg.UpstreamAccounts {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	cfg.UpstreamAccounts = filtered
	if err := model.SaveModelPriceSyncConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	model.ResetRealtimeProfitTracker()
	common.ApiSuccess(c, gin.H{
		"accounts":                  maskUpstreamAccountsForResponse(cfg.UpstreamAccounts),
		"auto_provisioned_channels": enrichAutoProvisionedChannelsWithBalance(cfg.AutoProvisionedChannels),
	})
}

func SyncUpstreamAccountsNow(c *gin.Context) {
	var req struct {
		AccountIDs []string `json:"account_ids"`
	}
	_ = c.ShouldBindJSON(&req)

	summary, err := ExecuteUpstreamAccountsProvisionSync(c.Request.Context(), req.AccountIDs)
	cfg := model.GetModelPriceSyncConfig()
	if err != nil {
		common.ApiSuccess(c, gin.H{
			"summary":                   summary,
			"error":                     err.Error(),
			"accounts":                  maskUpstreamAccountsForResponse(cfg.UpstreamAccounts),
			"auto_provisioned_channels": enrichAutoProvisionedChannelsWithBalance(cfg.AutoProvisionedChannels),
		})
		return
	}
	common.ApiSuccess(c, gin.H{
		"summary":                   summary,
		"accounts":                  maskUpstreamAccountsForResponse(cfg.UpstreamAccounts),
		"auto_provisioned_channels": enrichAutoProvisionedChannelsWithBalance(cfg.AutoProvisionedChannels),
	})
}

func buildChannelClassifierPreview(cfg model.ModelPriceSyncConfig) []model.ChannelClassificationItemResult {
	if model.DB == nil {
		return []model.ChannelClassificationItemResult{}
	}
	var channels []*model.Channel
	if err := model.DB.Omit("key").Order("id ASC").Find(&channels).Error; err != nil {
		return []model.ChannelClassificationItemResult{}
	}
	items := make([]model.ChannelClassificationItemResult, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		matched, upGroup, isNative := model.MatchChannelClassifierRules(ch, cfg)
		finalGroupStr := model.ComputeClassifiedChannelGroupString(ch.Group, matched, cfg.ClassifierRules)
		if matched == nil {
			matched = []string{}
		}
		items = append(items, model.ChannelClassificationItemResult{
			ChannelID:     ch.Id,
			ChannelName:   ch.Name,
			ChannelType:   ch.Type,
			UpstreamGroup: upGroup,
			IsNativeAPI:   isNative,
			MatchedGroups: matched,
			FinalGroups:   finalGroupStr,
			Changed:       finalGroupStr != strings.TrimSpace(ch.Group),
		})
	}
	return items
}

func GetChannelClassifier(c *gin.Context) {
	cfg := model.GetModelPriceSyncConfig()
	common.ApiSuccess(c, gin.H{
		"enabled": cfg.IsClassifierEnabled(),
		"rules":   cfg.ClassifierRules,
		"preview": buildChannelClassifierPreview(cfg),
	})
}

func SaveChannelClassifier(c *gin.Context) {
	var req struct {
		Enabled  *bool                         `json:"enabled"`
		Rules    []model.ChannelClassifierRule `json:"rules"`
		Rule     *model.ChannelClassifierRule  `json:"rule"`
		ApplyNow *bool                         `json:"apply_now"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	cfg := model.GetModelPriceSyncConfig()
	if req.Enabled != nil {
		enabled := *req.Enabled
		cfg.ClassifierEnabled = &enabled
	}

	if req.Rules != nil {
		cfg.ClassifierRules = req.Rules
	} else if req.Rule != nil {
		r := *req.Rule
		r.ID = strings.TrimSpace(r.ID)
		r.Name = strings.TrimSpace(r.Name)
		r.TargetGroup = strings.TrimSpace(r.TargetGroup)
		r.Description = strings.TrimSpace(r.Description)
		if r.TargetGroup == "" {
			common.ApiError(c, common.NewMessage("Target group cannot be empty"))
			return
		}
		if r.ID == "" {
			r.ID = fmt.Sprintf("classifier-%s-%d", r.TargetGroup, time.Now().UnixNano())
		}
		found := false
		for i := range cfg.ClassifierRules {
			if cfg.ClassifierRules[i].ID == r.ID {
				cfg.ClassifierRules[i] = r
				found = true
				break
			}
		}
		if !found {
			cfg.ClassifierRules = append(cfg.ClassifierRules, r)
		}
	}

	if err := model.SaveModelPriceSyncConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	cfg = model.GetModelPriceSyncConfig()

	_, _ = model.SyncClassifierGroupsToSystemSettings(cfg.ClassifierRules)

	var summary *model.ChannelClassificationSummary
	if req.ApplyNow == nil || *req.ApplyNow {
		if clsSummary, err := model.ApplyChannelClassifierToChannels(nil); err == nil {
			summary = &clsSummary
		}
	}

	common.ApiSuccess(c, gin.H{
		"enabled": cfg.IsClassifierEnabled(),
		"rules":   cfg.ClassifierRules,
		"summary": summary,
		"preview": buildChannelClassifierPreview(cfg),
	})
}

func DeleteChannelClassifierRule(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		common.ApiError(c, common.NewMessage("Invalid classifier rule ID"))
		return
	}
	cfg := model.GetModelPriceSyncConfig()
	filtered := make([]model.ChannelClassifierRule, 0, len(cfg.ClassifierRules))
	for _, r := range cfg.ClassifierRules {
		if r.ID != id {
			filtered = append(filtered, r)
		}
	}
	cfg.ClassifierRules = filtered
	if err := model.SaveModelPriceSyncConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	cfg = model.GetModelPriceSyncConfig()
	_, _ = model.SyncClassifierGroupsToSystemSettings(cfg.ClassifierRules)
	var summary *model.ChannelClassificationSummary
	if clsSummary, err := model.ApplyChannelClassifierToChannels(nil); err == nil {
		summary = &clsSummary
	}
	common.ApiSuccess(c, gin.H{
		"enabled": cfg.IsClassifierEnabled(),
		"rules":   cfg.ClassifierRules,
		"summary": summary,
		"preview": buildChannelClassifierPreview(cfg),
	})
}

func ApplyChannelClassifierNow(c *gin.Context) {
	var req struct {
		ChannelIDs []int `json:"channel_ids"`
	}
	_ = c.ShouldBindJSON(&req)

	summary, err := model.ApplyChannelClassifierToChannels(req.ChannelIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := model.GetModelPriceSyncConfig()
	common.ApiSuccess(c, gin.H{
		"enabled": cfg.IsClassifierEnabled(),
		"rules":   cfg.ClassifierRules,
		"summary": summary,
		"preview": buildChannelClassifierPreview(cfg),
	})
}

// ==================== Cost Management (Requirements 3 & 4) ====================

type ModelGroupPriceDetail struct {
	Group            string  `json:"group"`
	Description      string  `json:"description"`
	GroupRatio       float64 `json:"group_ratio"`
	QuotaType        int     `json:"quota_type"`
	InputPrice       float64 `json:"input_price"`  // USD per 1M input tokens (or per call)
	OutputPrice      float64 `json:"output_price"` // USD per 1M output tokens (or per call)
	CallCount        int64   `json:"call_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	RevenueUSD       float64 `json:"revenue_usd"`
	CostUSD          float64 `json:"cost_usd"`
	ProfitUSD        float64 `json:"profit_usd"`
	MarginPct        float64 `json:"margin_pct"`
}

type ModelChannelUsageDetail struct {
	ChannelID        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ChannelType      int     `json:"channel_type"`
	UpstreamGroup    string  `json:"upstream_group,omitempty"`
	BaseMultiplier   float64 `json:"base_multiplier"`
	TopupMultiplier  float64 `json:"topup_multiplier"`
	Currency         string  `json:"currency"`
	ExchangeRate     float64 `json:"exchange_rate"`
	FromUpstream     bool    `json:"from_upstream"`
	QuotaType        int     `json:"quota_type"`
	InputCost        float64 `json:"input_cost"`      // USD per 1M input tokens (or per call)
	OutputCost       float64 `json:"output_cost"`     // USD per 1M output tokens (or per call)
	RawInputCost     float64 `json:"raw_input_cost"`  // Original upstream currency per 1M input tokens
	RawOutputCost    float64 `json:"raw_output_cost"` // Original upstream currency per 1M output tokens
	CallCount        int64   `json:"call_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	RevenueUSD       float64 `json:"revenue_usd"`
	CostUSD          float64 `json:"cost_usd"`
	ProfitUSD        float64 `json:"profit_usd"`
	MarginPct        float64 `json:"margin_pct"`
}

type ModelCostAnalysisRow struct {
	Model            string                    `json:"model"`
	QuotaType        int                       `json:"quota_type"`
	BillingMode      string                    `json:"billing_mode"`
	BaseInputPrice   float64                   `json:"base_input_price"`
	BaseOutputPrice  float64                   `json:"base_output_price"`
	ModelRatio       float64                   `json:"model_ratio"`
	CompletionRatio  float64                   `json:"completion_ratio"`
	MinInputCost     float64                   `json:"min_input_cost"`
	MaxInputCost     float64                   `json:"max_input_cost"`
	MinOutputCost    float64                   `json:"min_output_cost"`
	MaxOutputCost    float64                   `json:"max_output_cost"`
	CallCount        int64                     `json:"call_count"`
	PromptTokens     int64                     `json:"prompt_tokens"`
	CompletionTokens int64                     `json:"completion_tokens"`
	TotalTokens      int64                     `json:"total_tokens"`
	RevenueUSD       float64                   `json:"revenue_usd"`
	CostUSD          float64                   `json:"cost_usd"`
	ProfitUSD        float64                   `json:"profit_usd"`
	MarginPct        float64                   `json:"margin_pct"`
	GroupPrices      []ModelGroupPriceDetail   `json:"group_prices"`
	ChannelUsages    []ModelChannelUsageDetail `json:"channel_usages"`
}

type UserModelChannelCostDetail struct {
	Model            string  `json:"model"`
	ChannelID        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	CallCount        int64   `json:"call_count"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	RevenueUSD       float64 `json:"revenue_usd"`
	CostUSD          float64 `json:"cost_usd"`
	ProfitUSD        float64 `json:"profit_usd"`
	MarginPct        float64 `json:"margin_pct"`
}

type UserCostAnalysisRow struct {
	UserID           int                          `json:"user_id"`
	Username         string                       `json:"username"`
	DisplayName      string                       `json:"display_name"`
	Group            string                       `json:"group"`
	Status           int                          `json:"status"`
	RemainQuotaUSD   float64                      `json:"remain_quota_usd"`
	LifetimeUsedUSD  float64                      `json:"lifetime_used_usd"`
	TopupTotalUSD    float64                      `json:"topup_total_usd"`
	OnlineTopupUSD   float64                      `json:"online_topup_usd"`
	OnlinePaidMoney  float64                      `json:"online_paid_money"`
	SystemTopupUSD   float64                      `json:"system_topup_usd"`
	ConsumedUSD      float64                      `json:"consumed_usd"`
	CallCount        int64                        `json:"call_count"`
	PromptTokens     int64                        `json:"prompt_tokens"`
	CompletionTokens int64                        `json:"completion_tokens"`
	TotalTokens      int64                        `json:"total_tokens"`
	RevenueUSD       float64                      `json:"revenue_usd"`
	CostUSD          float64                      `json:"cost_usd"`
	GrossProfitUSD   float64                      `json:"gross_profit_usd"`
	GrossMarginPct   float64                      `json:"gross_margin_pct"`
	ModelBreakdowns  []UserModelChannelCostDetail `json:"model_breakdowns"`
}

func roundMoney6(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1e6) / 1e6
}

func roundPct2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func resolveBasePricingForModel(modelName string) (quotaType int, billingMode string, inputPrice float64, outputPrice float64, modelRatio float64, completionRatio float64) {
	if price, ok := ratio_setting.GetModelPrice(modelName, false); ok && price > 0 {
		return 1, "fixed", roundMoney6(price), roundMoney6(price), 0, 1.0
	}
	if billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
		if expr, hasExpr := billing_setting.GetBillingExpr(modelName); hasExpr && strings.TrimSpace(expr) != "" {
			if unitPrice, ok := extractUpstreamExprUnitPrice(expr); ok && unitPrice > 0 {
				return 1, "tiered_expr", roundMoney6(unitPrice), roundMoney6(unitPrice), 0, 1.0
			}
			tiersRaw := extractBillingExprTierArgs(expr)
			tiers := make([]map[string]float64, 0, len(tiersRaw))
			for _, arg := range tiersRaw {
				tiers = append(tiers, parseBillingExprTierCoefficients(arg))
			}
			if base := chooseUpstreamBaseTier(tiers); base != nil {
				p := base["p"]
				mr := roundMoney6(p / 2.0)
				cr := 1.0
				if cVal, ok := base["c"]; ok && cVal > 0 {
					cr = roundMoney6(cVal / p)
				}
				inPrice := roundMoney6(mr * 2.0)
				outPrice := roundMoney6(inPrice * cr)
				return 0, "tiered_expr", inPrice, outPrice, mr, cr
			}
		}
	}
	mr, hasRatio, _ := ratio_setting.GetModelRatio(modelName)
	cr := ratio_setting.GetCompletionRatio(modelName)
	if cr <= 0 {
		cr = 1.0
	}
	if !hasRatio || isUnconfiguredDefaultModelRatio(modelName, 0, mr, cr) {
		return 0, "token", 0, 0, 0, cr
	}
	inPrice := roundMoney6(mr * 2.0)
	outPrice := roundMoney6(inPrice * cr)
	return 0, "token", inPrice, outPrice, mr, cr
}

func effectiveEntryCostsUSD(entry model.ChannelModelCostEntry, syncCfg model.ModelPriceSyncConfig) (inUSD float64, outUSD float64, rawIn float64, rawOut float64, curr string, rate float64) {
	curr = strings.ToUpper(strings.TrimSpace(entry.Currency))
	rate = entry.ExchangeRate
	if chCurr, ok := syncCfg.ChannelCurrencies[entry.ChannelID]; ok && strings.TrimSpace(chCurr) != "" {
		curr = strings.ToUpper(strings.TrimSpace(chCurr))
	}
	if chRate, ok := syncCfg.ChannelExchangeRates[entry.ChannelID]; ok && chRate > 0 {
		rate = chRate
	}
	if curr == "" {
		curr = "USD"
	}
	effRate := resolveEffectiveUpstreamExchangeRate(curr, rate)
	unitMult := resolveUpstreamUnitToLocalUnitMultiplier(curr, effRate)
	rate = effRate

	rawIn = entry.RawInputCost
	rawOut = entry.RawOutputCost
	if rawIn <= 0 {
		mult := entry.BaseMultiplier
		if mult <= 0 {
			mult = 1.0
		}
		if entry.QuotaType == 1 && entry.ModelPrice > 0 {
			rawIn = roundMoney6(entry.ModelPrice * mult)
			rawOut = rawIn
		} else if entry.ModelRatio > 0 {
			rawIn = roundMoney6(entry.ModelRatio * 2.0 * mult)
			rawOut = roundMoney6(rawIn * max(entry.CompletionRatio, 1.0))
		} else if entry.InputCost > 0 {
			rawIn = entry.InputCost
			rawOut = entry.OutputCost
		}
	}
	if (entry.FromUpstream || entry.ChannelType == constant.ChannelTypeNewAPI) && rawIn > 0 {
		inUSD = roundMoney6(rawIn * unitMult)
		outUSD = roundMoney6(rawOut * unitMult)
		return inUSD, outUSD, rawIn, rawOut, curr, rate
	}
	inUSD = entry.InputCost
	outUSD = entry.OutputCost
	if entry.BaseMultiplier <= 0 || math.Abs(entry.BaseMultiplier-1.0) < 1e-6 {
		if inferred, ok := model.InferChannelTrailingMultiplier(entry.ChannelName); ok && inferred > 0 && inferred != 1.0 {
			inUSD = roundMoney6(inUSD * inferred)
			outUSD = roundMoney6(outUSD * inferred)
		}
	}
	return inUSD, outUSD, rawIn, rawOut, curr, rate
}

func calculateUsageUpstreamCostUSD(entry model.ChannelModelCostEntry, hasEntry bool, baseQuotaType int, baseInputPrice float64, baseOutputPrice float64, callCount int64, promptTokens int64, completionTokens int64, cacheTokens ...int64) float64 {
	if callCount <= 0 && promptTokens <= 0 && completionTokens <= 0 {
		return 0
	}
	var cacheRead, cacheWrite int64
	if len(cacheTokens) > 0 {
		cacheRead = max(cacheTokens[0], 0)
	}
	if len(cacheTokens) > 1 {
		cacheWrite = max(cacheTokens[1], 0)
	}
	computeTokenCost := func(inUnitPrice float64, outUnitPrice float64, cacheReadRatio float64, cacheWriteRatio float64) float64 {
		nonCachedPrompt := max(promptTokens-cacheRead-cacheWrite, 0)
		inCost := (float64(nonCachedPrompt) / 1_000_000.0) * inUnitPrice
		if cacheRead > 0 {
			r := cacheReadRatio
			if r <= 0 {
				if sysCacheR, ok := ratio_setting.GetCacheRatio(entry.Model); ok && sysCacheR > 0 {
					r = sysCacheR
				} else {
					r = 1.0
				}
			}
			inCost += (float64(cacheRead) / 1_000_000.0) * inUnitPrice * r
		}
		if cacheWrite > 0 {
			w := cacheWriteRatio
			if w <= 0 {
				if sysCreateR, ok := ratio_setting.GetCreateCacheRatio(entry.Model); ok && sysCreateR > 0 {
					w = sysCreateR
				} else {
					w = 1.0
				}
			}
			inCost += (float64(cacheWrite) / 1_000_000.0) * inUnitPrice * w
		}
		outCost := (float64(completionTokens) / 1_000_000.0) * outUnitPrice
		return roundMoney6(inCost + outCost)
	}
	if hasEntry {
		inCostUSD := entry.InputCost
		outCostUSD := entry.OutputCost
		if entry.FromUpstream || entry.ChannelType == constant.ChannelTypeNewAPI {
			unitMult := resolveUpstreamUnitToLocalUnitMultiplier(entry.Currency, entry.ExchangeRate)
			if entry.RawInputCost > 0 {
				inCostUSD = roundMoney6(entry.RawInputCost * unitMult)
				outCostUSD = roundMoney6(entry.RawOutputCost * unitMult)
			}
		} else if entry.BaseMultiplier <= 0 || math.Abs(entry.BaseMultiplier-1.0) < 1e-6 {
			if inferred, ok := model.InferChannelTrailingMultiplier(entry.ChannelName); ok && inferred > 0 && inferred != 1.0 {
				inCostUSD = roundMoney6(inCostUSD * inferred)
				outCostUSD = roundMoney6(outCostUSD * inferred)
			}
		}
		if entry.QuotaType == 1 && inCostUSD > 0 {
			return roundMoney6(float64(callCount) * inCostUSD)
		}
		if inCostUSD > 0 || outCostUSD > 0 {
			return computeTokenCost(inCostUSD, outCostUSD, entry.CacheRatio, entry.CreateCacheRatio)
		}
	}
	if baseQuotaType == 1 && baseInputPrice > 0 {
		return roundMoney6(float64(callCount) * baseInputPrice)
	}
	return computeTokenCost(baseInputPrice, baseOutputPrice, entry.CacheRatio, entry.CreateCacheRatio)
}

func GetModelCostAnalysis(c *gin.Context) {
	startTs, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTs, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))

	var channels []*model.Channel
	if model.DB != nil {
		_ = model.DB.Omit("key").Order("id ASC").Find(&channels).Error
	}
	chByID := make(map[int]*model.Channel, len(channels))
	for _, ch := range channels {
		if ch != nil {
			chByID[ch.Id] = ch
		}
	}

	costMap := model.GetAllChannelModelCosts()
	if len(costMap) == 0 && model.DB != nil {
		model.RecalculateAllChannelModelCosts(false)
		costMap = model.GetAllChannelModelCosts()
	}
	syncCfg := model.GetModelPriceSyncConfig()
	groupRatioMap := ratio_setting.GetGroupRatioCopy()
	usableGroups := settingUsableGroupsSnapshot()

	logStats, err := model.AggregateModelChannelGroupUsage(startTs, endTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	modelSet := make(map[string]struct{})
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		for _, m := range ch.GetModels() {
			m = strings.TrimSpace(m)
			if m != "" {
				modelSet[m] = struct{}{}
			}
		}
	}
	for _, st := range logStats {
		m := strings.TrimSpace(st.ModelName)
		if m != "" {
			modelSet[m] = struct{}{}
		}
	}

	// Group log stats by model
	statsByModel := make(map[string][]model.ModelChannelGroupLogStat)
	for _, st := range logStats {
		m := strings.TrimSpace(st.ModelName)
		if m == "" {
			continue
		}
		statsByModel[m] = append(statsByModel[m], st)
	}

	// Build sorted group list
	allGroups := make([]string, 0, len(groupRatioMap)+2)
	seenGroups := make(map[string]struct{})
	for _, defG := range []string{"default", "vip"} {
		seenGroups[defG] = struct{}{}
		allGroups = append(allGroups, defG)
	}
	for g := range groupRatioMap {
		g = strings.TrimSpace(g)
		if g != "" {
			if _, ok := seenGroups[g]; !ok {
				seenGroups[g] = struct{}{}
				allGroups = append(allGroups, g)
			}
		}
	}
	sort.Slice(allGroups, func(i, j int) bool {
		if allGroups[i] == "default" {
			return true
		}
		if allGroups[j] == "default" {
			return false
		}
		return allGroups[i] < allGroups[j]
	})

	quotaUnit := common.QuotaPerUnit
	if quotaUnit <= 0 {
		quotaUnit = 500000
	}

	rows := make([]ModelCostAnalysisRow, 0, len(modelSet))
	var summaryCalls, summaryTokens int64
	var summaryRevenue, summaryCost float64

	for modelName := range modelSet {
		if keyword != "" && !strings.Contains(strings.ToLower(modelName), keyword) {
			continue
		}

		quotaType, billingMode, baseIn, baseOut, mr, cr := resolveBasePricingForModel(modelName)

		// Collect channels that support or have called this model
		channelIDSet := make(map[int]struct{})
		for _, ch := range channels {
			if ch != nil && slices.Contains(ch.GetModels(), modelName) {
				channelIDSet[ch.Id] = struct{}{}
			}
		}
		mStats := statsByModel[modelName]
		for _, st := range mStats {
			if st.ChannelID > 0 {
				channelIDSet[st.ChannelID] = struct{}{}
			}
		}

		// Fallback base price from channel costs if system base price is 0
		if baseIn <= 0 && baseOut <= 0 {
			for chID := range channelIDSet {
				if entry, ok := costMap[chID][modelName]; ok && entry.InputCost > 0 {
					quotaType = entry.QuotaType
					if entry.QuotaType == 1 && entry.ModelPrice > 0 {
						baseIn = roundMoney6(entry.ModelPrice)
						baseOut = roundMoney6(entry.ModelPrice)
					} else if entry.ModelRatio > 0 {
						mr = entry.ModelRatio
						cr = max(entry.CompletionRatio, 1.0)
						baseIn = roundMoney6(mr * 2.0)
						baseOut = roundMoney6(baseIn * cr)
					} else {
						baseIn = entry.InputCost
						baseOut = entry.OutputCost
					}
					break
				}
			}
		}

		// Per-channel usage and cost breakdown
		chStatMap := make(map[int]*ModelChannelUsageDetail, len(channelIDSet))
		minInCost := 0.0
		maxInCost := 0.0
		minOutCost := 0.0
		maxOutCost := 0.0
		hasCostBounds := false

		for chID := range channelIDSet {
			ch := chByID[chID]
			chName := fmt.Sprintf("Channel #%d", chID)
			chType := 0
			var archivedSnap model.ArchivedChannelSnapshot
			hasArchivedSnap := false
			if ch != nil {
				chName = ch.Name
				chType = ch.Type
			} else if snap, ok := model.GetArchivedChannelSnapshot(chID); ok {
				archivedSnap = snap
				hasArchivedSnap = true
				if strings.TrimSpace(snap.ChannelName) != "" {
					chName = strings.TrimSpace(snap.ChannelName) + " (Deleted)"
				}
				chType = snap.ChannelType
			}
			entry, hasEntry := costMap[chID][modelName]
			if !hasEntry && hasArchivedSnap {
				if archivedEntry, ok := archivedSnap.ModelCosts[modelName]; ok {
					entry = archivedEntry
					hasEntry = true
				}
			}
			inCost := baseIn
			outCost := baseOut
			rawInCost := baseIn
			rawOutCost := baseOut
			chQuotaType := quotaType
			baseMult := 1.0
			curr := "USD"
			exRate := 1.0
			if hasEntry {
				inCost, outCost, rawInCost, rawOutCost, curr, exRate = effectiveEntryCostsUSD(entry, syncCfg)
				chQuotaType = entry.QuotaType
				if entry.BaseMultiplier > 0 {
					baseMult = entry.BaseMultiplier
				}
				if baseMult == 1.0 {
					if inferred, ok := model.InferChannelTrailingMultiplier(chName); ok && inferred > 0 && inferred != 1.0 {
						baseMult = inferred
					}
				}
			} else if ch != nil {
				curr, exRate, baseMult = model.ResolveChannelCurrencyAndMultiplier(ch, costMap[chID], syncCfg)
				costScale := 1.0
				if baseMult > 0 && baseMult != 1.0 {
					costScale = baseMult
				}
				if ch.Type == constant.ChannelTypeNewAPI {
					unitMult := resolveUpstreamUnitToLocalUnitMultiplier(curr, exRate)
					if unitMult > 0 {
						costScale = roundMoney6(costScale * unitMult)
					}
				}
				inCost = roundMoney6(baseIn * costScale)
				outCost = roundMoney6(baseOut * costScale)
				rawInCost = roundMoney6(baseIn * baseMult)
				rawOutCost = roundMoney6(baseOut * baseMult)
			} else if hasArchivedSnap {
				curr = archivedSnap.Currency
				if curr == "" {
					curr = "USD"
				}
				exRate = archivedSnap.ExchangeRate
				if exRate <= 0 {
					exRate = 1.0
				}
				baseMult = archivedSnap.BaseMultiplier
				if baseMult <= 0 {
					baseMult = 1.0
				}
				costScale := baseMult
				if archivedSnap.ChannelType == constant.ChannelTypeNewAPI {
					unitMult := resolveUpstreamUnitToLocalUnitMultiplier(curr, exRate)
					if unitMult > 0 {
						costScale = roundMoney6(costScale * unitMult)
					}
				}
				inCost = roundMoney6(baseIn * costScale)
				outCost = roundMoney6(baseOut * costScale)
				rawInCost = roundMoney6(baseIn * baseMult)
				rawOutCost = roundMoney6(baseOut * baseMult)
			}
			topupMult := 1.0
			if ch != nil {
				topupMult = model.ResolveChannelTopupMultiplier(chID, ch, syncCfg)
			} else if hasArchivedSnap && archivedSnap.TopupMultiplier > 0 {
				topupMult = archivedSnap.TopupMultiplier
			} else {
				topupMult = model.ResolveChannelTopupMultiplier(chID, nil, syncCfg)
			}
			if topupMult <= 0 {
				topupMult = 1.0
			}
			if math.Abs(topupMult-1.0) > 1e-6 {
				inCost = roundMoney6(inCost / topupMult)
				outCost = roundMoney6(outCost / topupMult)
			}
			if inCost > 0 {
				if !hasCostBounds || inCost < minInCost {
					minInCost = inCost
				}
				if !hasCostBounds || inCost > maxInCost {
					maxInCost = inCost
				}
				if !hasCostBounds || outCost < minOutCost {
					minOutCost = outCost
				}
				if !hasCostBounds || outCost > maxOutCost {
					maxOutCost = outCost
				}
				hasCostBounds = true
			}
			upGroup := syncCfg.ChannelUpstreamGroups[chID]
			if upGroup == "" && hasArchivedSnap {
				upGroup = archivedSnap.UpstreamGroup
			}
			chStatMap[chID] = &ModelChannelUsageDetail{
				ChannelID:       chID,
				ChannelName:     chName,
				ChannelType:     chType,
				UpstreamGroup:   upGroup,
				BaseMultiplier:  baseMult,
				TopupMultiplier: topupMult,
				Currency:        curr,
				ExchangeRate:    exRate,
				FromUpstream:    hasEntry && entry.FromUpstream,
				QuotaType:       chQuotaType,
				InputCost:       inCost,
				OutputCost:      outCost,
				RawInputCost:    rawInCost,
				RawOutputCost:   rawOutCost,
			}
		}

		// Per-group usage and price breakdown
		grpStatMap := make(map[string]*ModelGroupPriceDetail, len(allGroups))
		for _, g := range allGroups {
			gRatio := groupRatioMap[g]
			if gRatio <= 0 {
				gRatio = 1.0
			}
			grpStatMap[g] = &ModelGroupPriceDetail{
				Group:       g,
				Description: usableGroups[g],
				GroupRatio:  gRatio,
				QuotaType:   quotaType,
				InputPrice:  roundMoney6(baseIn * gRatio),
				OutputPrice: roundMoney6(baseOut * gRatio),
			}
		}

		var totalCalls, totalPrompt, totalComp int64
		var totalRev, totalCost float64

		for _, st := range mStats {
			rev := roundMoney6(float64(st.TotalQuota) / quotaUnit)
			var cost float64
			if st.FrozenCallCount >= st.CallCount && st.FrozenCallCount > 0 {
				cost = roundMoney6(st.FrozenCostUSD)
			} else {
				remCalls := max(st.CallCount-st.FrozenCallCount, 0)
				remPrompt := max(st.PromptTokens-st.FrozenPromptTokens, 0)
				remComp := max(st.CompletionTokens-st.FrozenCompletionTokens, 0)
				remCache := max(st.CacheTokens-st.FrozenCacheTokens, 0)
				remCacheWrite := max(st.CacheWriteTokens-st.FrozenCacheWriteTokens, 0)

				entry, hasEntry := costMap[st.ChannelID][modelName]
				if !hasEntry {
					if snap, ok := model.GetArchivedChannelSnapshot(st.ChannelID); ok {
						if archivedEntry, ok := snap.ModelCosts[modelName]; ok {
							entry = archivedEntry
							hasEntry = true
						}
					}
				}
				if !hasEntry {
					entry.Model = modelName
					if cacheR, ok := ratio_setting.GetCacheRatio(modelName); ok && cacheR > 0 {
						entry.CacheRatio = cacheR
					}
					if createCacheR, ok := ratio_setting.GetCreateCacheRatio(modelName); ok && createCacheR > 0 {
						entry.CreateCacheRatio = createCacheR
					}
				}
				effBaseIn := baseIn
				effBaseOut := baseOut
				usedPreDividedDetail := false
				if !hasEntry {
					if chDetail, ok := chStatMap[st.ChannelID]; ok && (chDetail.InputCost > 0 || chDetail.OutputCost > 0) {
						effBaseIn = chDetail.InputCost
						effBaseOut = chDetail.OutputCost
						usedPreDividedDetail = true
					}
				}
				remCost := calculateUsageUpstreamCostUSD(entry, hasEntry, quotaType, effBaseIn, effBaseOut, remCalls, remPrompt, remComp, remCache, remCacheWrite)
				if !usedPreDividedDetail {
					topupMult := 1.0
					if chDetail, ok := chStatMap[st.ChannelID]; ok && chDetail.TopupMultiplier > 0 {
						topupMult = chDetail.TopupMultiplier
					} else {
						topupMult = model.ResolveChannelTopupMultiplier(st.ChannelID, chByID[st.ChannelID], syncCfg)
					}
					if topupMult > 0 && math.Abs(topupMult-1.0) > 1e-6 {
						remCost = roundMoney6(remCost / topupMult)
					}
				}
				cost = roundMoney6(st.FrozenCostUSD + remCost)
			}

			totalCalls += st.CallCount
			totalPrompt += st.PromptTokens
			totalComp += st.CompletionTokens
			totalRev += rev
			totalCost += cost

			if chDetail, ok := chStatMap[st.ChannelID]; ok {
				chDetail.CallCount += st.CallCount
				chDetail.PromptTokens += st.PromptTokens
				chDetail.CompletionTokens += st.CompletionTokens
				chDetail.TotalTokens += st.PromptTokens + st.CompletionTokens
				chDetail.RevenueUSD = roundMoney6(chDetail.RevenueUSD + rev)
				chDetail.CostUSD = roundMoney6(chDetail.CostUSD + cost)
			}

			gName := strings.TrimSpace(st.UserGroup)
			if gName == "" {
				gName = "default"
			}
			grpDetail, ok := grpStatMap[gName]
			if !ok {
				gRatio := groupRatioMap[gName]
				if gRatio <= 0 {
					gRatio = 1.0
				}
				grpDetail = &ModelGroupPriceDetail{
					Group:       gName,
					Description: usableGroups[gName],
					GroupRatio:  gRatio,
					QuotaType:   quotaType,
					InputPrice:  roundMoney6(baseIn * gRatio),
					OutputPrice: roundMoney6(baseOut * gRatio),
				}
				grpStatMap[gName] = grpDetail
			}
			grpDetail.CallCount += st.CallCount
			grpDetail.PromptTokens += st.PromptTokens
			grpDetail.CompletionTokens += st.CompletionTokens
			grpDetail.TotalTokens += st.PromptTokens + st.CompletionTokens
			grpDetail.RevenueUSD = roundMoney6(grpDetail.RevenueUSD + rev)
			grpDetail.CostUSD = roundMoney6(grpDetail.CostUSD + cost)
		}

		channelUsages := make([]ModelChannelUsageDetail, 0, len(chStatMap))
		for _, chD := range chStatMap {
			chD.ProfitUSD = roundMoney6(chD.RevenueUSD - chD.CostUSD)
			if chD.RevenueUSD > 0 {
				chD.MarginPct = roundPct2((chD.ProfitUSD / chD.RevenueUSD) * 100.0)
			}
			channelUsages = append(channelUsages, *chD)
		}
		sort.Slice(channelUsages, func(i, j int) bool {
			if channelUsages[i].CallCount != channelUsages[j].CallCount {
				return channelUsages[i].CallCount > channelUsages[j].CallCount
			}
			if channelUsages[i].InputCost != channelUsages[j].InputCost {
				return channelUsages[i].InputCost < channelUsages[j].InputCost
			}
			return channelUsages[i].ChannelID < channelUsages[j].ChannelID
		})

		groupPrices := make([]ModelGroupPriceDetail, 0, len(grpStatMap))
		for _, gD := range grpStatMap {
			gD.ProfitUSD = roundMoney6(gD.RevenueUSD - gD.CostUSD)
			if gD.RevenueUSD > 0 {
				gD.MarginPct = roundPct2((gD.ProfitUSD / gD.RevenueUSD) * 100.0)
			}
			groupPrices = append(groupPrices, *gD)
		}
		sort.Slice(groupPrices, func(i, j int) bool {
			if groupPrices[i].CallCount != groupPrices[j].CallCount {
				return groupPrices[i].CallCount > groupPrices[j].CallCount
			}
			if groupPrices[i].Group == "default" {
				return true
			}
			if groupPrices[j].Group == "default" {
				return false
			}
			return groupPrices[i].Group < groupPrices[j].Group
		})

		totalRev = roundMoney6(totalRev)
		totalCost = roundMoney6(totalCost)
		profit := roundMoney6(totalRev - totalCost)
		marginPct := 0.0
		if totalRev > 0 {
			marginPct = roundPct2((profit / totalRev) * 100.0)
		}

		summaryCalls += totalCalls
		summaryTokens += totalPrompt + totalComp
		summaryRevenue += totalRev
		summaryCost += totalCost

		rows = append(rows, ModelCostAnalysisRow{
			Model:            modelName,
			QuotaType:        quotaType,
			BillingMode:      billingMode,
			BaseInputPrice:   baseIn,
			BaseOutputPrice:  baseOut,
			ModelRatio:       mr,
			CompletionRatio:  cr,
			MinInputCost:     minInCost,
			MaxInputCost:     maxInCost,
			MinOutputCost:    minOutCost,
			MaxOutputCost:    maxOutCost,
			CallCount:        totalCalls,
			PromptTokens:     totalPrompt,
			CompletionTokens: totalComp,
			TotalTokens:      totalPrompt + totalComp,
			RevenueUSD:       totalRev,
			CostUSD:          totalCost,
			ProfitUSD:        profit,
			MarginPct:        marginPct,
			GroupPrices:      groupPrices,
			ChannelUsages:    channelUsages,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CallCount != rows[j].CallCount {
			return rows[i].CallCount > rows[j].CallCount
		}
		if rows[i].RevenueUSD != rows[j].RevenueUSD {
			return rows[i].RevenueUSD > rows[j].RevenueUSD
		}
		return rows[i].Model < rows[j].Model
	})

	summaryProfit := roundMoney6(summaryRevenue - summaryCost)
	summaryMargin := 0.0
	if summaryRevenue > 0 {
		summaryMargin = roundPct2((summaryProfit / summaryRevenue) * 100.0)
	}

	common.ApiSuccess(c, gin.H{
		"models": rows,
		"summary": gin.H{
			"model_count":   len(rows),
			"total_calls":   summaryCalls,
			"total_tokens":  summaryTokens,
			"total_revenue": roundMoney6(summaryRevenue),
			"total_cost":    roundMoney6(summaryCost),
			"total_profit":  summaryProfit,
			"margin_pct":    summaryMargin,
		},
	})
}

func settingUsableGroupsSnapshot() map[string]string {
	raw := setting.GetUserUsableGroupsCopy()
	if raw == nil {
		return map[string]string{}
	}
	return raw
}

func GetUserCostAnalysis(c *gin.Context) {
	startTs, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTs, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))

	var users []*model.User
	if model.DB != nil {
		_ = model.DB.Omit("password", "access_token").Order("id ASC").Find(&users).Error
	}
	userByID := make(map[int]*model.User, len(users))
	for _, u := range users {
		if u != nil {
			userByID[u.Id] = u
		}
	}

	var channels []*model.Channel
	if model.DB != nil {
		_ = model.DB.Omit("key").Find(&channels).Error
	}
	chNameByID := make(map[int]string, len(channels))
	chByID := make(map[int]*model.Channel, len(channels))
	for _, ch := range channels {
		if ch != nil {
			chNameByID[ch.Id] = ch.Name
			chByID[ch.Id] = ch
		}
	}

	costMap := model.GetAllChannelModelCosts()
	if len(costMap) == 0 && model.DB != nil {
		model.RecalculateAllChannelModelCosts(false)
		costMap = model.GetAllChannelModelCosts()
	}
	syncCfg := model.GetModelPriceSyncConfig()

	consumeStats, topupLogMap, onlineMap, err := model.AggregateUserCostUsage(startTs, endTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	quotaUnit := common.QuotaPerUnit
	if quotaUnit <= 0 {
		quotaUnit = 500000
	}

	// Ensure all user IDs present in logs or topups are included
	userIDSet := make(map[int]struct{}, len(users))
	for _, u := range users {
		if u != nil && u.Id > 0 {
			userIDSet[u.Id] = struct{}{}
		}
	}
	for _, st := range consumeStats {
		if st.UserID > 0 {
			userIDSet[st.UserID] = struct{}{}
		}
	}
	for uid := range topupLogMap {
		if uid > 0 {
			userIDSet[uid] = struct{}{}
		}
	}
	for uid := range onlineMap {
		if uid > 0 {
			userIDSet[uid] = struct{}{}
		}
	}

	consumeByUser := make(map[int][]model.UserModelChannelLogStat)
	for _, st := range consumeStats {
		if st.UserID > 0 {
			consumeByUser[st.UserID] = append(consumeByUser[st.UserID], st)
		}
	}

	// Cache base pricing per model
	type baseModelPricing struct {
		quotaType int
		inPrice   float64
		outPrice  float64
	}
	basePriceCache := make(map[string]baseModelPricing)
	getBasePricing := func(mName string) baseModelPricing {
		if cached, ok := basePriceCache[mName]; ok {
			return cached
		}
		qt, _, inP, outP, _, _ := resolveBasePricingForModel(mName)
		res := baseModelPricing{quotaType: qt, inPrice: inP, outPrice: outP}
		basePriceCache[mName] = res
		return res
	}

	rows := make([]UserCostAnalysisRow, 0, len(userIDSet))
	var totalTopupUSD, totalConsumedUSD, totalCostUSD, totalProfitUSD float64
	var totalCalls, totalTokens int64

	for uid := range userIDSet {
		u := userByID[uid]
		username := fmt.Sprintf("user#%d", uid)
		displayName := ""
		uGroup := "default"
		uStatus := 1
		remainUSD := 0.0
		lifetimeUsedUSD := 0.0
		if u != nil {
			username = u.Username
			displayName = u.DisplayName
			if strings.TrimSpace(u.Group) != "" {
				uGroup = strings.TrimSpace(u.Group)
			}
			uStatus = u.Status
			remainUSD = roundMoney6(float64(u.Quota) / quotaUnit)
			lifetimeUsedUSD = roundMoney6(float64(u.UsedQuota) / quotaUnit)
		}

		uLogs := consumeByUser[uid]
		if u == nil && len(uLogs) > 0 && strings.TrimSpace(uLogs[0].Username) != "" {
			username = uLogs[0].Username
		}

		if keyword != "" {
			matchUser := strings.Contains(strings.ToLower(username), keyword) ||
				strings.Contains(strings.ToLower(displayName), keyword) ||
				strings.Contains(strconv.Itoa(uid), keyword)
			if !matchUser {
				continue
			}
		}

		onlineStat := onlineMap[uid]
		onlineTopupUSD := roundMoney6(float64(onlineStat.OnlineAmount))
		onlinePaidMoney := roundMoney6(onlineStat.OnlineMoney)
		systemTopupUSD := roundMoney6(float64(topupLogMap[uid]) / quotaUnit)
		userTopupTotal := roundMoney6(onlineTopupUSD + systemTopupUSD)
		// If no period filter is active and user's lifetime balance (remain + used) exceeds logged topups
		// (e.g. initial quota or direct DB edits), ensure topup_total reflects at least remain + used when logs are empty
		if startTs == 0 && endTs == 0 && userTopupTotal == 0 && (remainUSD+lifetimeUsedUSD) > 0 {
			userTopupTotal = roundMoney6(remainUSD + lifetimeUsedUSD)
		}

		var uCalls, uPrompt, uComp int64
		var uRev, uCost float64
		breakdowns := make([]UserModelChannelCostDetail, 0, len(uLogs))

		for _, st := range uLogs {
			rev := roundMoney6(float64(st.TotalQuota) / quotaUnit)
			var cost float64
			if st.FrozenCallCount >= st.CallCount && st.FrozenCallCount > 0 {
				cost = roundMoney6(st.FrozenCostUSD)
			} else {
				remCalls := max(st.CallCount-st.FrozenCallCount, 0)
				remPrompt := max(st.PromptTokens-st.FrozenPromptTokens, 0)
				remComp := max(st.CompletionTokens-st.FrozenCompletionTokens, 0)
				remCache := max(st.CacheTokens-st.FrozenCacheTokens, 0)
				remCacheWrite := max(st.CacheWriteTokens-st.FrozenCacheWriteTokens, 0)

				bp := getBasePricing(st.ModelName)
				entry, hasEntry := costMap[st.ChannelID][st.ModelName]
				archivedSnap, hasArchivedSnap := model.GetArchivedChannelSnapshot(st.ChannelID)
				if !hasEntry && hasArchivedSnap {
					if archivedEntry, ok := archivedSnap.ModelCosts[st.ModelName]; ok {
						entry = archivedEntry
						hasEntry = true
					}
				}
				effIn := bp.inPrice
				effOut := bp.outPrice
				if !hasEntry {
					entry.Model = st.ModelName
					if cacheR, ok := ratio_setting.GetCacheRatio(st.ModelName); ok && cacheR > 0 {
						entry.CacheRatio = cacheR
					}
					if createCacheR, ok := ratio_setting.GetCreateCacheRatio(st.ModelName); ok && createCacheR > 0 {
						entry.CreateCacheRatio = createCacheR
					}
					if ch := chByID[st.ChannelID]; ch != nil {
						curr, exRate, baseMult := model.ResolveChannelCurrencyAndMultiplier(ch, costMap[st.ChannelID], syncCfg)
						costScale := 1.0
						if baseMult > 0 && baseMult != 1.0 {
							costScale = baseMult
						}
						if ch.Type == constant.ChannelTypeNewAPI {
							unitMult := resolveUpstreamUnitToLocalUnitMultiplier(curr, exRate)
							if unitMult > 0 {
								costScale = roundMoney6(costScale * unitMult)
							}
						}
						effIn = roundMoney6(bp.inPrice * costScale)
						effOut = roundMoney6(bp.outPrice * costScale)
					} else if hasArchivedSnap {
						costScale := 1.0
						if archivedSnap.BaseMultiplier > 0 && archivedSnap.BaseMultiplier != 1.0 {
							costScale = archivedSnap.BaseMultiplier
						}
						if archivedSnap.ChannelType == constant.ChannelTypeNewAPI {
							unitMult := resolveUpstreamUnitToLocalUnitMultiplier(archivedSnap.Currency, archivedSnap.ExchangeRate)
							if unitMult > 0 {
								costScale = roundMoney6(costScale * unitMult)
							}
						}
						effIn = roundMoney6(bp.inPrice * costScale)
						effOut = roundMoney6(bp.outPrice * costScale)
					}
				}
				remCost := calculateUsageUpstreamCostUSD(entry, hasEntry, bp.quotaType, effIn, effOut, remCalls, remPrompt, remComp, remCache, remCacheWrite)
				topupMult := 1.0
				if chByID[st.ChannelID] != nil {
					topupMult = model.ResolveChannelTopupMultiplier(st.ChannelID, chByID[st.ChannelID], syncCfg)
				} else if hasArchivedSnap && archivedSnap.TopupMultiplier > 0 {
					topupMult = archivedSnap.TopupMultiplier
				} else {
					topupMult = model.ResolveChannelTopupMultiplier(st.ChannelID, nil, syncCfg)
				}
				if topupMult > 0 && math.Abs(topupMult-1.0) > 1e-6 {
					remCost = roundMoney6(remCost / topupMult)
				}
				cost = roundMoney6(st.FrozenCostUSD + remCost)
			}
			prof := roundMoney6(rev - cost)
			mPct := 0.0
			if rev > 0 {
				mPct = roundPct2((prof / rev) * 100.0)
			}

			chName := chNameByID[st.ChannelID]
			if chName == "" {
				if archivedName := model.GetArchivedChannelName(st.ChannelID); archivedName != "" {
					chName = archivedName + " (Deleted)"
				} else {
					chName = fmt.Sprintf("Channel #%d", st.ChannelID)
				}
			}

			uCalls += st.CallCount
			uPrompt += st.PromptTokens
			uComp += st.CompletionTokens
			uRev += rev
			uCost += cost

			breakdowns = append(breakdowns, UserModelChannelCostDetail{
				Model:            st.ModelName,
				ChannelID:        st.ChannelID,
				ChannelName:      chName,
				CallCount:        st.CallCount,
				PromptTokens:     st.PromptTokens,
				CompletionTokens: st.CompletionTokens,
				TotalTokens:      st.PromptTokens + st.CompletionTokens,
				RevenueUSD:       rev,
				CostUSD:          cost,
				ProfitUSD:        prof,
				MarginPct:        mPct,
			})
		}

		sort.Slice(breakdowns, func(i, j int) bool {
			if breakdowns[i].RevenueUSD != breakdowns[j].RevenueUSD {
				return breakdowns[i].RevenueUSD > breakdowns[j].RevenueUSD
			}
			return breakdowns[i].CallCount > breakdowns[j].CallCount
		})

		uRev = roundMoney6(uRev)
		uCost = roundMoney6(uCost)
		consumedUSD := uRev
		if startTs == 0 && endTs == 0 && consumedUSD == 0 && lifetimeUsedUSD > 0 {
			consumedUSD = lifetimeUsedUSD
			uRev = lifetimeUsedUSD
		}
		uProfit := roundMoney6(uRev - uCost)
		uMargin := 0.0
		if uRev > 0 {
			uMargin = roundPct2((uProfit / uRev) * 100.0)
		}

		totalTopupUSD += userTopupTotal
		totalConsumedUSD += consumedUSD
		totalCostUSD += uCost
		totalProfitUSD += uProfit
		totalCalls += uCalls
		totalTokens += uPrompt + uComp

		rows = append(rows, UserCostAnalysisRow{
			UserID:           uid,
			Username:         username,
			DisplayName:      displayName,
			Group:            uGroup,
			Status:           uStatus,
			RemainQuotaUSD:   remainUSD,
			LifetimeUsedUSD:  lifetimeUsedUSD,
			TopupTotalUSD:    userTopupTotal,
			OnlineTopupUSD:   onlineTopupUSD,
			OnlinePaidMoney:  onlinePaidMoney,
			SystemTopupUSD:   systemTopupUSD,
			ConsumedUSD:      consumedUSD,
			CallCount:        uCalls,
			PromptTokens:     uPrompt,
			CompletionTokens: uComp,
			TotalTokens:      uPrompt + uComp,
			RevenueUSD:       uRev,
			CostUSD:          uCost,
			GrossProfitUSD:   uProfit,
			GrossMarginPct:   uMargin,
			ModelBreakdowns:  breakdowns,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ConsumedUSD != rows[j].ConsumedUSD {
			return rows[i].ConsumedUSD > rows[j].ConsumedUSD
		}
		if rows[i].TopupTotalUSD != rows[j].TopupTotalUSD {
			return rows[i].TopupTotalUSD > rows[j].TopupTotalUSD
		}
		return rows[i].UserID < rows[j].UserID
	})

	overallMargin := 0.0
	if totalConsumedUSD > 0 {
		overallMargin = roundPct2(((totalConsumedUSD - totalCostUSD) / totalConsumedUSD) * 100.0)
	}

	common.ApiSuccess(c, gin.H{
		"users": rows,
		"summary": gin.H{
			"user_count":         len(rows),
			"total_topup_usd":    roundMoney6(totalTopupUSD),
			"total_consumed_usd": roundMoney6(totalConsumedUSD),
			"total_revenue_usd":  roundMoney6(totalConsumedUSD),
			"total_cost_usd":     roundMoney6(totalCostUSD),
			"total_profit_usd":   roundMoney6(totalConsumedUSD - totalCostUSD),
			"overall_margin_pct": overallMargin,
			"total_calls":        totalCalls,
			"total_tokens":       totalTokens,
		},
	})
}

var logFrtMsRegex = regexp.MustCompile(`"frt"\s*:\s*([0-9]+(?:\.[0-9]+)?)`)

func parseLogFrtMs(other string) float64 {
	if !strings.Contains(other, `"frt"`) {
		return 0
	}
	if m := logFrtMsRegex.FindStringSubmatch(other); len(m) == 2 {
		if val, err := strconv.ParseFloat(m[1], 64); err == nil && val > 0 && !math.IsNaN(val) && !math.IsInf(val, 0) {
			return val
		}
	}
	return 0
}

type HealthPercentiles struct {
	SampleCount int     `json:"sample_count"`
	P5Low       float64 `json:"p5_low"`
	Median      float64 `json:"median"`
	Trimmed90   float64 `json:"trimmed_90"`
	P95High     float64 `json:"p95_high"`
	Mean        float64 `json:"mean"`
}

func computeHealthDistribution(samples []float64) HealthPercentiles {
	n := len(samples)
	if n == 0 {
		return HealthPercentiles{}
	}
	sorted := make([]float64, n)
	copy(sorted, samples)
	slices.Sort(sorted)

	percentileAt := func(p float64) float64 {
		if n == 1 {
			return sorted[0]
		}
		pos := p * float64(n-1)
		lower := int(math.Floor(pos))
		upper := int(math.Ceil(pos))
		if lower < 0 {
			lower = 0
		}
		if upper >= n {
			upper = n - 1
		}
		if lower == upper {
			return sorted[lower]
		}
		weight := pos - float64(lower)
		return sorted[lower]*(1.0-weight) + sorted[upper]*weight
	}

	var sumAll float64
	for _, v := range sorted {
		sumAll += v
	}
	meanAll := sumAll / float64(n)

	p5 := percentileAt(0.05)
	median := percentileAt(0.50)
	p95 := percentileAt(0.95)

	trimCount := int(math.Floor(float64(n) * 0.05))
	trimmedSlice := sorted
	if trimCount > 0 && n-2*trimCount > 0 {
		trimmedSlice = sorted[trimCount : n-trimCount]
	}
	var sumTrimmed float64
	for _, v := range trimmedSlice {
		sumTrimmed += v
	}
	trimmedMean := median
	if len(trimmedSlice) > 0 {
		trimmedMean = sumTrimmed / float64(len(trimmedSlice))
	}

	return HealthPercentiles{
		SampleCount: n,
		P5Low:       roundPct2(p5),
		Median:      roundPct2(median),
		Trimmed90:   roundPct2(trimmedMean),
		P95High:     roundPct2(p95),
		Mean:        roundPct2(meanAll),
	}
}

type ModelChannelHealthDetail struct {
	ChannelID        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ChannelType      int     `json:"channel_type"`
	ChannelStatus    int     `json:"channel_status"`
	Model            string  `json:"model"`
	RequestCount     int64   `json:"request_count"`
	SuccessCount     int64   `json:"success_count"`
	ErrorCount       int64   `json:"error_count"`
	Availability     float64 `json:"availability"`
	AvgPromptTokens  float64 `json:"avg_prompt_tokens"`
	AvgOutputTokens  float64 `json:"avg_output_tokens"`
	AvgTtftSec       float64 `json:"avg_ttft_sec"`
	MedianTtftSec    float64 `json:"median_ttft_sec"`
	P95TtftSec       float64 `json:"p95_ttft_sec"`
	PrefillTpsAvg    float64 `json:"prefill_tps_avg"`
	PrefillTpsMedian float64 `json:"prefill_tps_median"`
	DecodeTpsP5Low   float64 `json:"decode_tps_p5_low"`
	DecodeTpsMedian  float64 `json:"decode_tps_median"`
	DecodeTpsTrim90  float64 `json:"decode_tps_trimmed_90"`
	DecodeTpsP95High float64 `json:"decode_tps_p95_high"`
	DecodeTpsAvg     float64 `json:"decode_tps_avg"`
	TotalTpsP5Low    float64 `json:"total_tps_p5_low"`
	TotalTpsMedian   float64 `json:"total_tps_median"`
	TotalTpsTrim90   float64 `json:"total_tps_trimmed_90"`
	TotalTpsP95High  float64 `json:"total_tps_p95_high"`
	TotalTpsAvg      float64 `json:"total_tps_avg"`
}

type ModelHealthAnalysisRow struct {
	Model            string                     `json:"model"`
	ChannelCount     int                        `json:"channel_count"`
	RequestCount     int64                      `json:"request_count"`
	SuccessCount     int64                      `json:"success_count"`
	ErrorCount       int64                      `json:"error_count"`
	Availability     float64                    `json:"availability"`
	AvgPromptTokens  float64                    `json:"avg_prompt_tokens"`
	AvgOutputTokens  float64                    `json:"avg_output_tokens"`
	AvgTtftSec       float64                    `json:"avg_ttft_sec"`
	MedianTtftSec    float64                    `json:"median_ttft_sec"`
	P95TtftSec       float64                    `json:"p95_ttft_sec"`
	PrefillTpsAvg    float64                    `json:"prefill_tps_avg"`
	PrefillTpsMedian float64                    `json:"prefill_tps_median"`
	DecodeTpsP5Low   float64                    `json:"decode_tps_p5_low"`
	DecodeTpsMedian  float64                    `json:"decode_tps_median"`
	DecodeTpsTrim90  float64                    `json:"decode_tps_trimmed_90"`
	DecodeTpsP95High float64                    `json:"decode_tps_p95_high"`
	DecodeTpsAvg     float64                    `json:"decode_tps_avg"`
	TotalTpsP5Low    float64                    `json:"total_tps_p5_low"`
	TotalTpsMedian   float64                    `json:"total_tps_median"`
	TotalTpsTrim90   float64                    `json:"total_tps_trimmed_90"`
	TotalTpsP95High  float64                    `json:"total_tps_p95_high"`
	TotalTpsAvg      float64                    `json:"total_tps_avg"`
	Channels         []ModelChannelHealthDetail `json:"channels"`
}

type ChannelHealthSummaryRow struct {
	ChannelID        int                        `json:"channel_id"`
	ChannelName      string                     `json:"channel_name"`
	ChannelType      int                        `json:"channel_type"`
	ChannelStatus    int                        `json:"channel_status"`
	ModelCount       int                        `json:"model_count"`
	RequestCount     int64                      `json:"request_count"`
	SuccessCount     int64                      `json:"success_count"`
	ErrorCount       int64                      `json:"error_count"`
	Availability     float64                    `json:"availability"`
	AvgPromptTokens  float64                    `json:"avg_prompt_tokens"`
	AvgOutputTokens  float64                    `json:"avg_output_tokens"`
	AvgTtftSec       float64                    `json:"avg_ttft_sec"`
	MedianTtftSec    float64                    `json:"median_ttft_sec"`
	P95TtftSec       float64                    `json:"p95_ttft_sec"`
	PrefillTpsAvg    float64                    `json:"prefill_tps_avg"`
	PrefillTpsMedian float64                    `json:"prefill_tps_median"`
	DecodeTpsP5Low   float64                    `json:"decode_tps_p5_low"`
	DecodeTpsMedian  float64                    `json:"decode_tps_median"`
	DecodeTpsTrim90  float64                    `json:"decode_tps_trimmed_90"`
	DecodeTpsP95High float64                    `json:"decode_tps_p95_high"`
	DecodeTpsAvg     float64                    `json:"decode_tps_avg"`
	TotalTpsP5Low    float64                    `json:"total_tps_p5_low"`
	TotalTpsMedian   float64                    `json:"total_tps_median"`
	TotalTpsTrim90   float64                    `json:"total_tps_trimmed_90"`
	TotalTpsP95High  float64                    `json:"total_tps_p95_high"`
	TotalTpsAvg      float64                    `json:"total_tps_avg"`
	Models           []ModelChannelHealthDetail `json:"models"`
}

type ModelHealthSummary struct {
	ModelCount       int     `json:"model_count"`
	ChannelCount     int     `json:"channel_count"`
	RequestCount     int64   `json:"request_count"`
	SuccessCount     int64   `json:"success_count"`
	ErrorCount       int64   `json:"error_count"`
	Availability     float64 `json:"availability"`
	AvgTtftSec       float64 `json:"avg_ttft_sec"`
	MedianTtftSec    float64 `json:"median_ttft_sec"`
	PrefillTpsAvg    float64 `json:"prefill_tps_avg"`
	PrefillTpsMedian float64 `json:"prefill_tps_median"`
	DecodeTpsP5Low   float64 `json:"decode_tps_p5_low"`
	DecodeTpsMedian  float64 `json:"decode_tps_median"`
	DecodeTpsTrim90  float64 `json:"decode_tps_trimmed_90"`
	DecodeTpsP95High float64 `json:"decode_tps_p95_high"`
	DecodeTpsAvg     float64 `json:"decode_tps_avg"`
	TotalTpsP5Low    float64 `json:"total_tps_p5_low"`
	TotalTpsMedian   float64 `json:"total_tps_median"`
	TotalTpsTrim90   float64 `json:"total_tps_trimmed_90"`
	TotalTpsP95High  float64 `json:"total_tps_p95_high"`
	TotalTpsAvg      float64 `json:"total_tps_avg"`
}

type healthSampleCollector struct {
	successCount int64
	errorCount   int64
	promptSum    int64
	outputSum    int64
	ttftSec      []float64
	prefillTps   []float64
	decodeTps    []float64
	totalTps     []float64
}

func (hc *healthSampleCollector) addConsumeSample(promptTokens int, completionTokens int, useTimeSec int, isStream bool, frtMs float64) {
	hc.successCount++
	if promptTokens > 0 {
		hc.promptSum += int64(promptTokens)
	}
	if completionTokens > 0 {
		hc.outputSum += int64(completionTokens)
	}

	totalDurSec := float64(useTimeSec)
	ttftS := 0.0
	if isStream && frtMs > 0 {
		ttftS = frtMs / 1000.0
		if ttftS <= 600.0 {
			hc.ttftSec = append(hc.ttftSec, ttftS)
			if promptTokens > 0 && ttftS >= 0.01 {
				pTps := float64(promptTokens) / ttftS
				if pTps > 0 && pTps <= 5_000_000 {
					hc.prefillTps = append(hc.prefillTps, pTps)
				}
			}
		}
	}

	if completionTokens > 0 {
		if totalDurSec <= 0 && ttftS > 0 {
			totalDurSec = ttftS + 0.25
		}
		if totalDurSec > 0 {
			tTps := float64(completionTokens) / totalDurSec
			if tTps > 0 && tTps <= 100_000 {
				hc.totalTps = append(hc.totalTps, tTps)
			}
			decodeDurSec := totalDurSec
			if ttftS > 0 && totalDurSec > ttftS {
				decodeDurSec = max(totalDurSec-ttftS, 0.1)
			}
			dTps := float64(completionTokens) / decodeDurSec
			if dTps > 0 && dTps <= 100_000 {
				hc.decodeTps = append(hc.decodeTps, dTps)
			}
		}
	}
}

func GetModelHealthAnalysis(c *gin.Context) {
	startTs, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTs, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))

	var channels []*model.Channel
	if model.DB != nil {
		_ = model.DB.Omit("key").Order("id ASC").Find(&channels).Error
	}
	chByID := make(map[int]*model.Channel, len(channels))
	for _, ch := range channels {
		if ch != nil {
			chByID[ch.Id] = ch
		}
	}

	logDB := model.LOG_DB
	if logDB == nil {
		logDB = model.DB
	}

	type rawHealthLog struct {
		Type             int    `gorm:"column:type"`
		ModelName        string `gorm:"column:model_name"`
		ChannelID        int    `gorm:"column:channel_id"`
		PromptTokens     int    `gorm:"column:prompt_tokens"`
		CompletionTokens int    `gorm:"column:completion_tokens"`
		UseTime          int    `gorm:"column:use_time"`
		IsStream         bool   `gorm:"column:is_stream"`
		Other            string `gorm:"column:other"`
	}

	var logRows []rawHealthLog
	if logDB != nil {
		tx := logDB.Model(&model.Log{}).
			Select("type, model_name, channel_id, prompt_tokens, completion_tokens, use_time, is_stream, other").
			Where("type IN ?", []int{model.LogTypeConsume, model.LogTypeError})
		if startTs > 0 {
			tx = tx.Where("created_at >= ?", startTs)
		}
		if endTs > 0 {
			tx = tx.Where("created_at <= ?", endTs)
		}
		_ = tx.Order("id DESC").Limit(50000).Scan(&logRows).Error
	}

	type pairKey struct {
		model     string
		channelID int
	}

	pairCollectors := make(map[pairKey]*healthSampleCollector)
	modelCollectors := make(map[string]*healthSampleCollector)
	channelCollectors := make(map[int]*healthSampleCollector)
	globalCollector := &healthSampleCollector{}

	getPairCollector := func(m string, chID int) *healthSampleCollector {
		k := pairKey{model: m, channelID: chID}
		col, ok := pairCollectors[k]
		if !ok {
			col = &healthSampleCollector{}
			pairCollectors[k] = col
		}
		return col
	}
	getModelCollector := func(m string) *healthSampleCollector {
		col, ok := modelCollectors[m]
		if !ok {
			col = &healthSampleCollector{}
			modelCollectors[m] = col
		}
		return col
	}
	getChannelCollector := func(chID int) *healthSampleCollector {
		col, ok := channelCollectors[chID]
		if !ok {
			col = &healthSampleCollector{}
			channelCollectors[chID] = col
		}
		return col
	}

	modelSet := make(map[string]struct{})
	modelChannelsSet := make(map[string]map[int]struct{})
	ensureModelChannel := func(m string, chID int) {
		m = strings.TrimSpace(m)
		if m == "" || chID <= 0 {
			return
		}
		modelSet[m] = struct{}{}
		if modelChannelsSet[m] == nil {
			modelChannelsSet[m] = make(map[int]struct{})
		}
		modelChannelsSet[m][chID] = struct{}{}
	}

	for _, ch := range channels {
		if ch == nil {
			continue
		}
		for _, m := range ch.GetModels() {
			ensureModelChannel(m, ch.Id)
		}
	}

	for _, row := range logRows {
		m := strings.TrimSpace(row.ModelName)
		if m == "" || row.ChannelID <= 0 {
			continue
		}
		ensureModelChannel(m, row.ChannelID)
		pc := getPairCollector(m, row.ChannelID)
		mc := getModelCollector(m)
		cc := getChannelCollector(row.ChannelID)

		if row.Type == model.LogTypeError {
			pc.errorCount++
			mc.errorCount++
			cc.errorCount++
			globalCollector.errorCount++
			continue
		}
		frtMs := parseLogFrtMs(row.Other)
		pc.addConsumeSample(row.PromptTokens, row.CompletionTokens, row.UseTime, row.IsStream, frtMs)
		mc.addConsumeSample(row.PromptTokens, row.CompletionTokens, row.UseTime, row.IsStream, frtMs)
		cc.addConsumeSample(row.PromptTokens, row.CompletionTokens, row.UseTime, row.IsStream, frtMs)
		globalCollector.addConsumeSample(row.PromptTokens, row.CompletionTokens, row.UseTime, row.IsStream, frtMs)
	}

	// Supplement with in-memory channel telemetry when no historical logs exist for a pair
	for m, chSet := range modelChannelsSet {
		for chID := range chSet {
			pc := getPairCollector(m, chID)
			if pc.successCount == 0 && pc.errorCount == 0 {
				snap := model.GetChannelModelTelemetry(chID, m)
				if snap.RequestCount > 0 {
					errCnt := max(snap.RequestCount-snap.SuccessCount, 0)
					pc.successCount = snap.SuccessCount
					pc.errorCount = errCnt
					mc := getModelCollector(m)
					cc := getChannelCollector(chID)
					mc.successCount += snap.SuccessCount
					mc.errorCount += errCnt
					cc.successCount += snap.SuccessCount
					cc.errorCount += errCnt
					globalCollector.successCount += snap.SuccessCount
					globalCollector.errorCount += errCnt
					if snap.AvgTtftSec > 0 {
						pc.ttftSec = append(pc.ttftSec, snap.AvgTtftSec)
						mc.ttftSec = append(mc.ttftSec, snap.AvgTtftSec)
						cc.ttftSec = append(cc.ttftSec, snap.AvgTtftSec)
						globalCollector.ttftSec = append(globalCollector.ttftSec, snap.AvgTtftSec)
					}
					if snap.TpsAvg > 0 {
						pc.decodeTps = append(pc.decodeTps, snap.TpsAvg)
						pc.totalTps = append(pc.totalTps, snap.TpsAvg)
						mc.decodeTps = append(mc.decodeTps, snap.TpsAvg)
						mc.totalTps = append(mc.totalTps, snap.TpsAvg)
						cc.decodeTps = append(cc.decodeTps, snap.TpsAvg)
						cc.totalTps = append(cc.totalTps, snap.TpsAvg)
						globalCollector.decodeTps = append(globalCollector.decodeTps, snap.TpsAvg)
						globalCollector.totalTps = append(globalCollector.totalTps, snap.TpsAvg)
					}
				}
			}
		}
	}

	buildDetailFromCollector := func(m string, chID int, hc *healthSampleCollector) ModelChannelHealthDetail {
		ch := chByID[chID]
		chName := fmt.Sprintf("Channel #%d", chID)
		chType := 0
		chStatus := 1
		if ch != nil {
			chName = ch.Name
			chType = ch.Type
			chStatus = ch.Status
		}
		reqCount := hc.successCount + hc.errorCount
		avail := 100.0
		if reqCount > 0 {
			avail = roundPct2((float64(hc.successCount) / float64(reqCount)) * 100.0)
		}
		avgPrompt := 0.0
		avgOutput := 0.0
		if hc.successCount > 0 {
			avgPrompt = roundPct2(float64(hc.promptSum) / float64(hc.successCount))
			avgOutput = roundPct2(float64(hc.outputSum) / float64(hc.successCount))
		}
		ttftDist := computeHealthDistribution(hc.ttftSec)
		prefillDist := computeHealthDistribution(hc.prefillTps)
		decodeDist := computeHealthDistribution(hc.decodeTps)
		totalDist := computeHealthDistribution(hc.totalTps)

		return ModelChannelHealthDetail{
			ChannelID:        chID,
			ChannelName:      chName,
			ChannelType:      chType,
			ChannelStatus:    chStatus,
			Model:            m,
			RequestCount:     reqCount,
			SuccessCount:     hc.successCount,
			ErrorCount:       hc.errorCount,
			Availability:     avail,
			AvgPromptTokens:  avgPrompt,
			AvgOutputTokens:  avgOutput,
			AvgTtftSec:       ttftDist.Mean,
			MedianTtftSec:    ttftDist.Median,
			P95TtftSec:       ttftDist.P95High,
			PrefillTpsAvg:    prefillDist.Mean,
			PrefillTpsMedian: prefillDist.Trimmed90,
			DecodeTpsP5Low:   decodeDist.P5Low,
			DecodeTpsMedian:  decodeDist.Median,
			DecodeTpsTrim90:  decodeDist.Trimmed90,
			DecodeTpsP95High: decodeDist.P95High,
			DecodeTpsAvg:     decodeDist.Mean,
			TotalTpsP5Low:    totalDist.P5Low,
			TotalTpsMedian:   totalDist.Median,
			TotalTpsTrim90:   totalDist.Trimmed90,
			TotalTpsP95High:  totalDist.P95High,
			TotalTpsAvg:      totalDist.Mean,
		}
	}

	modelRows := make([]ModelHealthAnalysisRow, 0, len(modelSet))
	channelDetailsMap := make(map[int][]ModelChannelHealthDetail)

	for m := range modelSet {
		chSet := modelChannelsSet[m]
		mc := getModelCollector(m)

		chDetails := make([]ModelChannelHealthDetail, 0, len(chSet))
		matchesKeyword := keyword == "" || strings.Contains(strings.ToLower(m), keyword)

		for chID := range chSet {
			pc := getPairCollector(m, chID)
			detail := buildDetailFromCollector(m, chID, pc)
			if !matchesKeyword && (strings.Contains(strings.ToLower(detail.ChannelName), keyword) || strings.Contains(strconv.Itoa(chID), keyword)) {
				matchesKeyword = true
			}
			chDetails = append(chDetails, detail)
			channelDetailsMap[chID] = append(channelDetailsMap[chID], detail)
		}

		if !matchesKeyword {
			continue
		}

		sort.Slice(chDetails, func(i, j int) bool {
			if chDetails[i].RequestCount != chDetails[j].RequestCount {
				return chDetails[i].RequestCount > chDetails[j].RequestCount
			}
			if chDetails[i].DecodeTpsTrim90 != chDetails[j].DecodeTpsTrim90 {
				return chDetails[i].DecodeTpsTrim90 > chDetails[j].DecodeTpsTrim90
			}
			return chDetails[i].ChannelID < chDetails[j].ChannelID
		})

		mSummary := buildDetailFromCollector(m, 0, mc)
		modelRows = append(modelRows, ModelHealthAnalysisRow{
			Model:            m,
			ChannelCount:     len(chDetails),
			RequestCount:     mSummary.RequestCount,
			SuccessCount:     mSummary.SuccessCount,
			ErrorCount:       mSummary.ErrorCount,
			Availability:     mSummary.Availability,
			AvgPromptTokens:  mSummary.AvgPromptTokens,
			AvgOutputTokens:  mSummary.AvgOutputTokens,
			AvgTtftSec:       mSummary.AvgTtftSec,
			MedianTtftSec:    mSummary.MedianTtftSec,
			P95TtftSec:       mSummary.P95TtftSec,
			PrefillTpsAvg:    mSummary.PrefillTpsAvg,
			PrefillTpsMedian: mSummary.PrefillTpsMedian,
			DecodeTpsP5Low:   mSummary.DecodeTpsP5Low,
			DecodeTpsMedian:  mSummary.DecodeTpsMedian,
			DecodeTpsTrim90:  mSummary.DecodeTpsTrim90,
			DecodeTpsP95High: mSummary.DecodeTpsP95High,
			DecodeTpsAvg:     mSummary.DecodeTpsAvg,
			TotalTpsP5Low:    mSummary.TotalTpsP5Low,
			TotalTpsMedian:   mSummary.TotalTpsMedian,
			TotalTpsTrim90:   mSummary.TotalTpsTrim90,
			TotalTpsP95High:  mSummary.TotalTpsP95High,
			TotalTpsAvg:      mSummary.TotalTpsAvg,
			Channels:         chDetails,
		})
	}

	sort.Slice(modelRows, func(i, j int) bool {
		if modelRows[i].RequestCount != modelRows[j].RequestCount {
			return modelRows[i].RequestCount > modelRows[j].RequestCount
		}
		if modelRows[i].DecodeTpsTrim90 != modelRows[j].DecodeTpsTrim90 {
			return modelRows[i].DecodeTpsTrim90 > modelRows[j].DecodeTpsTrim90
		}
		if modelRows[i].ChannelCount != modelRows[j].ChannelCount {
			return modelRows[i].ChannelCount > modelRows[j].ChannelCount
		}
		return modelRows[i].Model < modelRows[j].Model
	})

	chIDSet := make(map[int]struct{}, len(chByID)+len(channelDetailsMap))
	for chID := range chByID {
		if chID > 0 {
			chIDSet[chID] = struct{}{}
		}
	}
	for chID := range channelDetailsMap {
		if chID > 0 {
			chIDSet[chID] = struct{}{}
		}
	}

	channelRows := make([]ChannelHealthSummaryRow, 0, len(chIDSet))
	for chID := range chIDSet {
		mDetails := channelDetailsMap[chID]
		if mDetails == nil {
			mDetails = []ModelChannelHealthDetail{}
		}
		cc := getChannelCollector(chID)
		cSummary := buildDetailFromCollector("", chID, cc)

		if keyword != "" {
			matchCh := strings.Contains(strings.ToLower(cSummary.ChannelName), keyword) || strings.Contains(strconv.Itoa(chID), keyword)
			if !matchCh {
				for _, md := range mDetails {
					if strings.Contains(strings.ToLower(md.Model), keyword) {
						matchCh = true
						break
					}
				}
			}
			if !matchCh {
				continue
			}
		}

		sort.Slice(mDetails, func(i, j int) bool {
			if mDetails[i].RequestCount != mDetails[j].RequestCount {
				return mDetails[i].RequestCount > mDetails[j].RequestCount
			}
			if mDetails[i].DecodeTpsTrim90 != mDetails[j].DecodeTpsTrim90 {
				return mDetails[i].DecodeTpsTrim90 > mDetails[j].DecodeTpsTrim90
			}
			return mDetails[i].Model < mDetails[j].Model
		})

		channelRows = append(channelRows, ChannelHealthSummaryRow{
			ChannelID:        chID,
			ChannelName:      cSummary.ChannelName,
			ChannelType:      cSummary.ChannelType,
			ChannelStatus:    cSummary.ChannelStatus,
			ModelCount:       len(mDetails),
			RequestCount:     cSummary.RequestCount,
			SuccessCount:     cSummary.SuccessCount,
			ErrorCount:       cSummary.ErrorCount,
			Availability:     cSummary.Availability,
			AvgPromptTokens:  cSummary.AvgPromptTokens,
			AvgOutputTokens:  cSummary.AvgOutputTokens,
			AvgTtftSec:       cSummary.AvgTtftSec,
			MedianTtftSec:    cSummary.MedianTtftSec,
			P95TtftSec:       cSummary.P95TtftSec,
			PrefillTpsAvg:    cSummary.PrefillTpsAvg,
			PrefillTpsMedian: cSummary.PrefillTpsMedian,
			DecodeTpsP5Low:   cSummary.DecodeTpsP5Low,
			DecodeTpsMedian:  cSummary.DecodeTpsMedian,
			DecodeTpsTrim90:  cSummary.DecodeTpsTrim90,
			DecodeTpsP95High: cSummary.DecodeTpsP95High,
			DecodeTpsAvg:     cSummary.DecodeTpsAvg,
			TotalTpsP5Low:    cSummary.TotalTpsP5Low,
			TotalTpsMedian:   cSummary.TotalTpsMedian,
			TotalTpsTrim90:   cSummary.TotalTpsTrim90,
			TotalTpsP95High:  cSummary.TotalTpsP95High,
			TotalTpsAvg:      cSummary.TotalTpsAvg,
			Models:           mDetails,
		})
	}

	sort.Slice(channelRows, func(i, j int) bool {
		if channelRows[i].RequestCount != channelRows[j].RequestCount {
			return channelRows[i].RequestCount > channelRows[j].RequestCount
		}
		if channelRows[i].DecodeTpsTrim90 != channelRows[j].DecodeTpsTrim90 {
			return channelRows[i].DecodeTpsTrim90 > channelRows[j].DecodeTpsTrim90
		}
		return channelRows[i].ChannelID < channelRows[j].ChannelID
	})

	gSummary := buildDetailFromCollector("", 0, globalCollector)
	summary := ModelHealthSummary{
		ModelCount:       len(modelRows),
		ChannelCount:     len(channelRows),
		RequestCount:     gSummary.RequestCount,
		SuccessCount:     gSummary.SuccessCount,
		ErrorCount:       gSummary.ErrorCount,
		Availability:     gSummary.Availability,
		AvgTtftSec:       gSummary.AvgTtftSec,
		MedianTtftSec:    gSummary.MedianTtftSec,
		PrefillTpsAvg:    gSummary.PrefillTpsAvg,
		PrefillTpsMedian: gSummary.PrefillTpsMedian,
		DecodeTpsP5Low:   gSummary.DecodeTpsP5Low,
		DecodeTpsMedian:  gSummary.DecodeTpsMedian,
		DecodeTpsTrim90:  gSummary.DecodeTpsTrim90,
		DecodeTpsP95High: gSummary.DecodeTpsP95High,
		DecodeTpsAvg:     gSummary.DecodeTpsAvg,
		TotalTpsP5Low:    gSummary.TotalTpsP5Low,
		TotalTpsMedian:   gSummary.TotalTpsMedian,
		TotalTpsTrim90:   gSummary.TotalTpsTrim90,
		TotalTpsP95High:  gSummary.TotalTpsP95High,
		TotalTpsAvg:      gSummary.TotalTpsAvg,
	}

	common.ApiSuccess(c, gin.H{
		"models":   modelRows,
		"channels": channelRows,
		"summary":  summary,
	})
}
