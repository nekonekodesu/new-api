package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type UpstreamSourceBrief struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	BaseURL      string  `json:"base_url,omitempty"`
	SyncMode     string  `json:"sync_mode,omitempty"`
	Enabled      bool    `json:"enabled"`
	IsDirect     bool    `json:"is_direct"`
	Currency     string  `json:"currency"`
	ExchangeRate float64 `json:"exchange_rate"`
	BalanceUSD   float64 `json:"balance_usd"`
	UsedQuotaUSD float64 `json:"used_quota_usd"`
	ChannelCount int     `json:"channel_count"`
}

type GroupSchedulingSummary struct {
	Group             string                            `json:"group"`
	Description       string                            `json:"description"`
	Ratio             float64                           `json:"ratio"`
	Mode              string                            `json:"mode"`
	EnabledChannelIDs []int                             `json:"enabled_channel_ids"`
	CostControl       model.GroupCostControlPolicy      `json:"cost_control"`
	RealtimeProfit    model.GroupRealtimeProfitSnapshot `json:"realtime_profit"`
}

type ChannelBrief struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Type               int     `json:"type"`
	Status             int     `json:"status"`
	Group              string  `json:"group"`
	Models             string  `json:"models"`
	Priority           int64   `json:"priority"`
	Weight             int     `json:"weight"`
	Tag                string  `json:"tag"`
	BaseURL            string  `json:"base_url"`
	Balance            float64 `json:"balance"`
	BalanceUpdatedTime int64   `json:"balance_updated_time"`
	UpstreamSourceID   string  `json:"upstream_source_id"`
	UpstreamSourceName string  `json:"upstream_source_name"`
	UpstreamGroup      string  `json:"upstream_group,omitempty"`
	BaseMultiplier     float64 `json:"base_multiplier"`
	Currency           string  `json:"currency"`
	FromUpstream       bool    `json:"from_upstream"`
}

type ModelPriceTableRow struct {
	ChannelID        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ChannelType      int     `json:"channel_type"`
	ChannelStatus    int     `json:"channel_status"`
	Model            string  `json:"model"`
	InputCost        float64 `json:"input_cost"`
	OutputCost       float64 `json:"output_cost"`
	RawInputCost     float64 `json:"raw_input_cost"`
	RawOutputCost    float64 `json:"raw_output_cost"`
	ModelRatio       float64 `json:"model_ratio"`
	CompletionRatio  float64 `json:"completion_ratio"`
	CacheRatio       float64 `json:"cache_ratio"`
	CreateCacheRatio float64 `json:"create_cache_ratio"`
	ModelPrice       float64 `json:"model_price"`
	RawModelPrice    float64 `json:"raw_model_price"`
	QuotaType        int     `json:"quota_type"`
	BaseMultiplier   float64 `json:"base_multiplier"`
	Currency         string  `json:"currency"`
	ExchangeRate     float64 `json:"exchange_rate"`
	FromUpstream     bool    `json:"from_upstream"`
	Availability     float64 `json:"availability"`
	TpsMin           float64 `json:"tps_min"`
	TpsAvg           float64 `json:"tps_avg"`
	TpsMax           float64 `json:"tps_max"`
	AvgTtft          float64 `json:"avg_ttft"`
	IsSelectedPrice  bool    `json:"is_selected_price"`
}

type ChannelUpstreamSyncItemResult struct {
	ChannelID          int                `json:"channel_id"`
	ChannelName        string             `json:"channel_name"`
	ChannelType        int                `json:"channel_type"`
	Success            bool               `json:"success"`
	Error              string             `json:"error,omitempty"`
	SyncedModelCount   int                `json:"synced_model_count"`
	AddedModels        []string           `json:"added_models"`
	RemovedModels      []string           `json:"removed_models"`
	SyncedPriceCount   int                `json:"synced_price_count"`
	DetectedCurrency   string             `json:"detected_currency"`
	ExchangeRate       float64            `json:"exchange_rate"`
	BaseMultiplier     float64            `json:"base_multiplier"`
	UpstreamGroup      string             `json:"upstream_group"`
	AvailableGroups    map[string]float64 `json:"available_groups,omitempty"`
	Balance            float64            `json:"balance"`
	BalanceUpdatedTime int64              `json:"balance_updated_time"`
}

var (
	cronSyncOnce      sync.Once
	cronSyncRunningMu sync.Mutex
	cronJobRunningMap sync.Map // map[string]bool
)

func InitChannelCronSync() {
	// Only the master node runs the scheduled sync; slave nodes receive the
	// resulting options through the regular option sync.
	if !common.IsMasterNode {
		return
	}
	cronSyncOnce.Do(func() {
		go func() {
			if model.DB != nil {
				model.RecalculateAllChannelModelCosts(true)
				_, _ = applyAuthoritativeModelPricesToSystem(model.GetModelPriceSyncConfig())
			}
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				now := time.Now().Unix()
				jobs := model.GetCronJobs()
				for _, job := range jobs {
					if !job.Enabled {
						continue
					}
					if job.NextRunAt > 0 && now >= job.NextRunAt {
						jobID := job.ID
						go func() {
							_, _ = ExecuteCronJob(context.Background(), jobID)
						}()
					}
				}
			}
		}()
	})
}

func GetGroupScheduling(c *gin.Context) {
	var channels []*model.Channel
	if err := model.DB.Omit("key").Order("priority DESC, id ASC").Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	groupRatioMap := ratio_setting.GetGroupRatioCopy()
	usableGroups := setting.GetUserUsableGroupsCopy()
	schedulingModes := model.GetGroupSchedulingModes()
	syncCfg := model.GetModelPriceSyncConfig()
	costMap := model.GetAllChannelModelCosts()

	groupSet := make(map[string]struct{})
	for g := range groupRatioMap {
		if strings.TrimSpace(g) != "" {
			groupSet[strings.TrimSpace(g)] = struct{}{}
		}
	}
	for g := range usableGroups {
		if strings.TrimSpace(g) != "" {
			groupSet[strings.TrimSpace(g)] = struct{}{}
		}
	}
	groupSet["default"] = struct{}{}
	groupSet["vip"] = struct{}{}

	sourceChannelCounts := make(map[string]int)
	orphanSourceNames := make(map[string]string)
	groupChannels := make(map[string][]int)
	channelBriefs := make([]ChannelBrief, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		for g := range strings.SplitSeq(ch.Group, ",") {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			groupSet[g] = struct{}{}
			if !slices.Contains(groupChannels[g], ch.Id) {
				groupChannels[g] = append(groupChannels[g], ch.Id)
			}
		}

		upID, upName := model.ResolveChannelUpstreamSource(ch, nil, syncCfg)
		sourceChannelCounts[upID]++
		if upID != model.DirectUpstreamSourceID {
			orphanSourceNames[upID] = upName
		}

		upGroup := strings.TrimSpace(syncCfg.ChannelUpstreamGroups[ch.Id])
		if upGroup == "" {
			if meta, ok := syncCfg.AutoProvisionedChannels[ch.Id]; ok {
				upGroup = strings.TrimSpace(meta.UpstreamGroup)
			}
		}

		baseMult := 1.0
		curr := strings.ToUpper(strings.TrimSpace(syncCfg.ChannelCurrencies[ch.Id]))
		fromUpstream := false
		if chCosts, ok := costMap[ch.Id]; ok && len(chCosts) > 0 {
			for _, entry := range chCosts {
				if entry.FromUpstream || !fromUpstream {
					if entry.BaseMultiplier > 0 {
						baseMult = entry.BaseMultiplier
					}
					if curr == "" && strings.TrimSpace(entry.Currency) != "" {
						curr = strings.ToUpper(strings.TrimSpace(entry.Currency))
					}
					if entry.FromUpstream {
						fromUpstream = true
						break
					}
				}
			}
		}
		if curr == "" {
			curr = "USD"
		}

		channelBriefs = append(channelBriefs, ChannelBrief{
			ID:                 ch.Id,
			Name:               ch.Name,
			Type:               ch.Type,
			Status:             ch.Status,
			Group:              ch.Group,
			Models:             ch.Models,
			Priority:           ch.GetPriority(),
			Weight:             ch.GetWeight(),
			Tag:                ch.GetTag(),
			BaseURL:            ch.GetBaseURL(),
			Balance:            ch.Balance,
			BalanceUpdatedTime: ch.BalanceUpdatedTime,
			UpstreamSourceID:   upID,
			UpstreamSourceName: upName,
			UpstreamGroup:      upGroup,
			BaseMultiplier:     baseMult,
			Currency:           curr,
			FromUpstream:       fromUpstream,
		})
	}

	upstreamSources := make([]UpstreamSourceBrief, 0, len(syncCfg.UpstreamAccounts)+1)
	seenUpstreamIDs := make(map[string]struct{}, len(syncCfg.UpstreamAccounts)+1)
	for _, acc := range syncCfg.UpstreamAccounts {
		key := model.UpstreamAccountKey(acc)
		if key == "" {
			continue
		}
		if _, exists := seenUpstreamIDs[key]; exists {
			continue
		}
		seenUpstreamIDs[key] = struct{}{}
		curr := strings.ToUpper(strings.TrimSpace(acc.Currency))
		if curr == "" {
			curr = "CNY"
		}
		name := strings.TrimSpace(acc.Name)
		if name == "" {
			name = key
		}
		upstreamSources = append(upstreamSources, UpstreamSourceBrief{
			ID:           key,
			Name:         name,
			BaseURL:      strings.TrimSpace(acc.BaseURL),
			SyncMode:     strings.TrimSpace(acc.SyncMode),
			Enabled:      acc.Enabled,
			IsDirect:     false,
			Currency:     curr,
			ExchangeRate: model.ResolveEffectiveChannelExchangeRate(curr, acc.ExchangeRate),
			BalanceUSD:   acc.BalanceUSD,
			UsedQuotaUSD: acc.UsedQuotaUSD,
			ChannelCount: sourceChannelCounts[key],
		})
	}
	for orphanID, orphanName := range orphanSourceNames {
		if _, exists := seenUpstreamIDs[orphanID]; exists {
			continue
		}
		seenUpstreamIDs[orphanID] = struct{}{}
		upstreamSources = append(upstreamSources, UpstreamSourceBrief{
			ID:           orphanID,
			Name:         orphanName,
			Enabled:      true,
			IsDirect:     false,
			Currency:     "CNY",
			ExchangeRate: model.ResolveEffectiveChannelExchangeRate("CNY", 0),
			ChannelCount: sourceChannelCounts[orphanID],
		})
	}
	upstreamSources = append(upstreamSources, UpstreamSourceBrief{
		ID:           model.DirectUpstreamSourceID,
		Name:         "Direct",
		Enabled:      true,
		IsDirect:     true,
		Currency:     "USD",
		ExchangeRate: model.ResolveEffectiveChannelExchangeRate("USD", 0),
		ChannelCount: sourceChannelCounts[model.DirectUpstreamSourceID],
	})

	sortedGroups := make([]string, 0, len(groupSet))
	for g := range groupSet {
		sortedGroups = append(sortedGroups, g)
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		if sortedGroups[i] == "default" {
			return true
		}
		if sortedGroups[j] == "default" {
			return false
		}
		if sortedGroups[i] == "vip" {
			return true
		}
		if sortedGroups[j] == "vip" {
			return false
		}
		return sortedGroups[i] < sortedGroups[j]
	})

	summaries := make([]GroupSchedulingSummary, 0, len(sortedGroups))
	for _, g := range sortedGroups {
		ratio := groupRatioMap[g]
		if ratio <= 0 {
			ratio = 1.0
		}
		mode := schedulingModes[g]
		if mode == "" {
			mode = model.SchedulingModeStandard
		}
		normalizedMode := model.NormalizeSchedulingMode(mode)
		enabledIDs := groupChannels[g]
		if enabledIDs == nil {
			enabledIDs = []int{}
		}
		policy := model.GetGroupCostControlPolicy(g)
		profitSnap := model.GetGroupRealtimeProfitSnapshot(g, true)
		summaries = append(summaries, GroupSchedulingSummary{
			Group:             g,
			Description:       usableGroups[g],
			Ratio:             ratio,
			Mode:              normalizedMode,
			EnabledChannelIDs: enabledIDs,
			CostControl:       policy,
			RealtimeProfit:    profitSnap,
		})
	}

	common.ApiSuccess(c, gin.H{
		"groups":           summaries,
		"channels":         channelBriefs,
		"upstream_sources": upstreamSources,
	})
}

func UpdateGroupScheduling(c *gin.Context) {
	var req struct {
		Group             string                                  `json:"group"`
		Mode              string                                  `json:"mode"`
		EnabledChannelIDs *[]int                                  `json:"enabled_channel_ids"`
		Modes             map[string]string                       `json:"modes"`
		CostControl       *model.GroupCostControlPolicy           `json:"cost_control"`
		CostControls      map[string]model.GroupCostControlPolicy `json:"cost_controls"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	modesToSave := make(map[string]string)
	for g, m := range req.Modes {
		if strings.TrimSpace(g) != "" {
			modesToSave[strings.TrimSpace(g)] = model.NormalizeSchedulingMode(m)
		}
	}
	if strings.TrimSpace(req.Group) != "" && strings.TrimSpace(req.Mode) != "" {
		modesToSave[strings.TrimSpace(req.Group)] = model.NormalizeSchedulingMode(req.Mode)
	}
	if len(modesToSave) > 0 {
		if err := model.SaveGroupSchedulingModes(modesToSave); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	for g, policy := range req.CostControls {
		if strings.TrimSpace(g) == "" {
			continue
		}
		if err := model.SaveGroupCostControlPolicy(strings.TrimSpace(g), policy); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if strings.TrimSpace(req.Group) != "" && req.CostControl != nil {
		if err := model.SaveGroupCostControlPolicy(strings.TrimSpace(req.Group), *req.CostControl); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	if strings.TrimSpace(req.Group) != "" && req.EnabledChannelIDs != nil {
		if err := model.UpdateGroupChannels(strings.TrimSpace(req.Group), *req.EnabledChannelIDs); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	GetGroupScheduling(c)
}

type ChannelTelemetryItem struct {
	model.ChannelModelTelemetrySnapshot
	ChannelName string             `json:"channel_name,omitempty"`
	Scores      map[string]float64 `json:"scores,omitempty"`
}

func GetChannelTelemetry(c *gin.Context) {
	rawChannelID := strings.TrimSpace(c.Query("channel_id"))
	channelID := 0
	if rawChannelID != "" {
		parsedID, err := strconv.Atoi(rawChannelID)
		if err != nil || parsedID <= 0 {
			common.ApiErrorT(c, "Invalid parameters")
			return
		}
		channelID = parsedID
	}
	modelName := strings.TrimSpace(c.Query("model"))
	groupFilter := strings.TrimSpace(c.Query("group"))

	chByID := make(map[int]*model.Channel)
	if model.DB != nil {
		var channels []*model.Channel
		if err := model.DB.Omit("key").Order("id ASC").Find(&channels).Error; err == nil {
			for _, ch := range channels {
				if ch != nil {
					chByID[ch.Id] = ch
				}
			}
		}
	}

	snapshots := model.ListChannelModelTelemetry(channelID, modelName)
	if channelID > 0 && modelName != "" && len(snapshots) == 0 {
		snapshots = []model.ChannelModelTelemetrySnapshot{
			model.GetChannelModelTelemetry(channelID, modelName),
		}
	}

	items := make([]ChannelTelemetryItem, 0, len(snapshots))
	for _, snap := range snapshots {
		ch := chByID[snap.ChannelID]
		if groupFilter != "" && ch != nil {
			inGroup := false
			for g := range strings.SplitSeq(ch.Group, ",") {
				if strings.TrimSpace(g) == groupFilter {
					inGroup = true
					break
				}
			}
			if !inGroup {
				continue
			}
		} else if groupFilter != "" && ch == nil {
			continue
		}

		evalCh := ch
		if evalCh == nil {
			evalCh = &model.Channel{Id: snap.ChannelID}
		}
		item := ChannelTelemetryItem{
			ChannelModelTelemetrySnapshot: snap,
			Scores: map[string]float64{
				model.SchedulingModeSpeed:        model.ScoreChannelForSchedule(model.SchedulingModeSpeed, evalCh, snap.Model),
				model.SchedulingModeBBR:          model.ScoreChannelForSchedule(model.SchedulingModeBBR, evalCh, snap.Model),
				model.SchedulingModeAvailability: model.ScoreChannelForSchedule(model.SchedulingModeAvailability, evalCh, snap.Model),
				model.SchedulingModeLowerCost:    model.ScoreChannelForSchedule(model.SchedulingModeLowerCost, evalCh, snap.Model),
			},
		}
		if ch != nil {
			item.ChannelName = ch.Name
		}
		items = append(items, item)
	}

	resp := gin.H{
		"items": items,
	}
	if groupFilter != "" {
		resp["group"] = groupFilter
		resp["mode"] = model.GetGroupSchedulingMode(groupFilter)
	}
	common.ApiSuccess(c, resp)
}

func buildModelPriceTableRows(keyword string) ([]ModelPriceTableRow, model.ModelPriceSyncConfig, error) {
	var channels []*model.Channel
	if err := model.DB.Omit("key").Order("id ASC").Find(&channels).Error; err != nil {
		return nil, model.ModelPriceSyncConfig{}, err
	}

	costMap := model.GetAllChannelModelCosts()
	if len(costMap) == 0 {
		model.RecalculateAllChannelModelCosts(false)
		costMap = model.GetAllChannelModelCosts()
	}
	cfg := model.GetModelPriceSyncConfig()
	kw := strings.ToLower(strings.TrimSpace(keyword))

	rows := make([]ModelPriceTableRow, 0)
	// First pass: collect rows and determine highest 1x price channel per model
	highestUpstreamChByModel := make(map[string]int)
	highestUpstreamScoreByModel := make(map[string]float64)
	highestAnyChByModel := make(map[string]int)
	highestAnyScoreByModel := make(map[string]float64)
	recalculated := false

	for _, ch := range channels {
		if ch == nil || ch.Id <= 0 {
			continue
		}
		models := ch.GetModels()
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			if kw != "" && !strings.Contains(strings.ToLower(m), kw) && !strings.Contains(strings.ToLower(ch.Name), kw) {
				continue
			}
			costEntry, ok := costMap[ch.Id][m]
			if !ok {
				if !recalculated {
					model.RecalculateAllChannelModelCosts(false)
					recalculated = true
				}
				costEntry, _ = model.GetChannelModelCost(ch.Id, m)
			}
			telemetry := model.GetChannelModelTelemetry(ch.Id, m)

			_, _, rawInCost, rawOutCost, effCurr, effRate := model.ResolveEffectiveEntryCostsUSD(costEntry, cfg)
			unitMult := model.ResolveUpstreamToQuotaUnitMultiplier(effCurr, effRate)

			// Only synced upstream prices compete first for the selected price,
			// since local fallback entries mirror the site's current price.
			// Restore channel price to 1x first and then convert by the central currency rate.
			mult := costEntry.BaseMultiplier
			if mult <= 0 {
				mult = 1.0
			}
			baseRawIn := common.RoundNonNegativeFloat6(rawInCost / mult)
			baseRawOut := common.RoundNonNegativeFloat6(rawOutCost / mult)
			baseInUSD := common.RoundNonNegativeFloat6(baseRawIn * unitMult)
			baseOutUSD := common.RoundNonNegativeFloat6(baseRawOut * unitMult)
			basePriceScore := baseInUSD + baseOutUSD
			if costEntry.QuotaType == 1 && (costEntry.RawModelPrice > 0 || costEntry.ModelPrice > 0) {
				rawP := costEntry.RawModelPrice
				if rawP <= 0 {
					rawP = costEntry.ModelPrice
				}
				baseRawIn = common.RoundNonNegativeFloat6(rawP)
				baseRawOut = baseRawIn
				baseInUSD = common.RoundNonNegativeFloat6(rawP * unitMult)
				baseOutUSD = baseInUSD
				basePriceScore = baseInUSD
			}
			if prevBest, exists := highestAnyScoreByModel[m]; !exists || basePriceScore > prevBest {
				highestAnyScoreByModel[m] = basePriceScore
				highestAnyChByModel[m] = ch.Id
			}
			if costEntry.FromUpstream && !model.IsDefaultUnconfiguredModelRatio(m, costEntry.QuotaType, costEntry.ModelRatio, costEntry.CompletionRatio) {
				if prevBest, exists := highestUpstreamScoreByModel[m]; !exists || basePriceScore > prevBest {
					highestUpstreamScoreByModel[m] = basePriceScore
					highestUpstreamChByModel[m] = ch.Id
				}
			}
			rows = append(rows, ModelPriceTableRow{
				ChannelID:        ch.Id,
				ChannelName:      ch.Name,
				ChannelType:      ch.Type,
				ChannelStatus:    ch.Status,
				Model:            m,
				InputCost:        baseInUSD,
				OutputCost:       baseOutUSD,
				RawInputCost:     baseRawIn,
				RawOutputCost:    baseRawOut,
				ModelRatio:       costEntry.ModelRatio,
				CompletionRatio:  costEntry.CompletionRatio,
				CacheRatio:       costEntry.CacheRatio,
				CreateCacheRatio: costEntry.CreateCacheRatio,
				ModelPrice:       costEntry.ModelPrice,
				RawModelPrice:    costEntry.RawModelPrice,
				QuotaType:        costEntry.QuotaType,
				BaseMultiplier:   costEntry.BaseMultiplier,
				Currency:         effCurr,
				ExchangeRate:     effRate,
				FromUpstream:     costEntry.FromUpstream,
				Availability:     telemetry.Availability,
				TpsMin:           telemetry.TpsMin,
				TpsAvg:           telemetry.TpsAvg,
				TpsMax:           telemetry.TpsMax,
				AvgTtft:          telemetry.AvgTtftSec,
			})
		}
	}

	for i := range rows {
		m := rows[i].Model
		if prefChID := cfg.PreferredChannels[m]; prefChID > 0 {
			// A manual choice wins; choosing a channel without a synced price
			// keeps the site's current price for that model.
			rows[i].IsSelectedPrice = rows[i].ChannelID == prefChID
			continue
		}
		if upChID, hasUpstream := highestUpstreamChByModel[m]; hasUpstream {
			rows[i].IsSelectedPrice = rows[i].FromUpstream && rows[i].ChannelID == upChID
		} else {
			rows[i].IsSelectedPrice = rows[i].ChannelID == highestAnyChByModel[m]
		}
	}

	return rows, cfg, nil
}

func GetModelPriceTable(c *gin.Context) {
	keyword := c.Query("model")
	rows, cfg, err := buildModelPriceTableRows(keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"rows":   rows,
		"config": cfg.PublicView(),
	})
}

func applyAuthoritativeModelPricesToSystem(cfg model.ModelPriceSyncConfig) (int, error) {
	costMap := model.GetAllChannelModelCosts()
	if len(costMap) == 0 {
		model.RecalculateAllChannelModelCosts(false)
		costMap = model.GetAllChannelModelCosts()
	}

	// Group candidate 1x entries by model. Local fallback entries only mirror
	// the site's current price (or the 37.5 default ratio for an unpriced
	// model), so letting them compete would ratchet prices up and write the
	// default ratio as an explicit price for unpriced models.
	allSeenModels := make(map[string]bool)
	byModel := make(map[string][]model.ChannelModelCostEntry)
	manualLocal := make(map[string]bool)
	for chID, chMap := range costMap {
		for m, entry := range chMap {
			allSeenModels[m] = true
			if entry.FromUpstream {
				if model.IsDefaultUnconfiguredModelRatio(m, entry.QuotaType, entry.ModelRatio, entry.CompletionRatio) {
					continue
				}
				byModel[m] = append(byModel[m], entry)
			} else if cfg.PreferredChannels[m] == chID {
				manualLocal[m] = true
			}
		}
	}

	modelRatioCopy := ratio_setting.GetModelRatioCopy()
	compRatioCopy := ratio_setting.GetCompletionRatioCopy()
	modelPriceCopy := ratio_setting.GetModelPriceCopy()
	cacheRatioCopy := ratio_setting.GetCacheRatioCopy()
	createCacheRatioCopy := ratio_setting.GetCreateCacheRatioCopy()

	updatedCount := 0
	cacheUpdated := false

	// Replace any previously persisted 37.5 default fallback ratios on non-gpt-4.5 models
	// with sensible family fallbacks so GetModelRatio does not fall back to 37.5 ($75/1M).
	defaultRatios := ratio_setting.GetDefaultModelRatioMap()
	for m, r := range modelRatioCopy {
		if _, inDefault := defaultRatios[m]; !inDefault && model.IsDefaultUnconfiguredModelRatio(m, 0, r, compRatioCopy[m]) {
			mr, cr := model.InferSensibleFallbackModelPricing(m)
			modelRatioCopy[m] = common.RoundNonNegativeFloat6(mr)
			compRatioCopy[m] = common.RoundNonNegativeFloat6(cr)
			updatedCount++
		}
	}
	for m := range allSeenModels {
		if len(byModel[m]) > 0 || manualLocal[m] {
			continue
		}
		if modelPriceCopy[m] > 0 {
			continue
		}
		if r, hasR := modelRatioCopy[m]; !hasR || r <= 0 || model.IsDefaultUnconfiguredModelRatio(m, 0, r, compRatioCopy[m]) {
			mr, cr := model.InferSensibleFallbackModelPricing(m)
			modelRatioCopy[m] = common.RoundNonNegativeFloat6(mr)
			compRatioCopy[m] = common.RoundNonNegativeFloat6(cr)
			updatedCount++
		}
	}

	for m, entries := range byModel {
		if len(entries) == 0 || manualLocal[m] {
			continue
		}
		var chosen *model.ChannelModelCostEntry
		prefChID := cfg.PreferredChannels[m]
		if prefChID > 0 {
			for i := range entries {
				if entries[i].ChannelID == prefChID {
					chosen = &entries[i]
					break
				}
			}
		}
		if chosen == nil {
			// Highest price priority (or default fallback): restore channel price to 1x first,
			// then convert by the central currency rate.
			bestIdx := 0
			bestScore := -1.0
			for i := range entries {
				inUSD, outUSD, _, _, effCurr, effRate := model.ResolveEffectiveEntryCostsUSD(entries[i], cfg)
				unitMult := model.ResolveUpstreamToQuotaUnitMultiplier(effCurr, effRate)
				mult := entries[i].BaseMultiplier
				if mult <= 0 {
					mult = 1.0
				}
				score := (inUSD + outUSD) / mult
				if entries[i].QuotaType == 1 && (entries[i].RawModelPrice > 0 || entries[i].ModelPrice > 0) {
					rawP := entries[i].RawModelPrice
					if rawP <= 0 {
						rawP = entries[i].ModelPrice
					}
					score = rawP * unitMult
				}
				if score > bestScore {
					bestScore = score
					bestIdx = i
				}
			}
			chosen = &entries[bestIdx]
		}
		if chosen == nil {
			continue
		}
		// Restore the chosen channel's price to 1x first, then multiply by the central currency multiplier.
		_, _, rawInCost, rawOutCost, effCurr, effRate := model.ResolveEffectiveEntryCostsUSD(*chosen, cfg)
		unitMult := model.ResolveUpstreamToQuotaUnitMultiplier(effCurr, effRate)
		mult := chosen.BaseMultiplier
		if mult <= 0 {
			mult = 1.0
		}
		basePrice := chosen.RawModelPrice
		if basePrice <= 0 {
			basePrice = chosen.ModelPrice
		}
		if basePrice <= 0 && rawInCost > 0 {
			basePrice = rawInCost / mult
		}
		baseModelRatio := chosen.ModelRatio
		if rawInCost > 0 {
			baseModelRatio = rawInCost / (2.0 * mult)
		}
		usdBasePrice := common.RoundNonNegativeFloat6(basePrice * unitMult)
		usdBaseModelRatio := common.RoundNonNegativeFloat6(baseModelRatio * unitMult)
		compRatio := chosen.CompletionRatio
		if compRatio <= 0 && rawInCost > 0 && rawOutCost > 0 {
			compRatio = rawOutCost / rawInCost
		}
		if chosen.QuotaType == 1 && usdBasePrice > 0 {
			modelPriceCopy[m] = usdBasePrice
			if model.IsDefaultUnconfiguredModelRatio(m, 0, modelRatioCopy[m], compRatioCopy[m]) {
				delete(modelRatioCopy, m)
			}
			updatedCount++
		} else if chosen.QuotaType == 0 && usdBaseModelRatio > 0 {
			modelRatioCopy[m] = usdBaseModelRatio
			if compRatio > 0 {
				compRatioCopy[m] = common.RoundNonNegativeFloat6(compRatio)
			}
			delete(modelPriceCopy, m)
			updatedCount++
		}
		if chosen.CacheRatio > 0 {
			cacheRatioCopy[m] = common.RoundNonNegativeFloat6(chosen.CacheRatio)
			cacheUpdated = true
		}
		if chosen.CreateCacheRatio > 0 {
			createCacheRatioCopy[m] = common.RoundNonNegativeFloat6(chosen.CreateCacheRatio)
			cacheUpdated = true
		}
	}

	pricingUpdates := make(map[string]string)
	if cacheUpdated {
		if cacheBytes, err := common.Marshal(cacheRatioCopy); err == nil {
			pricingUpdates["CacheRatio"] = string(cacheBytes)
		}
		if createCacheBytes, err := common.Marshal(createCacheRatioCopy); err == nil {
			pricingUpdates["CreateCacheRatio"] = string(createCacheBytes)
		}
	}
	if updatedCount > 0 {
		if ratioBytes, err := common.Marshal(modelRatioCopy); err == nil {
			pricingUpdates["ModelRatio"] = string(ratioBytes)
		}
		if compBytes, err := common.Marshal(compRatioCopy); err == nil {
			pricingUpdates["CompletionRatio"] = string(compBytes)
		}
		if priceBytes, err := common.Marshal(modelPriceCopy); err == nil {
			pricingUpdates["ModelPrice"] = string(priceBytes)
		}
	}
	if len(pricingUpdates) > 0 {
		if model.DB == nil {
			if v, ok := pricingUpdates["CacheRatio"]; ok {
				_ = ratio_setting.UpdateCacheRatioByJSONString(v)
			}
			if v, ok := pricingUpdates["CreateCacheRatio"]; ok {
				_ = ratio_setting.UpdateCreateCacheRatioByJSONString(v)
			}
			if v, ok := pricingUpdates["ModelRatio"]; ok {
				_ = ratio_setting.UpdateModelRatioByJSONString(v)
			}
			if v, ok := pricingUpdates["CompletionRatio"]; ok {
				_ = ratio_setting.UpdateCompletionRatioByJSONString(v)
			}
			if v, ok := pricingUpdates["ModelPrice"]; ok {
				_ = ratio_setting.UpdateModelPriceByJSONString(v)
			}
		} else {
			if err := model.UpdateModelPricingOptions(pricingUpdates); err != nil {
				return 0, err
			}
		}
		model.InvalidatePricingCache()
	}
	return updatedCount, nil
}

func ApplyModelPriceTable(c *gin.Context) {
	var req struct {
		HighestPricePriority *bool          `json:"highest_price_priority"`
		PreferredChannels    map[string]int `json:"preferred_channels"`
		Model                string         `json:"model"`
		ChannelID            *int           `json:"channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	cfg, err := model.UpdateModelPriceSyncConfig(func(cfg *model.ModelPriceSyncConfig) error {
		if req.HighestPricePriority != nil {
			cfg.HighestPricePriority = *req.HighestPricePriority
			if cfg.HighestPricePriority && len(req.PreferredChannels) == 0 && req.ChannelID == nil {
				// Clear manual overrides when switching back to global highest-price priority
				cfg.PreferredChannels = make(map[string]int)
			}
		}
		if cfg.PreferredChannels == nil {
			cfg.PreferredChannels = make(map[string]int)
		}
		if req.PreferredChannels != nil {
			for m, chID := range req.PreferredChannels {
				m = strings.TrimSpace(m)
				if m == "" {
					continue
				}
				if chID <= 0 {
					delete(cfg.PreferredChannels, m)
				} else {
					cfg.PreferredChannels[m] = chID
				}
			}
		}
		if strings.TrimSpace(req.Model) != "" && req.ChannelID != nil {
			m := strings.TrimSpace(req.Model)
			if *req.ChannelID <= 0 {
				delete(cfg.PreferredChannels, m)
			} else {
				cfg.PreferredChannels[m] = *req.ChannelID
			}
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	model.RecalculateAllChannelModelCosts(true)
	updatedCount, err := applyAuthoritativeModelPricesToSystem(cfg)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	rows, updatedCfg, err := buildModelPriceTableRows("")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"updated_models": updatedCount,
		"config":         updatedCfg.PublicView(),
		"rows":           rows,
	})
}

func GetCronSyncConfig(c *gin.Context) {
	var channels []*model.Channel
	if err := model.DB.Omit("key").Order("id ASC").Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	cfg := model.GetModelPriceSyncConfig()
	if cfg.ChannelCurrencies == nil {
		cfg.ChannelCurrencies = make(map[int]string)
	}
	if cfg.ChannelTopupMultipliers == nil {
		cfg.ChannelTopupMultipliers = make(map[int]float64)
	}
	briefs := make([]ChannelBrief, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		if cfg.ChannelCurrencies[ch.Id] == "" && ch.CurrencyCode != nil {
			if c := strings.ToUpper(strings.TrimSpace(*ch.CurrencyCode)); c != "" && c != "AUTO" {
				cfg.ChannelCurrencies[ch.Id] = c
			}
		} else if c := strings.ToUpper(strings.TrimSpace(cfg.ChannelCurrencies[ch.Id])); c != "" && c != "AUTO" {
			dbCurr := ""
			if ch.CurrencyCode != nil {
				dbCurr = strings.ToUpper(strings.TrimSpace(*ch.CurrencyCode))
			}
			if dbCurr == "" && model.DB != nil {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("currency_code", c).Error
			}
		}
		if cfg.ChannelTopupMultipliers[ch.Id] <= 0 {
			cfg.ChannelTopupMultipliers[ch.Id] = model.ResolveChannelTopupMultiplier(ch.Id, ch, cfg)
		}
		briefs = append(briefs, ChannelBrief{
			ID:                 ch.Id,
			Name:               ch.Name,
			Type:               ch.Type,
			Status:             ch.Status,
			Group:              ch.Group,
			Models:             ch.Models,
			Priority:           ch.GetPriority(),
			Weight:             ch.GetWeight(),
			Tag:                ch.GetTag(),
			BaseURL:            ch.GetBaseURL(),
			Balance:            ch.Balance,
			BalanceUpdatedTime: ch.BalanceUpdatedTime,
		})
	}
	common.ApiSuccess(c, gin.H{
		"config":   cfg.PublicView(),
		"channels": briefs,
	})
}

type CronSyncConfigUpdateInput struct {
	HighestPricePriority    *bool                         `json:"highest_price_priority"`
	PreferredChannels       map[string]int                `json:"preferred_channels"`
	DefaultCurrency         *string                       `json:"default_currency"`
	CustomExchangeRate      *float64                      `json:"custom_exchange_rate"`
	USDExchangeRate         *float64                      `json:"usd_exchange_rate"`
	CNYExchangeRate         *float64                      `json:"cny_exchange_rate"`
	TWDExchangeRate         *float64                      `json:"twd_exchange_rate"`
	ChannelCurrencies       map[int]string                `json:"channel_currencies"`
	ChannelExchangeRates    map[int]float64               `json:"channel_exchange_rates"`
	ChannelTopupMultipliers map[int]float64               `json:"channel_topup_multipliers"`
	ChannelUpstreamGroups   map[int]string                `json:"channel_upstream_groups"`
	ClassifierEnabled       *bool                         `json:"classifier_enabled"`
	ClassifierRules         []model.ChannelClassifierRule `json:"classifier_rules"`
	CronEnabled             *bool                         `json:"cron_enabled"`
	CronIntervalMinutes     *int                          `json:"cron_interval_minutes"`
	AutoSyncModels          *bool                         `json:"auto_sync_models"`
	AutoSyncPrices          *bool                         `json:"auto_sync_prices"`
	ReplaceModels           *bool                         `json:"replace_models"`
}

func UpdateCronSyncConfig(c *gin.Context) {
	var req CronSyncConfigUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	updated, err := model.UpdateModelPriceSyncConfig(func(cfg *model.ModelPriceSyncConfig) error {
		if req.HighestPricePriority != nil {
			cfg.HighestPricePriority = *req.HighestPricePriority
		}
		if req.PreferredChannels != nil {
			cfg.PreferredChannels = req.PreferredChannels
		}
		if req.DefaultCurrency != nil {
			cfg.DefaultCurrency = *req.DefaultCurrency
		}
		if req.CustomExchangeRate != nil {
			cfg.CustomExchangeRate = *req.CustomExchangeRate
		}
		if req.USDExchangeRate != nil && *req.USDExchangeRate > 0 {
			cfg.USDExchangeRate = *req.USDExchangeRate
		}
		if req.CNYExchangeRate != nil && *req.CNYExchangeRate > 0 {
			cfg.CNYExchangeRate = *req.CNYExchangeRate
		}
		if req.TWDExchangeRate != nil && *req.TWDExchangeRate > 0 {
			cfg.TWDExchangeRate = *req.TWDExchangeRate
		}
		if req.ChannelCurrencies != nil {
			cfg.ChannelCurrencies = req.ChannelCurrencies
		}
		if req.ChannelExchangeRates != nil {
			cfg.ChannelExchangeRates = req.ChannelExchangeRates
		}
		if req.ChannelTopupMultipliers != nil {
			cfg.ChannelTopupMultipliers = req.ChannelTopupMultipliers
		}
		if req.ChannelUpstreamGroups != nil {
			cfg.ChannelUpstreamGroups = req.ChannelUpstreamGroups
		}
		if req.ClassifierEnabled != nil {
			v := *req.ClassifierEnabled
			cfg.ClassifierEnabled = &v
		}
		if req.ClassifierRules != nil {
			cfg.ClassifierRules = req.ClassifierRules
		}
		if req.CronEnabled != nil {
			cfg.CronEnabled = *req.CronEnabled
		}
		if req.CronIntervalMinutes != nil && *req.CronIntervalMinutes > 0 {
			cfg.CronIntervalMinutes = *req.CronIntervalMinutes
		}
		if req.AutoSyncModels != nil {
			cfg.AutoSyncModels = *req.AutoSyncModels
		}
		if req.AutoSyncPrices != nil {
			cfg.AutoSyncPrices = *req.AutoSyncPrices
		}
		if req.ReplaceModels != nil {
			v := *req.ReplaceModels
			cfg.ReplaceModels = &v
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.ResetRealtimeProfitTracker()
	if model.DB != nil && len(req.ChannelCurrencies) > 0 {
		for chID, rawCurr := range req.ChannelCurrencies {
			if chID <= 0 {
				continue
			}
			curr := strings.ToUpper(strings.TrimSpace(rawCurr))
			if curr == "" || curr == "AUTO" {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", chID).Update("currency_code", nil).Error
			} else {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", chID).Update("currency_code", curr).Error
			}
		}
		model.RecalculateAllChannelModelCosts(true)
	}
	common.ApiSuccess(c, gin.H{
		"config": updated.PublicView(),
	})
}

func RunCronSyncNow(c *gin.Context) {
	var req struct {
		ChannelIDs           []int   `json:"channel_ids"`
		SyncModels           *bool   `json:"sync_models"`
		SyncPrices           *bool   `json:"sync_prices"`
		ReplaceModels        *bool   `json:"replace_models"`
		Currency             string  `json:"currency"`
		ExchangeRate         float64 `json:"exchange_rate"`
		HighestPricePriority *bool   `json:"highest_price_priority"`
	}
	_ = c.ShouldBindJSON(&req)

	cfg := model.GetModelPriceSyncConfig()
	if req.ReplaceModels != nil {
		v := *req.ReplaceModels
		if updated, err := model.UpdateModelPriceSyncConfig(func(cur *model.ModelPriceSyncConfig) error {
			cp := v
			cur.ReplaceModels = &cp
			return nil
		}); err == nil {
			cfg = updated
		}
	}
	syncModels := cfg.AutoSyncModels
	if req.SyncModels != nil {
		syncModels = *req.SyncModels
	}
	syncPrices := cfg.AutoSyncPrices
	if req.SyncPrices != nil {
		syncPrices = *req.SyncPrices
	}
	currency := cfg.DefaultCurrency
	if strings.TrimSpace(req.Currency) != "" {
		currency = strings.TrimSpace(req.Currency)
	}
	exchangeRate := cfg.CustomExchangeRate
	if req.ExchangeRate > 0 {
		exchangeRate = req.ExchangeRate
	}

	results, err := ExecuteChannelUpstreamSync(
		c.Request.Context(),
		req.ChannelIDs,
		syncModels,
		syncPrices,
		currency,
		exchangeRate,
		req.HighestPricePriority,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"results": results,
		"config":  model.GetModelPriceSyncConfig().PublicView(),
	})
}

type newAPIUpstreamPricingItem struct {
	ModelName        string   `json:"model_name"`
	QuotaType        int      `json:"quota_type"`
	ModelRatio       *float64 `json:"model_ratio"`
	ModelPrice       *float64 `json:"model_price"`
	CompletionRatio  *float64 `json:"completion_ratio"`
	CacheRatio       *float64 `json:"cache_ratio"`
	CreateCacheRatio *float64 `json:"create_cache_ratio"`
	BillingMode      string   `json:"billing_mode"`
	BillingExpr      string   `json:"billing_expr"`
	EnableGroups     []string `json:"enable_groups"`
}

type newAPIUpstreamPricingResponse struct {
	Success     bool                        `json:"success"`
	Message     string                      `json:"message"`
	Data        []newAPIUpstreamPricingItem `json:"data"`
	GroupRatio  map[string]float64          `json:"group_ratio"`
	UsableGroup map[string]string           `json:"usable_group"`
}

func fetchNewAPIUpstreamPricing(ctx context.Context, channel *model.Channel) (*newAPIUpstreamPricingResponse, error) {
	baseURL := model.CleanUpstreamAPIBaseURL(channel.GetBaseURL(), "")
	if baseURL == "" {
		baseURL = model.CleanUpstreamAPIBaseURL(constant.GetChannelBaseURL(channel.Type), "")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("channel #%d base URL is empty", channel.Id)
	}

	httpClient, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}

	key, _, apiErr := channel.GetNextEnabledKey()
	authKey := ""
	if apiErr == nil {
		authKey = strings.TrimSpace(key)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	pricingURL := baseURL + "/api/pricing"
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, pricingURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "New-API-UpstreamSync/1.0")
	if authKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authKey)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream /api/pricing returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxRatioConfigBytes))
	if err != nil {
		return nil, err
	}
	if err := checkNotHTMLBody(bodyBytes, pricingURL); err != nil {
		return nil, err
	}

	var pricingResp newAPIUpstreamPricingResponse
	if err := common.DecodeJson(bytes.NewReader(bodyBytes), &pricingResp); err != nil {
		return nil, fmt.Errorf("failed to parse /api/pricing response: %w", err)
	}
	if !pricingResp.Success && len(pricingResp.Data) == 0 {
		return nil, fmt.Errorf("upstream /api/pricing unsuccessful: %s", pricingResp.Message)
	}

	return &pricingResp, nil
}

type upstreamBasePricing struct {
	QuotaType        int
	ModelPrice       float64
	ModelRatio       float64
	CompletionRatio  float64
	CacheRatio       float64
	CreateCacheRatio float64
	Priced           bool
}

func deriveUpstreamBasePricing(item newAPIUpstreamPricingItem) upstreamBasePricing {
	if strings.EqualFold(strings.TrimSpace(item.BillingMode), billing_setting.BillingModeTieredExpr) {
		if mr, cr, cacheR, createCacheR, hasCache, hasCreateCache, ok := model.ExtractExprBaseTokenPricing(item.BillingExpr); ok {
			if !hasCache {
				cacheR = 1.0
			}
			if !hasCreateCache {
				createCacheR = 1.0
			}
			return upstreamBasePricing{
				QuotaType:        0,
				ModelRatio:       common.RoundNonNegativeFloat6(mr),
				CompletionRatio:  common.RoundNonNegativeFloat6(cr),
				CacheRatio:       common.RoundNonNegativeFloat6(cacheR),
				CreateCacheRatio: common.RoundNonNegativeFloat6(createCacheR),
				Priced:           true,
			}
		}
		if unitPrice, ok := model.ExtractExprUnitPrice(item.BillingExpr); ok {
			return upstreamBasePricing{
				QuotaType:       1,
				ModelPrice:      common.RoundNonNegativeFloat6(unitPrice),
				CompletionRatio: 1.0,
				Priced:          true,
			}
		}
	}

	if item.QuotaType == 1 && item.ModelPrice != nil && *item.ModelPrice >= 0 {
		return upstreamBasePricing{
			QuotaType:       1,
			ModelPrice:      common.RoundNonNegativeFloat6(*item.ModelPrice),
			CompletionRatio: 1.0,
			Priced:          true,
		}
	}

	base := upstreamBasePricing{
		QuotaType:       0,
		ModelRatio:      derefNonNegative(item.ModelRatio),
		CompletionRatio: 1.0,
	}
	if item.CompletionRatio != nil && *item.CompletionRatio > 0 {
		base.CompletionRatio = *item.CompletionRatio
	}
	if item.CacheRatio != nil && *item.CacheRatio > 0 {
		base.CacheRatio = *item.CacheRatio
	}
	if item.CreateCacheRatio != nil && *item.CreateCacheRatio > 0 {
		base.CreateCacheRatio = *item.CreateCacheRatio
	}
	base.Priced = item.ModelRatio != nil
	return base
}

func buildUpstreamChannelModelCostEntry(channelID int, channelName string, channelType int, modelName string, base upstreamBasePricing, groupRatio float64, currency string, effRate float64, unitMult float64, updatedAt int64) (model.ChannelModelCostEntry, bool) {
	entry := model.ChannelModelCostEntry{
		ChannelID:        channelID,
		ChannelName:      channelName,
		ChannelType:      channelType,
		Model:            modelName,
		ModelRatio:       common.RoundNonNegativeFloat6(base.ModelRatio),
		CompletionRatio:  base.CompletionRatio,
		CacheRatio:       base.CacheRatio,
		CreateCacheRatio: base.CreateCacheRatio,
		BaseMultiplier:   groupRatio,
		Currency:         currency,
		ExchangeRate:     effRate,
		FromUpstream:     true,
		UpdatedAt:        updatedAt,
	}
	if base.QuotaType == 1 && base.ModelPrice >= 0 {
		entry.QuotaType = 1
		entry.RawModelPrice = common.RoundNonNegativeFloat6(base.ModelPrice)
		entry.ModelPrice = common.RoundNonNegativeFloat6(base.ModelPrice)
		entry.RawInputCost = common.RoundNonNegativeFloat6(entry.RawModelPrice * groupRatio)
		entry.RawOutputCost = entry.RawInputCost
		entry.InputCost = common.RoundNonNegativeFloat6(entry.RawInputCost * unitMult)
		entry.OutputCost = entry.InputCost
		return entry, true
	}
	if base.Priced && !model.IsDefaultUnconfiguredModelRatio(modelName, 0, base.ModelRatio, base.CompletionRatio) {
		entry.QuotaType = 0
		entry.RawInputCost = common.RoundNonNegativeFloat6(base.ModelRatio * 2.0 * groupRatio)
		entry.RawOutputCost = common.RoundNonNegativeFloat6(entry.RawInputCost * base.CompletionRatio)
		entry.InputCost = common.RoundNonNegativeFloat6(entry.RawInputCost * unitMult)
		entry.OutputCost = common.RoundNonNegativeFloat6(entry.InputCost * base.CompletionRatio)
		return entry, true
	}
	return model.ChannelModelCostEntry{}, false
}

func derefNonNegative(value *float64) float64 {
	if value == nil || *value < 0 {
		return 0
	}
	return *value
}

func upstreamItemInGroup(item newAPIUpstreamPricingItem, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" || len(item.EnableGroups) == 0 {
		return true
	}
	for _, allowed := range item.EnableGroups {
		if strings.TrimSpace(allowed) == group {
			return true
		}
	}
	return false
}

func resolveUpstreamGroupRatio(groupRatio map[string]float64, items []newAPIUpstreamPricingItem, configuredGroup string) (string, float64) {
	configuredGroup = strings.TrimSpace(configuredGroup)
	if configuredGroup != "" {
		ratio := groupRatio[configuredGroup]
		if ratio <= 0 {
			ratio = 1.0
		}
		return configuredGroup, ratio
	}
	if def, ok := groupRatio["default"]; ok && def > 0 {
		return "default", def
	}

	names := make([]string, 0, len(groupRatio))
	for name, value := range groupRatio {
		if value > 0 && name != "default" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", 1.0
	}

	restricted := false
	supported := make(map[string]struct{})
	for _, item := range items {
		if len(item.EnableGroups) > 0 {
			restricted = true
			for _, group := range item.EnableGroups {
				supported[strings.TrimSpace(group)] = struct{}{}
			}
		}
	}
	if restricted && len(supported) > 0 {
		kept := make([]string, 0, len(names))
		for _, name := range names {
			if _, ok := supported[name]; ok {
				kept = append(kept, name)
			}
		}
		if len(kept) > 0 {
			names = kept
		}
	}

	sort.Strings(names)
	best := names[0]
	for _, name := range names {
		if groupRatio[name] < groupRatio[best] {
			best = name
		}
	}
	return best, groupRatio[best]
}

var trailingMultiplierRe = regexp.MustCompile(`-(\d+(?:\.\d+)?)$`)

func collectAvailableUpstreamGroups(groupRatio map[string]float64, items []newAPIUpstreamPricingItem) map[string]float64 {
	valid := make(map[string]float64, len(groupRatio))
	for name, ratio := range groupRatio {
		name = strings.TrimSpace(name)
		if name != "" && ratio > 0 {
			valid[name] = ratio
		}
	}
	for _, item := range items {
		for _, g := range item.EnableGroups {
			g = strings.TrimSpace(g)
			if g != "" {
				if _, ok := valid[g]; !ok {
					valid[g] = 1.0
				}
			}
		}
	}
	return valid
}

func runeOverlapSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1.0
	}
	setA := make(map[rune]int)
	lenA := 0
	for _, r := range a {
		if r == ' ' || r == '\t' {
			continue
		}
		setA[r]++
		lenA++
	}
	setB := make(map[rune]int)
	lenB := 0
	for _, r := range b {
		if r == ' ' || r == '\t' {
			continue
		}
		setB[r]++
		lenB++
	}
	if lenA == 0 || lenB == 0 {
		return 0
	}
	commonRunes := 0
	for r, cA := range setA {
		if cB, ok := setB[r]; ok {
			commonRunes += min(cA, cB)
		}
	}
	return float64(2*commonRunes) / float64(lenA+lenB)
}

func resolveChannelUpstreamGroup(
	channelName string,
	pricingResp *newAPIUpstreamPricingResponse,
	v1Models []string,
	configuredGroup string,
) (string, float64, bool) {
	if pricingResp == nil {
		return strings.TrimSpace(configuredGroup), 1.0, false
	}
	validGroups := collectAvailableUpstreamGroups(pricingResp.GroupRatio, pricingResp.Data)
	configuredGroup = strings.TrimSpace(configuredGroup)
	if len(validGroups) == 0 {
		g, r := resolveUpstreamGroupRatio(pricingResp.GroupRatio, pricingResp.Data, configuredGroup)
		return g, r, false
	}

	if configuredGroup != "" {
		if ratio, ok := validGroups[configuredGroup]; ok {
			return configuredGroup, ratio, false
		}
		for g, ratio := range validGroups {
			if strings.EqualFold(g, configuredGroup) {
				return g, ratio, g != configuredGroup
			}
		}
	}

	hasTrailingRatio := false
	trailingRatio := 0.0
	if m := trailingMultiplierRe.FindStringSubmatch(strings.TrimSpace(channelName)); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
			hasTrailingRatio = true
			trailingRatio = v
		}
	}

	// Pass 1: Exact substring match of group name inside channelName (longest match wins).
	var exactNameMatches []string
	for g := range validGroups {
		if g == "default" && !strings.Contains(strings.ToLower(channelName), "default") {
			continue
		}
		if strings.Contains(channelName, g) {
			exactNameMatches = append(exactNameMatches, g)
		}
	}
	if len(exactNameMatches) > 0 {
		sort.Slice(exactNameMatches, func(i, j int) bool {
			li := len([]rune(exactNameMatches[i]))
			lj := len([]rune(exactNameMatches[j]))
			if li != lj {
				return li > lj
			}
			if hasTrailingRatio {
				mi := math.Abs(validGroups[exactNameMatches[i]]-trailingRatio) < 1e-6
				mj := math.Abs(validGroups[exactNameMatches[j]]-trailingRatio) < 1e-6
				if mi != mj {
					return mi
				}
			}
			return exactNameMatches[i] < exactNameMatches[j]
		})
		best := exactNameMatches[0]
		return best, validGroups[best], true
	}

	// Pass 2: Score candidates by model overlap with v1Models, trailing ratio match, and name/configuredGroup similarity.
	v1Set := make(map[string]struct{}, len(v1Models))
	for _, m := range v1Models {
		m = strings.TrimSpace(m)
		if m != "" {
			v1Set[m] = struct{}{}
		}
	}
	groupModelSets := make(map[string]map[string]struct{}, len(validGroups))
	for g := range validGroups {
		groupModelSets[g] = make(map[string]struct{})
	}
	for _, item := range pricingResp.Data {
		mName := strings.TrimSpace(item.ModelName)
		if mName == "" {
			continue
		}
		for g := range validGroups {
			if upstreamItemInGroup(item, g) {
				groupModelSets[g][mName] = struct{}{}
			}
		}
	}

	strippedChName := strings.TrimSpace(channelName)
	if hasTrailingRatio {
		strippedChName = trailingMultiplierRe.ReplaceAllString(strippedChName, "")
	}
	if _, afterHyphen, ok := strings.Cut(strippedChName, "-"); ok && strings.TrimSpace(afterHyphen) != "" {
		strippedChName = strings.TrimSpace(afterHyphen)
	}

	bestCandidate := ""
	bestScore := 0.0
	candidateNames := make([]string, 0, len(validGroups))
	for g := range validGroups {
		candidateNames = append(candidateNames, g)
	}
	sort.Strings(candidateNames)

	for _, g := range candidateNames {
		score := 0.0
		gModels := groupModelSets[g]
		if len(v1Set) > 0 && len(gModels) > 0 {
			overlap := 0
			for m := range v1Set {
				if _, ok := gModels[m]; ok {
					overlap++
				}
			}
			if overlap > 0 {
				precision := float64(overlap) / float64(len(v1Set))
				recall := float64(overlap) / float64(len(gModels))
				score += precision*45.0 + recall*45.0
			}
		}
		if hasTrailingRatio && math.Abs(validGroups[g]-trailingRatio) < 1e-6 {
			score += 25.0
		}
		sim := runeOverlapSimilarity(g, strippedChName)
		if configuredGroup != "" {
			sim = max(sim, runeOverlapSimilarity(g, configuredGroup))
		}
		if sim >= 0.45 {
			score += sim * 40.0
		}
		if score > bestScore {
			bestScore = score
			bestCandidate = g
		}
	}

	if bestCandidate != "" && bestScore >= 35.0 {
		return bestCandidate, validGroups[bestCandidate], true
	}

	g, r := resolveUpstreamGroupRatio(pricingResp.GroupRatio, pricingResp.Data, configuredGroup)
	return g, r, false
}

func ExecuteChannelUpstreamSync(
	ctx context.Context,
	channelIDs []int,
	syncModels bool,
	syncPrices bool,
	currencyOverride string,
	exchangeRateOverride float64,
	highestPricePriorityOverride *bool,
) ([]ChannelUpstreamSyncItemResult, error) {
	cronSyncRunningMu.Lock()
	defer cronSyncRunningMu.Unlock()

	var channels []*model.Channel
	query := model.DB.Select(channelUpstreamModelUpdateSelectFields)
	if len(channelIDs) > 0 {
		query = query.Where("id IN ?", channelIDs)
	} else {
		query = query.Where("status = ?", common.ChannelStatusEnabled)
	}
	if err := query.Order("id ASC").Find(&channels).Error; err != nil {
		return nil, err
	}

	cfg := model.GetModelPriceSyncConfig()
	if highestPricePriorityOverride != nil {
		cfg.HighestPricePriority = *highestPricePriorityOverride
	}
	if strings.TrimSpace(currencyOverride) != "" {
		cfg.DefaultCurrency = strings.ToUpper(strings.TrimSpace(currencyOverride))
	}
	if exchangeRateOverride > 0 {
		cfg.CustomExchangeRate = exchangeRateOverride
	}
	replaceModels := cfg.IsReplaceModelsEnabled()

	now := time.Now().Unix()
	results := make([]ChannelUpstreamSyncItemResult, 0, len(channels))
	anyChannelUpdated := false
	pricingCache := make(map[string]*newAPIUpstreamPricingResponse)

	// Refresh configured upstream account balances and token balances once per upstream source
	upstreamSourceIdxByHost := make(map[string]int, len(cfg.UpstreamAccounts))
	upstreamSelfBalByHost := make(map[string]upstreamSelfQuotaInfo, len(cfg.UpstreamAccounts))
	upstreamTokensByHost := make(map[string]map[string]upstreamTokenRecord, len(cfg.UpstreamAccounts))
	for i := range cfg.UpstreamAccounts {
		src := &cfg.UpstreamAccounts[i]
		if !src.Enabled || strings.TrimSpace(src.BaseURL) == "" || strings.TrimSpace(src.AccessToken) == "" {
			continue
		}
		normHost := strings.ToLower(model.CleanUpstreamAPIBaseURL(src.BaseURL, ""))
		if normHost == "" {
			continue
		}
		upstreamSourceIdxByHost[normHost] = i
		selfBal := fetchUpstreamSelfBalance(ctx, src.BaseURL, src.AccessToken, src.UserID, src.SyncMode, src.Currency, src.ExchangeRate)
		if selfBal.HasValidQuota {
			src.BalanceUSD = selfBal.BalanceUSD
			src.UsedQuotaUSD = selfBal.UsedQuotaUSD
			upstreamSelfBalByHost[normHost] = selfBal
		}
		if toks, _, err := fetchUpstreamTokensList(ctx, src.BaseURL, src.AccessToken, src.UserID, src.SyncMode); err == nil {
			byGroup := make(map[string]upstreamTokenRecord, len(toks))
			for _, t := range toks {
				g := strings.TrimSpace(t.Group)
				if g != "" && t.Status == 1 {
					if _, exists := byGroup[g]; !exists {
						byGroup[g] = t
					}
				}
			}
			upstreamTokensByHost[normHost] = byGroup
		}
	}

	for _, ch := range channels {
		if ch == nil || ch.Id <= 0 {
			continue
		}
		itemRes := ChannelUpstreamSyncItemResult{
			ChannelID:          ch.Id,
			ChannelName:        ch.Name,
			ChannelType:        ch.Type,
			DetectedCurrency:   "USD",
			ExchangeRate:       1.0,
			BaseMultiplier:     1.0,
			Balance:            ch.Balance,
			BalanceUpdatedTime: ch.BalanceUpdatedTime,
		}

		chCurr := cfg.DefaultCurrency
		if v, ok := cfg.ChannelCurrencies[ch.Id]; ok && strings.TrimSpace(v) != "" {
			chCurr = v
		}
		chCurr = strings.ToUpper(strings.TrimSpace(chCurr))
		chRate := cfg.CustomExchangeRate
		if v, ok := cfg.ChannelExchangeRates[ch.Id]; ok && v > 0 {
			chRate = v
		}

		var upstreamModels []string
		var upstreamErr error
		confirmedEmptyNewAPIGroup := false

		if ch.Type == constant.ChannelTypeNewAPI {
			cleanHost := model.CleanUpstreamAPIBaseURL(ch.GetBaseURL(), "")
			var pricingResp *newAPIUpstreamPricingResponse
			var pErr error
			if cleanHost != "" {
				if cached, ok := pricingCache[cleanHost]; ok {
					pricingResp = cached
				} else {
					pricingResp, pErr = fetchNewAPIUpstreamPricing(ctx, ch)
					if pErr == nil && pricingResp != nil {
						pricingCache[cleanHost] = pricingResp
					}
				}
			} else {
				pricingResp, pErr = fetchNewAPIUpstreamPricing(ctx, ch)
			}

			configuredGroup := strings.TrimSpace(cfg.ChannelUpstreamGroups[ch.Id])
			var v1Models []string
			var v1Err error
			if pricingResp == nil || configuredGroup == "" {
				v1Models, v1Err = fetchChannelUpstreamModelIDs(ch)
			}

			if pErr == nil && pricingResp != nil {
				resolvedCurrency := "CNY"
				resolvedRate := 1.0
				if chCurr != "" && chCurr != "AUTO" {
					resolvedCurrency = chCurr
					resolvedRate = model.ResolveEffectiveChannelExchangeRate(chCurr, chRate)
				} else {
					upID, _ := model.ResolveChannelUpstreamSource(ch, nil, cfg)
					if upID != "" && upID != model.DirectUpstreamSourceID {
						if acc, ok := model.FindUpstreamAccountByKey(cfg, upID); ok {
							accCurr := strings.ToUpper(strings.TrimSpace(acc.Currency))
							if accCurr != "" && accCurr != "AUTO" {
								resolvedCurrency = accCurr
							}
							if chRate <= 0 && acc.ExchangeRate > 0 {
								chRate = acc.ExchangeRate
							}
						}
					}
					resolvedRate = model.ResolveEffectiveChannelExchangeRate(resolvedCurrency, chRate)
				}
				if resolvedRate <= 0 {
					resolvedRate = 1.0
				}
				itemRes.DetectedCurrency = resolvedCurrency
				itemRes.ExchangeRate = resolvedRate
				itemRes.AvailableGroups = collectAvailableUpstreamGroups(pricingResp.GroupRatio, pricingResp.Data)
				if resolvedCurrency != "" && resolvedCurrency != "AUTO" {
					cfg.ChannelCurrencies[ch.Id] = resolvedCurrency
					dbCurr := ""
					if ch.CurrencyCode != nil {
						dbCurr = strings.ToUpper(strings.TrimSpace(*ch.CurrencyCode))
					}
					if dbCurr != resolvedCurrency && model.DB != nil {
						currCopy := resolvedCurrency
						ch.CurrencyCode = &currCopy
						_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("currency_code", resolvedCurrency).Error
					}
				}

				upstreamGroup, upstreamGroupRatio, autoUpdated := resolveChannelUpstreamGroup(
					ch.Name,
					pricingResp,
					v1Models,
					configuredGroup,
				)
				if autoUpdated && upstreamGroup != "" {
					cfg.ChannelUpstreamGroups[ch.Id] = upstreamGroup
				}
				itemRes.UpstreamGroup = upstreamGroup
				itemRes.BaseMultiplier = upstreamGroupRatio

				knownPricingModels := make(map[string]bool, len(pricingResp.Data))
				costEntries := make(map[string]model.ChannelModelCostEntry, len(pricingResp.Data))
				for _, pItem := range pricingResp.Data {
					mName := strings.TrimSpace(pItem.ModelName)
					if mName == "" {
						continue
					}
					knownPricingModels[mName] = true
					if !upstreamItemInGroup(pItem, upstreamGroup) {
						continue
					}
					upstreamModels = append(upstreamModels, mName)

					// Preserve the unscaled 1x base ModelRatio / ModelPrice for system pricing,
					// and convert only the channel's actual cost (InputCost / OutputCost) into local quota units via unitMult.
					base := deriveUpstreamBasePricing(pItem)
					effRate := model.ResolveEffectiveChannelExchangeRate(resolvedCurrency, resolvedRate)
					unitMult := model.ResolveUpstreamToQuotaUnitMultiplier(resolvedCurrency, effRate)
					if entry, ok := buildUpstreamChannelModelCostEntry(ch.Id, ch.Name, ch.Type, mName, base, upstreamGroupRatio, resolvedCurrency, effRate, unitMult, now); ok {
						costEntries[mName] = entry
					}
				}
				if len(costEntries) > 0 {
					model.SetChannelUpstreamModelCosts(ch.Id, costEntries, false)
					itemRes.SyncedPriceCount = len(costEntries)
				}

				if len(upstreamModels) == 0 && len(v1Models) == 0 && v1Err == nil {
					v1Models, v1Err = fetchChannelUpstreamModelIDs(ch)
				}
				// If /v1/models returned models, only include models that are not
				// explicitly restricted to a different group in /api/pricing.
				if v1Err == nil && len(v1Models) > 0 {
					if upstreamGroup == "" || len(upstreamModels) == 0 {
						upstreamModels = mergeModelNames(upstreamModels, v1Models)
					} else {
						for _, vm := range v1Models {
							vm = strings.TrimSpace(vm)
							if vm != "" && !knownPricingModels[vm] {
								upstreamModels = append(upstreamModels, vm)
							}
						}
					}
				}
				if len(upstreamModels) == 0 && v1Err == nil {
					confirmedEmptyNewAPIGroup = true
				}
			} else {
				if pErr != nil {
					upstreamErr = pErr
				}
				if v1Err == nil && len(v1Models) > 0 {
					upstreamModels = v1Models
					upstreamErr = nil
				} else if upstreamErr == nil && v1Err != nil {
					upstreamErr = v1Err
				}
			}
		} else {
			upstreamModels, upstreamErr = fetchChannelUpstreamModelIDs(ch)
		}

		if upstreamErr != nil && len(upstreamModels) == 0 {
			itemRes.Success = false
			itemRes.Error = upstreamErr.Error()
			results = append(results, itemRes)
			continue
		}

		upstreamModels = normalizeModelNames(upstreamModels)
		itemRes.SyncedModelCount = len(upstreamModels)
		itemRes.Success = true

		if syncModels && len(upstreamModels) > 0 {
			existingModels := normalizeModelNames(ch.GetModels())
			ignoredModels := ch.GetOtherSettings().UpstreamModelUpdateIgnoredModels
			added, removed := collectPendingUpstreamModelChangesFromModels(
				existingModels,
				upstreamModels,
				ignoredModels,
				normalizeChannelModelMapping(ch),
			)
			itemRes.AddedModels = added
			var nextModels []string
			if replaceModels {
				itemRes.RemovedModels = removed
				nextModels = applySelectedModelChanges(existingModels, added, removed)
			} else {
				nextModels = mergeModelNames(existingModels, added)
			}
			newModelsStr := strings.Join(nextModels, ",")
			if newModelsStr != ch.Models {
				ch.Models = newModelsStr
				if err := model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("models", ch.Models).Error; err == nil {
					_ = ch.UpdateAbilities(nil)
					anyChannelUpdated = true
				}
			}
		} else if syncModels && replaceModels && confirmedEmptyNewAPIGroup {
			if ch.Models != "" || ch.Status == common.ChannelStatusEnabled {
				ch.Models = ""
				ch.Status = common.ChannelStatusAutoDisabled
				if err := model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{
					"models": "",
					"status": common.ChannelStatusAutoDisabled,
				}).Error; err == nil {
					_ = ch.UpdateAbilities(nil)
					anyChannelUpdated = true
				}
			}
		}

		// Sync channel balance from matched upstream account or standard balance endpoint
		balanceSynced := false
		if ch.Type == constant.ChannelTypeNewAPI {
			normHost := strings.ToLower(model.CleanUpstreamAPIBaseURL(ch.GetBaseURL(), ""))
			if selfBal, hasSelf := upstreamSelfBalByHost[normHost]; hasSelf {
				upGroup := strings.TrimSpace(itemRes.UpstreamGroup)
				if upGroup == "" {
					upGroup = strings.TrimSpace(cfg.ChannelUpstreamGroups[ch.Id])
				}
				var tok upstreamTokenRecord
				hasTok := false
				if byGroup, ok := upstreamTokensByHost[normHost]; ok && upGroup != "" {
					tok, hasTok = byGroup[upGroup]
				}
				if chBalUSD, ok := resolveUpstreamChannelBalanceUSD(tok, hasTok, selfBal); ok {
					ch.Balance = chBalUSD
					ch.BalanceUpdatedTime = now
					if model.DB != nil {
						_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{
							"balance":              chBalUSD,
							"balance_updated_time": now,
						}).Error
					}
					balanceSynced = true
				}
			}
		}
		if !balanceSynced && !ch.ChannelInfo.IsMultiKey && strings.TrimSpace(ch.Key) != "" {
			if balRes, err := updateChannelBalance(ch); err == nil && balRes.RawResponse == "" {
				ch.Balance = balRes.Balance
				ch.BalanceUpdatedTime = now
				balanceSynced = true
			}
		}
		if balanceSynced {
			itemRes.Balance = ch.Balance
			itemRes.BalanceUpdatedTime = ch.BalanceUpdatedTime
			if meta, ok := cfg.AutoProvisionedChannels[ch.Id]; ok {
				meta.ChannelName = ch.Name
				meta.Balance = ch.Balance
				meta.BalanceUpdatedTime = ch.BalanceUpdatedTime
				meta.UpdatedAt = now
				cfg.AutoProvisionedChannels[ch.Id] = meta
			}
		}

		results = append(results, itemRes)
	}

	if anyChannelUpdated {
		model.InitChannelCache()
	}
	model.RecalculateAllChannelModelCosts(true)

	var validIDs map[int]struct{}
	if len(channelIDs) == 0 && model.DB != nil {
		var allIDs []int
		if err := model.DB.Model(&model.Channel{}).Pluck("id", &allIDs).Error; err == nil && len(allIDs) > 0 {
			validIDs = make(map[int]struct{}, len(allIDs))
			for _, id := range allIDs {
				if id > 0 {
					validIDs[id] = struct{}{}
				}
			}
		}
	}

	updatedCfg, _ := model.UpdateModelPriceSyncConfig(func(live *model.ModelPriceSyncConfig) error {
		if highestPricePriorityOverride != nil {
			live.HighestPricePriority = *highestPricePriorityOverride
		}
		if live.ChannelCurrencies == nil {
			live.ChannelCurrencies = make(map[int]string)
		}
		if live.ChannelUpstreamGroups == nil {
			live.ChannelUpstreamGroups = make(map[int]string)
		}
		for chID, curr := range cfg.ChannelCurrencies {
			live.ChannelCurrencies[chID] = curr
		}
		for chID, upGroup := range cfg.ChannelUpstreamGroups {
			live.ChannelUpstreamGroups[chID] = upGroup
		}
		for chID, meta := range cfg.AutoProvisionedChannels {
			if curMeta, ok := live.AutoProvisionedChannels[chID]; ok {
				curMeta.ChannelName = meta.ChannelName
				curMeta.Balance = meta.Balance
				curMeta.BalanceUpdatedTime = meta.BalanceUpdatedTime
				curMeta.UpdatedAt = meta.UpdatedAt
				live.AutoProvisionedChannels[chID] = curMeta
			}
		}
		if validIDs != nil {
			model.PruneOrphanChannelsFromSyncConfig(live, validIDs)
		}
		live.LastSyncTime = now
		return nil
	})

	if syncPrices {
		_, _ = applyAuthoritativeModelPricesToSystem(updatedCfg)
	}

	if updatedCfg.IsClassifierEnabled() && model.DB != nil {
		_, _ = model.ApplyChannelClassifierToChannels(channelIDs)
	}

	return results, nil
}

const maxCronJobOutputBytes = 16384

func GetCronJobs(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"jobs": model.GetCronJobs(),
	})
}

func SaveCronJob(c *gin.Context) {
	var req model.CronJob
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.CronExpr = strings.TrimSpace(req.CronExpr)
	req.JobType = strings.TrimSpace(req.JobType)
	if req.Name == "" {
		common.ApiError(c, common.NewMessage("Job name cannot be empty"))
		return
	}
	if req.CronExpr == "" {
		req.CronExpr = "0 * * * *"
	}
	if err := model.ValidateCronExpr(req.CronExpr); err != nil {
		common.ApiError(c, err)
		return
	}
	switch req.JobType {
	case model.CronJobTypeUpstreamSync, model.CronJobTypeUpstreamGroupSync, model.CronJobTypeChannelTest, model.CronJobTypeChannelBalance, model.CronJobTypeShellScript, model.CronJobTypeHTTPRequest:
	default:
		req.JobType = model.CronJobTypeShellScript
	}
	if (req.JobType == model.CronJobTypeShellScript || req.JobType == model.CronJobTypeHTTPRequest) && strings.TrimSpace(req.Script) == "" {
		common.ApiError(c, common.NewMessage("Script or target URL cannot be empty"))
		return
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 120
	}

	jobs := model.GetCronJobs()
	if req.ID == "" {
		req.ID = fmt.Sprintf("cron-%d", time.Now().UnixNano())
	}
	found := false
	now := time.Now()
	for i := range jobs {
		if jobs[i].ID == req.ID {
			req.BuiltIn = jobs[i].BuiltIn
			if req.BuiltIn {
				req.JobType = jobs[i].JobType
			}
			req.LastRunAt = jobs[i].LastRunAt
			req.LastStatus = jobs[i].LastStatus
			req.LastDurationMs = jobs[i].LastDurationMs
			req.LastOutput = jobs[i].LastOutput
			req.RunCount = jobs[i].RunCount
			if req.Enabled {
				if next, err := model.NextCronTime(req.CronExpr, now); err == nil {
					req.NextRunAt = next.Unix()
				}
			} else {
				req.NextRunAt = 0
			}
			jobs[i] = req
			found = true
			break
		}
	}
	if !found {
		req.LastStatus = "idle"
		if req.Enabled {
			if next, err := model.NextCronTime(req.CronExpr, now); err == nil {
				req.NextRunAt = next.Unix()
			}
		} else {
			req.NextRunAt = 0
		}
		jobs = append(jobs, req)
	}

	if req.ID == model.BuiltInUpstreamSyncCronJobID {
		_, _ = model.UpdateModelPriceSyncConfig(func(syncCfg *model.ModelPriceSyncConfig) error {
			syncCfg.CronEnabled = req.Enabled
			return nil
		})
	}

	if err := model.SaveCronJobs(jobs); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"jobs": model.GetCronJobs(),
	})
}

func DeleteCronJob(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		common.ApiError(c, common.NewMessage("Invalid job ID"))
		return
	}
	if model.IsBuiltInCronJobID(id) {
		common.ApiError(c, common.NewMessage("Built-in cron job cannot be deleted; disable it instead"))
		return
	}
	jobs := model.GetCronJobs()
	filtered := make([]model.CronJob, 0, len(jobs))
	for _, j := range jobs {
		if j.ID != id {
			filtered = append(filtered, j)
		}
	}
	if err := model.SaveCronJobs(filtered); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"jobs": model.GetCronJobs(),
	})
}

func RunCronJobNow(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		common.ApiError(c, common.NewMessage("Invalid job ID"))
		return
	}
	job, err := ExecuteCronJob(c.Request.Context(), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"job":  job,
		"jobs": model.GetCronJobs(),
	})
}

func resolveSystemTaskTypeForCronJob(job model.CronJob) string {
	switch job.JobType {
	case model.CronJobTypeUpstreamGroupSync:
		return model.SystemTaskTypeUpstreamGroupSync
	case model.CronJobTypeUpstreamSync:
		return model.SystemTaskTypeUpstreamSync
	case model.CronJobTypeChannelTest:
		return model.SystemTaskTypeChannelTest
	case model.CronJobTypeChannelBalance:
		return model.SystemTaskTypeChannelBalance
	default:
		return model.SystemTaskTypeCustomCronJob
	}
}

func ExecuteCronJob(ctx context.Context, jobID string) (*model.CronJob, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job ID is empty")
	}
	if _, loaded := cronJobRunningMap.LoadOrStore(jobID, true); loaded {
		return nil, fmt.Errorf("cron job %q is already running", jobID)
	}
	defer cronJobRunningMap.Delete(jobID)

	jobs := model.GetCronJobs()
	idx := -1
	for i := range jobs {
		if jobs[i].ID == jobID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("cron job %q not found", jobID)
	}
	job := jobs[idx]

	sysTaskType := resolveSystemTaskTypeForCronJob(job)
	var sysTask *model.SystemTask
	runnerID := fmt.Sprintf("cron-%s-%s", common.NodeName, common.GetRandomString(6))
	if model.DB != nil {
		if active, err := model.GetActiveSystemTask(sysTaskType); err == nil && active != nil {
			return nil, fmt.Errorf("system task %q is already running", sysTaskType)
		}
		if created, err := model.CreateSystemTask(sysTaskType, map[string]any{
			"job_id":   job.ID,
			"job_name": job.Name,
			"job_type": job.JobType,
		}, nil); err == nil && created != nil {
			lockUntil := common.GetTimestamp() + int64(max(job.TimeoutSec, 60)) + 60
			if claimed, ok, claimErr := model.ClaimSystemTask(created.ID, sysTaskType, runnerID, lockUntil); claimErr == nil && ok && claimed != nil {
				sysTask = claimed
			}
		}
	}

	timeout := time.Duration(max(job.TimeoutSec, 5)) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var output string
	var runErr error

	switch job.JobType {
	case model.CronJobTypeUpstreamGroupSync:
		summary, err := ExecuteUpstreamAccountsProvisionSync(runCtx, nil)
		if err != nil {
			runErr = err
			output = fmt.Sprintf("Upstream account group sync failed: %v", err)
		} else {
			output = summary
		}

	case model.CronJobTypeUpstreamSync:
		cfg := model.GetModelPriceSyncConfig()
		res, err := ExecuteChannelUpstreamSync(runCtx, nil, cfg.AutoSyncModels, cfg.AutoSyncPrices, cfg.DefaultCurrency, cfg.CustomExchangeRate, nil)
		if err != nil {
			runErr = err
			output = fmt.Sprintf("Upstream sync failed: %v", err)
		} else {
			okCount := 0
			totalAdded := 0
			totalRemoved := 0
			totalPrices := 0
			for _, r := range res {
				if r.Success {
					okCount++
					totalAdded += len(r.AddedModels)
					totalRemoved += len(r.RemovedModels)
					totalPrices += r.SyncedPriceCount
				}
			}
			output = fmt.Sprintf("Synced %d/%d channels (models +%d / -%d, prices %d)", okCount, len(res), totalAdded, totalRemoved, totalPrices)
		}

	case model.CronJobTypeChannelTest:
		summary, err := runChannelTestTask(runCtx, "", false, nil)
		if err != nil {
			runErr = err
			output = fmt.Sprintf("Channel test failed: %v", err)
		} else {
			output = fmt.Sprintf("Channel test completed: tested %d, succeeded %d, failed %d", summary.Tested, summary.Succeeded, summary.Failed)
		}

	case model.CronJobTypeChannelBalance:
		_, err := updateAllChannelsBalance(runCtx, func(processed, total int) {})
		if err != nil {
			runErr = err
			output = fmt.Sprintf("Balance update failed: %v", err)
		} else {
			output = "Updated balances for all enabled channels"
		}

	case model.CronJobTypeHTTPRequest:
		targetURL := strings.TrimSpace(job.Script)
		method := strings.ToUpper(strings.TrimSpace(job.HTTPMethod))
		if method == "" {
			method = http.MethodGet
		}
		var bodyReader io.Reader
		if strings.TrimSpace(job.HTTPBody) != "" {
			bodyReader = strings.NewReader(job.HTTPBody)
		}
		httpReq, err := http.NewRequestWithContext(runCtx, method, targetURL, bodyReader)
		if err != nil {
			runErr = err
			output = err.Error()
			break
		}
		if strings.TrimSpace(job.HTTPHeaders) != "" {
			var headers map[string]string
			if err := common.UnmarshalJsonStr(job.HTTPHeaders, &headers); err == nil {
				for k, v := range headers {
					httpReq.Header.Set(k, v)
				}
			}
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(httpReq)
		if err != nil {
			runErr = err
			output = err.Error()
			break
		}
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxCronJobOutputBytes))
		_ = resp.Body.Close()
		output = fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
		if resp.StatusCode >= 400 {
			runErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
		}

	case model.CronJobTypeShellScript:
		scriptText := strings.TrimSpace(job.Script)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			if pwshPath, err := exec.LookPath("pwsh"); err == nil {
				cmd = exec.CommandContext(runCtx, pwshPath, "-NoProfile", "-NonInteractive", "-Command", scriptText)
			} else if psPath, err := exec.LookPath("powershell"); err == nil {
				cmd = exec.CommandContext(runCtx, psPath, "-NoProfile", "-NonInteractive", "-Command", scriptText)
			} else {
				cmd = exec.CommandContext(runCtx, "cmd", "/C", scriptText)
			}
		} else {
			if bashPath, err := exec.LookPath("bash"); err == nil {
				cmd = exec.CommandContext(runCtx, bashPath, "-c", scriptText)
			} else {
				cmd = exec.CommandContext(runCtx, "/bin/sh", "-c", scriptText)
			}
		}
		if strings.TrimSpace(job.WorkingDir) != "" {
			cmd.Dir = strings.TrimSpace(job.WorkingDir)
		}
		outBytes, err := cmd.CombinedOutput()
		if len(outBytes) > maxCronJobOutputBytes {
			outBytes = outBytes[:maxCronJobOutputBytes]
		}
		output = strings.TrimSpace(string(outBytes))
		if err != nil {
			runErr = err
			if output == "" {
				output = err.Error()
			} else {
				output = output + "\nError: " + err.Error()
			}
		}
	}

	finish := time.Now()
	durMs := max(finish.Sub(start).Milliseconds(), 1)
	if sysTask != nil {
		sysStatus := model.SystemTaskStatusSucceeded
		errMsg := ""
		if runErr != nil {
			sysStatus = model.SystemTaskStatusFailed
			errMsg = runErr.Error()
		}
		_ = model.FinishSystemTask(sysTask.TaskID, runnerID, sysStatus, map[string]any{
			"job_id":      job.ID,
			"job_name":    job.Name,
			"duration_ms": durMs,
			"output":      output,
		}, errMsg)
	}

	status := "success"
	if runErr != nil {
		status = "failed"
	}
	updatedJob := model.RecordCronJobRun(jobID, finish, status, durMs, output)
	return updatedJob, nil
}
