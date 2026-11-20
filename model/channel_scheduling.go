package model

import (
	"fmt"
	"maps"
	"math"
	"math/rand"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

const (
	GroupSchedulingConfigOptionKey  = "GroupSchedulingConfig"
	GroupCostControlConfigOptionKey = "GroupCostControlConfig"
	ChannelModelCostsOptionKey      = "ChannelModelCosts"
	ModelPriceSyncConfigOptionKey   = "ModelPriceSyncConfig"
	CronJobsConfigOptionKey         = "CronJobsConfig"

	SchedulingModeStandard     = "standard"
	SchedulingModeBBR          = "bbr"
	SchedulingModeLowerCost    = "lowercost"
	SchedulingModeSpeed        = "speed"
	SchedulingModeAvailability = "availability"
	SchedulingModeCostControl  = "costcontrol"

	DirectUpstreamSourceID = "direct"

	CostControlWindowToday = "today"
	CostControlWindow24h   = "24h"
	CostControlWindow7d    = "7d"
	CostControlWindow30d   = "30d"

	CronJobTypeUpstreamSync      = "upstream_sync"
	CronJobTypeUpstreamGroupSync = "upstream_group_sync"
	CronJobTypeChannelTest       = "channel_test"
	CronJobTypeChannelBalance    = "channel_balance"
	CronJobTypeShellScript       = "shell_script"
	CronJobTypeHTTPRequest       = "http_request"

	BuiltInUpstreamSyncCronJobID      = "builtin-upstream-sync"
	BuiltInUpstreamGroupSyncCronJobID = "builtin-upstream-group-sync"
	BuiltInChannelTestCronJobID       = "builtin-channel-test"
	BuiltInChannelBalanceCronJobID    = "builtin-channel-balance"

	UpstreamSyncModeStandard = "standard"
	UpstreamSyncModeLumiya   = "lumiya"
)

func IsBuiltInCronJobID(id string) bool {
	switch id {
	case BuiltInUpstreamSyncCronJobID,
		BuiltInUpstreamGroupSyncCronJobID,
		BuiltInChannelTestCronJobID,
		BuiltInChannelBalanceCronJobID:
		return true
	default:
		return false
	}
}

type UpstreamAccountSource struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	BaseURL              string   `json:"base_url"`
	SyncMode             string   `json:"sync_mode"` // "standard" | "lumiya"
	UserID               string   `json:"user_id"`
	AccessToken          string   `json:"access_token,omitempty"`
	HasAccessToken       bool     `json:"has_access_token,omitempty"`
	TokenNameRule        string   `json:"token_name_rule"`
	ChannelNameRule      string   `json:"channel_name_rule"`
	ChannelTag           string   `json:"channel_tag"`
	LocalGroup           string   `json:"local_group"`
	Currency             string   `json:"currency"`
	ExchangeRate         float64  `json:"exchange_rate"`
	CleanRemovedGroups   bool     `json:"clean_removed_groups"`
	AdoptExistingChannel bool     `json:"adopt_existing_channel"`
	Enabled              bool     `json:"enabled"`
	BalanceUSD           float64  `json:"balance_usd"`
	UsedQuotaUSD         float64  `json:"used_quota_usd"`
	LastSyncAt           int64    `json:"last_sync_at"`
	LastStatus           string   `json:"last_status"`
	LastMessage          string   `json:"last_message,omitempty"`
	ActiveGroups         []string `json:"active_groups,omitempty"`
	CreatedChannels      []string `json:"created_channels,omitempty"`
	UpdatedChannels      []string `json:"updated_channels,omitempty"`
	RemovedChannels      []string `json:"removed_channels,omitempty"`
}

type AutoProvisionedChannelMeta struct {
	ChannelID          int     `json:"channel_id"`
	ChannelName        string  `json:"channel_name,omitempty"`
	SourceID           string  `json:"source_id"`
	SourceName         string  `json:"source_name"`
	UpstreamGroup      string  `json:"upstream_group"`
	AutoCreated        bool    `json:"auto_created"`
	Balance            float64 `json:"balance"`
	BalanceUpdatedTime int64   `json:"balance_updated_time"`
	CreatedAt          int64   `json:"created_at"`
	UpdatedAt          int64   `json:"updated_at"`
}

type ChannelClassifierRule struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	TargetGroup      string   `json:"target_group"`
	Description      string   `json:"description,omitempty"`
	GroupRatio       float64  `json:"group_ratio"`
	Keywords         []string `json:"keywords"`
	IncludeNativeAPI bool     `json:"include_native_api"`
	Enabled          bool     `json:"enabled"`
}

type ChannelClassificationItemResult struct {
	ChannelID     int      `json:"channel_id"`
	ChannelName   string   `json:"channel_name"`
	ChannelType   int      `json:"channel_type"`
	UpstreamGroup string   `json:"upstream_group,omitempty"`
	IsNativeAPI   bool     `json:"is_native_api"`
	MatchedGroups []string `json:"matched_groups"`
	FinalGroups   string   `json:"final_groups"`
	Changed       bool     `json:"changed"`
}

type ChannelClassificationSummary struct {
	TotalChannels   int                               `json:"total_channels"`
	UpdatedChannels int                               `json:"updated_channels"`
	SyncedGroups    []string                          `json:"synced_groups"`
	Items           []ChannelClassificationItemResult `json:"items"`
}

type CronJob struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Enabled        bool   `json:"enabled"`
	CronExpr       string `json:"cron_expr"`
	JobType        string `json:"job_type"`
	Script         string `json:"script,omitempty"`
	HTTPMethod     string `json:"http_method,omitempty"`
	HTTPHeaders    string `json:"http_headers,omitempty"`
	HTTPBody       string `json:"http_body,omitempty"`
	WorkingDir     string `json:"working_dir,omitempty"`
	TimeoutSec     int    `json:"timeout_sec"`
	BuiltIn        bool   `json:"built_in,omitempty"`
	LastRunAt      int64  `json:"last_run_at"`
	NextRunAt      int64  `json:"next_run_at"`
	LastStatus     string `json:"last_status"`
	LastDurationMs int64  `json:"last_duration_ms"`
	LastOutput     string `json:"last_output,omitempty"`
	RunCount       int64  `json:"run_count"`
}

type ChannelModelCostEntry struct {
	ChannelID        int     `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ChannelType      int     `json:"channel_type"`
	Model            string  `json:"model"`
	InputCost        float64 `json:"input_cost"`         // USD per 1M input tokens (converted by ExchangeRate when Currency != USD)
	OutputCost       float64 `json:"output_cost"`        // USD per 1M output tokens (converted by ExchangeRate when Currency != USD)
	RawInputCost     float64 `json:"raw_input_cost"`     // Original upstream currency per 1M input tokens (before ExchangeRate division)
	RawOutputCost    float64 `json:"raw_output_cost"`    // Original upstream currency per 1M output tokens (before ExchangeRate division)
	ModelRatio       float64 `json:"model_ratio"`        // 1x base model ratio (normalized to USD)
	CompletionRatio  float64 `json:"completion_ratio"`   // output / input ratio
	CacheRatio       float64 `json:"cache_ratio"`        // cache read / input price (0 = unknown)
	CreateCacheRatio float64 `json:"create_cache_ratio"` // cache write / input price (0 = unknown)
	ModelPrice       float64 `json:"model_price"`        // fixed price in USD if QuotaType == 1
	RawModelPrice    float64 `json:"raw_model_price"`    // fixed price in upstream currency before ExchangeRate division
	QuotaType        int     `json:"quota_type"`
	BaseMultiplier   float64 `json:"base_multiplier"` // upstream group ratio (1.0 for official APIs)
	Currency         string  `json:"currency"`        // "USD", "CNY", "TWD"
	ExchangeRate     float64 `json:"exchange_rate"`   // units per 1 USD (1.0 for USD)
	FromUpstream     bool    `json:"from_upstream"`
	UpdatedAt        int64   `json:"updated_at"`
}

type ModelPriceSyncConfig struct {
	HighestPricePriority    bool                               `json:"highest_price_priority"`
	PreferredChannels       map[string]int                     `json:"preferred_channels"`
	DefaultCurrency         string                             `json:"default_currency"`
	CustomExchangeRate      float64                            `json:"custom_exchange_rate"`
	USDExchangeRate         float64                            `json:"usd_exchange_rate"`
	CNYExchangeRate         float64                            `json:"cny_exchange_rate"`
	TWDExchangeRate         float64                            `json:"twd_exchange_rate"`
	ChannelCurrencies       map[int]string                     `json:"channel_currencies"`
	ChannelExchangeRates    map[int]float64                    `json:"channel_exchange_rates"`
	ChannelUpstreamGroups   map[int]string                     `json:"channel_upstream_groups"`
	UpstreamAccounts        []UpstreamAccountSource            `json:"upstream_accounts,omitempty"`
	AutoProvisionedChannels map[int]AutoProvisionedChannelMeta `json:"auto_provisioned_channels,omitempty"`
	ClassifierEnabled       *bool                              `json:"classifier_enabled,omitempty"`
	ClassifierRules         []ChannelClassifierRule            `json:"classifier_rules,omitempty"`
	CronEnabled             bool                               `json:"cron_enabled"`
	CronIntervalMinutes     int                                `json:"cron_interval_minutes"`
	AutoSyncModels          bool                               `json:"auto_sync_models"`
	AutoSyncPrices          bool                               `json:"auto_sync_prices"`
	ReplaceModels           *bool                              `json:"replace_models,omitempty"`
	LastSyncTime            int64                              `json:"last_sync_time"`
}

func (c ModelPriceSyncConfig) IsReplaceModelsEnabled() bool {
	if c.ReplaceModels == nil {
		return true
	}
	return *c.ReplaceModels
}

func (c ModelPriceSyncConfig) IsClassifierEnabled() bool {
	if c.ClassifierEnabled == nil {
		return true
	}
	return *c.ClassifierEnabled
}

type ChannelModelTelemetrySnapshot struct {
	ChannelID      int     `json:"channel_id"`
	Model          string  `json:"model"`
	RequestCount   int64   `json:"request_count"`
	SuccessCount   int64   `json:"success_count"`
	Availability   float64 `json:"availability"`
	TpsMin         float64 `json:"tps_min"`
	TpsAvg         float64 `json:"tps_avg"`
	TpsMax         float64 `json:"tps_max"`
	AvgTtftSec     float64 `json:"avg_ttft_sec"`
	EmaSuccessRate float64 `json:"ema_success_rate"`
	EmaTps         float64 `json:"ema_tps"`
	EmaTtftMs      float64 `json:"ema_ttft_ms"`
}

type channelModelTelemetryState struct {
	mu             sync.Mutex
	requestCount   int64
	successCount   int64
	tpsMin         float64
	tpsMax         float64
	tpsSum         float64
	tpsCount       int64
	ttftSumMs      int64
	ttftCount      int64
	emaSuccessRate float64
	emaTps         float64
	emaTtftMs      float64
	lastSampleAt   time.Time
}

// channelHealthRecoveryHalfLife is how long it takes for half of a channel's
// failure history to fade when the channel receives no new samples. Without
// it, a channel demoted by a burst of failures is never selected again and so
// never gets the successful samples it needs to recover.
const channelHealthRecoveryHalfLife = 5 * time.Minute

// recoverChannelSuccessRate moves an EMA success rate back toward 1.0
// according to the time elapsed since the last sample.
func recoverChannelSuccessRate(rate float64, lastSampleAt time.Time, now time.Time) float64 {
	if lastSampleAt.IsZero() || rate >= 1 {
		return rate
	}
	elapsed := now.Sub(lastSampleAt)
	if elapsed <= 0 {
		return rate
	}
	remaining := math.Pow(0.5, elapsed.Seconds()/channelHealthRecoveryHalfLife.Seconds())
	return 1 - (1-rate)*remaining
}

type CostControlUpstreamAlloc struct {
	Enabled bool `json:"enabled"`
	Weight  int  `json:"weight"`
}

type CostControlChannelRule struct {
	UpstreamID string `json:"upstream_id,omitempty"`
	Priority   *int   `json:"priority,omitempty"`
	Weight     *int   `json:"weight,omitempty"`
}

type GroupCostControlPolicy struct {
	Window              string                              `json:"window"`
	MaxCostRatioPct     float64                             `json:"max_cost_ratio_pct"`
	MinNetProfitUSD     float64                             `json:"min_net_profit_usd"`
	OverflowSensitivity float64                             `json:"overflow_sensitivity"`
	UpstreamAllocations map[string]CostControlUpstreamAlloc `json:"upstream_allocations"`
	ChannelRules        map[int]CostControlChannelRule      `json:"channel_rules"`
}

type RealtimeProfitCounter struct {
	Calls        int64   `json:"calls"`
	RevenueUSD   float64 `json:"revenue_usd"`
	CostUSD      float64 `json:"cost_usd"`
	NetProfitUSD float64 `json:"net_profit_usd"`
}

type GroupRealtimeProfitSnapshot struct {
	Group            string                           `json:"group"`
	Window           string                           `json:"window"`
	WindowStart      int64                            `json:"window_start"`
	UpdatedAt        int64                            `json:"updated_at"`
	TotalCalls       int64                            `json:"total_calls"`
	RevenueUSD       float64                          `json:"revenue_usd"`
	CostUSD          float64                          `json:"cost_usd"`
	NetProfitUSD     float64                          `json:"net_profit_usd"`
	NetMarginPct     float64                          `json:"net_margin_pct"`
	CostRatioPct     float64                          `json:"cost_ratio_pct"`
	MaxCostRatioPct  float64                          `json:"max_cost_ratio_pct"`
	MinNetProfitUSD  float64                          `json:"min_net_profit_usd"`
	AllowedCostUSD   float64                          `json:"allowed_cost_usd"`
	ProfitCushionUSD float64                          `json:"profit_cushion_usd"`
	IsOverCost       bool                             `json:"is_over_cost"`
	DivertProbPct    float64                          `json:"divert_prob_pct"`
	ByUpstream       map[string]RealtimeProfitCounter `json:"by_upstream"`
	ByChannel        map[int]RealtimeProfitCounter    `json:"by_channel"`
}

type groupRealtimeProfitState struct {
	window             string
	windowStart        int64
	lastHydratedAt     int64
	updatedAt          int64
	syncedLogVersion   uint64
	hydrated           bool
	refreshing         bool
	hasUnpersistedData bool
	totalCalls         int64
	revenueUSD         float64
	costUSD            float64
	byUpstream         map[string]RealtimeProfitCounter
	byChannel          map[int]RealtimeProfitCounter
}

type channelModelKey struct {
	channelID int
	model     string
}

var (
	groupSchedulingMu    sync.RWMutex
	groupSchedulingModes = map[string]string{}

	groupCostControlMu       sync.RWMutex
	groupCostControlPolicies = map[string]GroupCostControlPolicy{}

	groupRealtimeProfitMu     sync.RWMutex
	groupRealtimeProfitStates = map[string]*groupRealtimeProfitState{}

	channelModelCostMu  sync.RWMutex
	channelModelCostMap = map[int]map[string]ChannelModelCostEntry{}

	modelPriceSyncMu  sync.RWMutex
	modelPriceSyncCfg = defaultModelPriceSyncConfig()

	cronJobsMu sync.RWMutex
	cronJobs   = defaultCronJobs()

	channelTelemetryMap sync.Map // map[channelModelKey]*channelModelTelemetryState
)

func DefaultGroupCostControlPolicy() GroupCostControlPolicy {
	return GroupCostControlPolicy{
		Window:              CostControlWindow24h,
		MaxCostRatioPct:     100.0,
		MinNetProfitUSD:     0.0,
		OverflowSensitivity: 1.0,
		UpstreamAllocations: make(map[string]CostControlUpstreamAlloc),
		ChannelRules:        make(map[int]CostControlChannelRule),
	}
}

func NormalizeCostControlWindow(window string) string {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case CostControlWindowToday:
		return CostControlWindowToday
	case CostControlWindow24h:
		return CostControlWindow24h
	case CostControlWindow7d:
		return CostControlWindow7d
	case CostControlWindow30d:
		return CostControlWindow30d
	default:
		return CostControlWindow24h
	}
}

func NormalizeGroupCostControlPolicy(p GroupCostControlPolicy) GroupCostControlPolicy {
	p.Window = NormalizeCostControlWindow(p.Window)
	if math.IsNaN(p.MaxCostRatioPct) || math.IsInf(p.MaxCostRatioPct, 0) || p.MaxCostRatioPct <= 0 {
		p.MaxCostRatioPct = 100.0
	} else if p.MaxCostRatioPct > 500.0 {
		p.MaxCostRatioPct = 500.0
	}
	if math.IsNaN(p.MinNetProfitUSD) || math.IsInf(p.MinNetProfitUSD, 0) {
		p.MinNetProfitUSD = 0.0
	}
	if math.IsNaN(p.OverflowSensitivity) || math.IsInf(p.OverflowSensitivity, 0) || p.OverflowSensitivity <= 0 {
		p.OverflowSensitivity = 1.0
	} else {
		p.OverflowSensitivity = min(max(p.OverflowSensitivity, 0.1), 10.0)
	}

	cleanUpstreams := make(map[string]CostControlUpstreamAlloc, len(p.UpstreamAllocations))
	for rawID, alloc := range p.UpstreamAllocations {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if alloc.Weight < 0 {
			alloc.Weight = 0
		} else if alloc.Weight > 10000 {
			alloc.Weight = 10000
		}
		cleanUpstreams[id] = alloc
	}
	p.UpstreamAllocations = cleanUpstreams

	cleanRules := make(map[int]CostControlChannelRule, len(p.ChannelRules))
	for chID, rule := range p.ChannelRules {
		if chID <= 0 {
			continue
		}
		rule.UpstreamID = strings.TrimSpace(rule.UpstreamID)
		if rule.Priority != nil {
			v := min(max(*rule.Priority, -10000), 100000)
			rule.Priority = &v
		}
		if rule.Weight != nil {
			v := min(max(*rule.Weight, 0), 10000)
			rule.Weight = &v
		}
		cleanRules[chID] = rule
	}
	p.ChannelRules = cleanRules
	return p
}

func cloneGroupCostControlPolicy(p GroupCostControlPolicy) GroupCostControlPolicy {
	out := p
	out.UpstreamAllocations = maps.Clone(p.UpstreamAllocations)
	if out.UpstreamAllocations == nil {
		out.UpstreamAllocations = make(map[string]CostControlUpstreamAlloc)
	}
	out.ChannelRules = make(map[int]CostControlChannelRule, len(p.ChannelRules))
	for chID, r := range p.ChannelRules {
		cp := r
		if r.Priority != nil {
			v := *r.Priority
			cp.Priority = &v
		}
		if r.Weight != nil {
			v := *r.Weight
			cp.Weight = &v
		}
		out.ChannelRules[chID] = cp
	}
	return out
}

func GetGroupCostControlPolicy(group string) GroupCostControlPolicy {
	group = strings.TrimSpace(group)
	groupCostControlMu.RLock()
	p, ok := groupCostControlPolicies[group]
	groupCostControlMu.RUnlock()
	if !ok {
		return DefaultGroupCostControlPolicy()
	}
	return cloneGroupCostControlPolicy(NormalizeGroupCostControlPolicy(p))
}

func GetAllGroupCostControlPolicies() map[string]GroupCostControlPolicy {
	groupCostControlMu.RLock()
	defer groupCostControlMu.RUnlock()
	out := make(map[string]GroupCostControlPolicy, len(groupCostControlPolicies))
	for g, p := range groupCostControlPolicies {
		out[g] = cloneGroupCostControlPolicy(NormalizeGroupCostControlPolicy(p))
	}
	return out
}

func LoadGroupCostControlConfigFromJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		groupCostControlMu.Lock()
		groupCostControlPolicies = make(map[string]GroupCostControlPolicy)
		groupCostControlMu.Unlock()
		return nil
	}
	var parsed map[string]GroupCostControlPolicy
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return err
	}
	normalized := make(map[string]GroupCostControlPolicy, len(parsed))
	for g, p := range parsed {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		normalized[g] = NormalizeGroupCostControlPolicy(p)
	}
	groupCostControlMu.Lock()
	groupCostControlPolicies = normalized
	groupCostControlMu.Unlock()
	return nil
}

func SaveGroupCostControlPolicy(group string, policy GroupCostControlPolicy) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return common.NewMessage("Group cannot be empty")
	}
	normalized := NormalizeGroupCostControlPolicy(policy)

	groupCostControlMu.Lock()
	if groupCostControlPolicies == nil {
		groupCostControlPolicies = make(map[string]GroupCostControlPolicy)
	}
	prevWindow := ""
	if prev, exists := groupCostControlPolicies[group]; exists {
		prevWindow = prev.Window
	}
	groupCostControlPolicies[group] = cloneGroupCostControlPolicy(normalized)
	snapshot := make(map[string]GroupCostControlPolicy, len(groupCostControlPolicies))
	for g, p := range groupCostControlPolicies {
		snapshot[g] = cloneGroupCostControlPolicy(p)
	}
	groupCostControlMu.Unlock()

	if prevWindow != "" && prevWindow != normalized.Window {
		groupRealtimeProfitMu.Lock()
		delete(groupRealtimeProfitStates, group)
		groupRealtimeProfitMu.Unlock()
	}

	encoded, err := common.Marshal(snapshot)
	if err != nil {
		return err
	}
	if DB == nil {
		return nil
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	return UpdateOption(GroupCostControlConfigOptionKey, string(encoded))
}

func defaultChannelClassifierRules() []ChannelClassifierRule {
	return []ChannelClassifierRule{
		{
			ID:               "builtin-classifier-ctf",
			Name:             "乖乖分組 (CTF)",
			TargetGroup:      "ctf",
			Description:      "乖乖分組 (CTF)",
			GroupRatio:       1.0,
			Keywords:         []string{"乖乖", "ctf"},
			IncludeNativeAPI: false,
			Enabled:          true,
		},
		{
			ID:               "builtin-classifier-official",
			Name:             "官Key與原生API分組",
			TargetGroup:      "official",
			Description:      "官Key與原生API分組",
			GroupRatio:       1.0,
			Keywords:         []string{"官", "官key", "官方", "原生", "official"},
			IncludeNativeAPI: true,
			Enabled:          true,
		},
		{
			ID:               "builtin-classifier-full",
			Name:             "不降智 / 滿智分組",
			TargetGroup:      "full",
			Description:      "不降智 / 滿智分組",
			GroupRatio:       1.0,
			Keywords:         []string{"不降智", "滿智", "满智", "滿血", "满血"},
			IncludeNativeAPI: false,
			Enabled:          true,
		},
	}
}

func normalizeChannelClassifierRules(rules []ChannelClassifierRule) []ChannelClassifierRule {
	if rules == nil {
		return defaultChannelClassifierRules()
	}
	out := make([]ChannelClassifierRule, 0, len(rules))
	seenIDs := make(map[string]struct{}, len(rules))
	for i, r := range rules {
		r.ID = strings.TrimSpace(r.ID)
		r.Name = strings.TrimSpace(r.Name)
		r.TargetGroup = strings.TrimSpace(r.TargetGroup)
		r.Description = strings.TrimSpace(r.Description)
		if r.TargetGroup == "" {
			continue
		}
		if r.Name == "" {
			r.Name = r.TargetGroup
		}
		if r.Description == "" {
			r.Description = r.Name
		}
		if r.ID == "" {
			r.ID = fmt.Sprintf("classifier-%s-%d", r.TargetGroup, i+1)
		}
		if _, exists := seenIDs[r.ID]; exists {
			r.ID = fmt.Sprintf("%s-%d", r.ID, i+1)
		}
		seenIDs[r.ID] = struct{}{}
		if r.GroupRatio <= 0 || math.IsNaN(r.GroupRatio) || math.IsInf(r.GroupRatio, 0) {
			r.GroupRatio = 1.0
		}
		cleanKws := make([]string, 0, len(r.Keywords))
		seenKw := make(map[string]struct{}, len(r.Keywords))
		for _, kw := range r.Keywords {
			for part := range strings.SplitSeq(kw, ",") {
				k := strings.TrimSpace(part)
				if k == "" {
					continue
				}
				lowerK := strings.ToLower(k)
				if _, ok := seenKw[lowerK]; !ok {
					seenKw[lowerK] = struct{}{}
					cleanKws = append(cleanKws, k)
				}
			}
		}
		r.Keywords = cleanKws
		out = append(out, r)
	}
	return out
}

func defaultModelPriceSyncConfig() ModelPriceSyncConfig {
	replace := true
	classifierEnabled := true
	return ModelPriceSyncConfig{
		HighestPricePriority:    true,
		PreferredChannels:       make(map[string]int),
		DefaultCurrency:         "AUTO",
		CustomExchangeRate:      0,
		USDExchangeRate:         35.0,
		CNYExchangeRate:         5.0,
		TWDExchangeRate:         1.0,
		ChannelCurrencies:       make(map[int]string),
		ChannelExchangeRates:    make(map[int]float64),
		ChannelUpstreamGroups:   make(map[int]string),
		UpstreamAccounts:        make([]UpstreamAccountSource, 0),
		AutoProvisionedChannels: make(map[int]AutoProvisionedChannelMeta),
		ClassifierEnabled:       &classifierEnabled,
		ClassifierRules:         defaultChannelClassifierRules(),
		CronEnabled:             false,
		CronIntervalMinutes:     60,
		AutoSyncModels:          true,
		AutoSyncPrices:          true,
		ReplaceModels:           &replace,
	}
}

func defaultCronJobs() []CronJob {
	return []CronJob{
		{
			ID:          BuiltInUpstreamGroupSyncCronJobID,
			Name:        "Upstream Account & Group Channel Sync",
			Description: "Sync upstream New API account groups into channels, auto-create upstream tokens, and remove channels for deleted upstream groups",
			Enabled:     false,
			CronExpr:    "0 * * * *",
			JobType:     CronJobTypeUpstreamGroupSync,
			TimeoutSec:  180,
			BuiltIn:     true,
			LastStatus:  "idle",
		},
		{
			ID:          BuiltInUpstreamSyncCronJobID,
			Name:        "Channel Upstream Model & Price Sync",
			Description: "Sync upstream channel model lists and 1x base pricing automatically",
			Enabled:     false,
			CronExpr:    "0 * * * *",
			JobType:     CronJobTypeUpstreamSync,
			TimeoutSec:  120,
			BuiltIn:     true,
			LastStatus:  "idle",
		},
		{
			ID:          BuiltInChannelTestCronJobID,
			Name:        "Test All Enabled Channels",
			Description: "Periodically test all enabled channels to detect latency and availability issues",
			Enabled:     false,
			CronExpr:    "0 */6 * * *",
			JobType:     CronJobTypeChannelTest,
			TimeoutSec:  300,
			BuiltIn:     true,
			LastStatus:  "idle",
		},
		{
			ID:          BuiltInChannelBalanceCronJobID,
			Name:        "Update All Channel Balances",
			Description: "Periodically refresh remaining balances for all enabled channels",
			Enabled:     false,
			CronExpr:    "0 * * * *",
			JobType:     CronJobTypeChannelBalance,
			TimeoutSec:  180,
			BuiltIn:     true,
			LastStatus:  "idle",
		},
	}
}

func NormalizeSchedulingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SchedulingModeBBR:
		return SchedulingModeBBR
	case SchedulingModeLowerCost, "lower_cost", "cost":
		return SchedulingModeLowerCost
	case SchedulingModeSpeed:
		return SchedulingModeSpeed
	case SchedulingModeAvailability:
		return SchedulingModeAvailability
	case SchedulingModeCostControl, "cost_control", "cost-control":
		return SchedulingModeCostControl
	default:
		return SchedulingModeStandard
	}
}

func GetGroupSchedulingModes() map[string]string {
	groupSchedulingMu.RLock()
	defer groupSchedulingMu.RUnlock()
	out := make(map[string]string, len(groupSchedulingModes))
	maps.Copy(out, groupSchedulingModes)
	return out
}

func GetGroupSchedulingMode(group string) string {
	groupSchedulingMu.RLock()
	defer groupSchedulingMu.RUnlock()
	if mode, ok := groupSchedulingModes[group]; ok && mode != "" {
		return NormalizeSchedulingMode(mode)
	}
	return SchedulingModeStandard
}

func LoadGroupSchedulingConfigFromJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		groupSchedulingMu.Lock()
		groupSchedulingModes = make(map[string]string)
		groupSchedulingMu.Unlock()
		return nil
	}
	var parsed map[string]string
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return err
	}
	normalized := make(map[string]string, len(parsed))
	for g, mode := range parsed {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		normalized[g] = NormalizeSchedulingMode(mode)
	}
	groupSchedulingMu.Lock()
	groupSchedulingModes = normalized
	groupSchedulingMu.Unlock()
	return nil
}

func SaveGroupSchedulingModes(modes map[string]string) error {
	groupSchedulingMu.Lock()
	if groupSchedulingModes == nil {
		groupSchedulingModes = make(map[string]string)
	}
	for g, m := range modes {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		groupSchedulingModes[g] = NormalizeSchedulingMode(m)
	}
	snapshot := make(map[string]string, len(groupSchedulingModes))
	maps.Copy(snapshot, groupSchedulingModes)
	groupSchedulingMu.Unlock()

	encoded, err := common.Marshal(snapshot)
	if err != nil {
		return err
	}
	if DB == nil {
		return nil
	}
	return UpdateOption(GroupSchedulingConfigOptionKey, string(encoded))
}

// UpdateGroupChannels updates which channels belong to the target group and refreshes abilities & cache.
func UpdateGroupChannels(group string, enabledChannelIDs []int) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return common.NewMessage("Group cannot be empty")
	}
	enabledSet := make(map[int]struct{}, len(enabledChannelIDs))
	for _, id := range enabledChannelIDs {
		if id > 0 {
			enabledSet[id] = struct{}{}
		}
	}

	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return err
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		for _, ch := range channels {
			existingGroups := make([]string, 0)
			hasTarget := false
			for g := range strings.SplitSeq(ch.Group, ",") {
				g = strings.TrimSpace(g)
				if g == "" {
					continue
				}
				if g == group {
					hasTarget = true
				}
				if !slices.Contains(existingGroups, g) {
					existingGroups = append(existingGroups, g)
				}
			}

			_, shouldEnable := enabledSet[ch.Id]
			if shouldEnable == hasTarget {
				continue
			}

			var updatedGroups []string
			if shouldEnable {
				updatedGroups = append(existingGroups, group)
			} else {
				for _, g := range existingGroups {
					if g != group {
						updatedGroups = append(updatedGroups, g)
					}
				}
			}

			ch.Group = strings.Join(updatedGroups, ",")
			if err := tx.Model(&Channel{}).Where("id = ?", ch.Id).Update("group", ch.Group).Error; err != nil {
				return err
			}
			if err := ch.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	InitChannelCache()
	return nil
}

func GetModelPriceSyncConfig() ModelPriceSyncConfig {
	modelPriceSyncMu.RLock()
	defer modelPriceSyncMu.RUnlock()
	cfg := modelPriceSyncCfg
	cfg.PreferredChannels = maps.Clone(modelPriceSyncCfg.PreferredChannels)
	if cfg.PreferredChannels == nil {
		cfg.PreferredChannels = make(map[string]int)
	}
	cfg.ChannelCurrencies = maps.Clone(modelPriceSyncCfg.ChannelCurrencies)
	if cfg.ChannelCurrencies == nil {
		cfg.ChannelCurrencies = make(map[int]string)
	}
	cfg.ChannelExchangeRates = maps.Clone(modelPriceSyncCfg.ChannelExchangeRates)
	if cfg.ChannelExchangeRates == nil {
		cfg.ChannelExchangeRates = make(map[int]float64)
	}
	cfg.ChannelUpstreamGroups = maps.Clone(modelPriceSyncCfg.ChannelUpstreamGroups)
	if cfg.ChannelUpstreamGroups == nil {
		cfg.ChannelUpstreamGroups = make(map[int]string)
	}
	cfg.UpstreamAccounts = slices.Clone(modelPriceSyncCfg.UpstreamAccounts)
	for i := range cfg.UpstreamAccounts {
		normalizeUpstreamAccountSource(&cfg.UpstreamAccounts[i])
		cfg.UpstreamAccounts[i].ActiveGroups = slices.Clone(cfg.UpstreamAccounts[i].ActiveGroups)
		cfg.UpstreamAccounts[i].CreatedChannels = slices.Clone(cfg.UpstreamAccounts[i].CreatedChannels)
		cfg.UpstreamAccounts[i].UpdatedChannels = slices.Clone(cfg.UpstreamAccounts[i].UpdatedChannels)
		cfg.UpstreamAccounts[i].RemovedChannels = slices.Clone(cfg.UpstreamAccounts[i].RemovedChannels)
	}
	cfg.AutoProvisionedChannels = maps.Clone(modelPriceSyncCfg.AutoProvisionedChannels)
	if cfg.AutoProvisionedChannels == nil {
		cfg.AutoProvisionedChannels = make(map[int]AutoProvisionedChannelMeta)
	}
	if cfg.ClassifierRules == nil {
		cfg.ClassifierRules = defaultChannelClassifierRules()
	} else {
		cfg.ClassifierRules = slices.Clone(modelPriceSyncCfg.ClassifierRules)
		for i := range cfg.ClassifierRules {
			cfg.ClassifierRules[i].Keywords = slices.Clone(cfg.ClassifierRules[i].Keywords)
		}
		cfg.ClassifierRules = normalizeChannelClassifierRules(cfg.ClassifierRules)
	}
	if cfg.ClassifierEnabled == nil {
		enabled := true
		cfg.ClassifierEnabled = &enabled
	} else {
		enabled := *cfg.ClassifierEnabled
		cfg.ClassifierEnabled = &enabled
	}
	if cfg.DefaultCurrency == "" {
		cfg.DefaultCurrency = "AUTO"
	}
	if cfg.USDExchangeRate <= 0 {
		cfg.USDExchangeRate = 35.0
	}
	if cfg.CNYExchangeRate <= 0 {
		cfg.CNYExchangeRate = 5.0
	}
	if cfg.TWDExchangeRate <= 0 {
		cfg.TWDExchangeRate = 1.0
	}
	if cfg.CronIntervalMinutes <= 0 {
		cfg.CronIntervalMinutes = 60
	}
	return cfg
}

func LoadModelPriceSyncConfigFromJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		modelPriceSyncMu.Lock()
		modelPriceSyncCfg = defaultModelPriceSyncConfig()
		modelPriceSyncMu.Unlock()
		return nil
	}
	cfg := defaultModelPriceSyncConfig()
	if err := common.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	if cfg.PreferredChannels == nil {
		cfg.PreferredChannels = make(map[string]int)
	}
	if cfg.ChannelCurrencies == nil {
		cfg.ChannelCurrencies = make(map[int]string)
	}
	if cfg.ChannelExchangeRates == nil {
		cfg.ChannelExchangeRates = make(map[int]float64)
	}
	if cfg.ChannelUpstreamGroups == nil {
		cfg.ChannelUpstreamGroups = make(map[int]string)
	}
	if cfg.AutoProvisionedChannels == nil {
		cfg.AutoProvisionedChannels = make(map[int]AutoProvisionedChannelMeta)
	}
	cfg.ClassifierRules = normalizeChannelClassifierRules(cfg.ClassifierRules)
	if cfg.ClassifierEnabled == nil {
		enabled := true
		cfg.ClassifierEnabled = &enabled
	}
	if cfg.DefaultCurrency == "" {
		cfg.DefaultCurrency = "AUTO"
	}
	if cfg.USDExchangeRate <= 0 {
		cfg.USDExchangeRate = 35.0
	}
	if cfg.CNYExchangeRate <= 0 {
		cfg.CNYExchangeRate = 5.0
	}
	if cfg.TWDExchangeRate <= 0 {
		cfg.TWDExchangeRate = 1.0
	}
	if cfg.CronIntervalMinutes <= 0 {
		cfg.CronIntervalMinutes = 60
	}
	for i := range cfg.UpstreamAccounts {
		normalizeUpstreamAccountSource(&cfg.UpstreamAccounts[i])
	}
	modelPriceSyncMu.Lock()
	modelPriceSyncCfg = cfg
	modelPriceSyncMu.Unlock()
	return nil
}

func SaveModelPriceSyncConfig(cfg ModelPriceSyncConfig) error {
	if cfg.PreferredChannels == nil {
		cfg.PreferredChannels = make(map[string]int)
	}
	if cfg.ChannelCurrencies == nil {
		cfg.ChannelCurrencies = make(map[int]string)
	}
	if cfg.ChannelExchangeRates == nil {
		cfg.ChannelExchangeRates = make(map[int]float64)
	}
	if cfg.ChannelUpstreamGroups == nil {
		cfg.ChannelUpstreamGroups = make(map[int]string)
	}
	if cfg.AutoProvisionedChannels == nil {
		cfg.AutoProvisionedChannels = make(map[int]AutoProvisionedChannelMeta)
	}
	cfg.ClassifierRules = normalizeChannelClassifierRules(cfg.ClassifierRules)
	if cfg.ClassifierEnabled == nil {
		enabled := true
		cfg.ClassifierEnabled = &enabled
	}
	if cfg.DefaultCurrency == "" {
		cfg.DefaultCurrency = "AUTO"
	}
	if cfg.USDExchangeRate <= 0 {
		cfg.USDExchangeRate = 35.0
	}
	if cfg.CNYExchangeRate <= 0 {
		cfg.CNYExchangeRate = 5.0
	}
	if cfg.TWDExchangeRate <= 0 {
		cfg.TWDExchangeRate = 1.0
	}
	if cfg.CronIntervalMinutes <= 0 {
		cfg.CronIntervalMinutes = 60
	}
	for i := range cfg.UpstreamAccounts {
		normalizeUpstreamAccountSource(&cfg.UpstreamAccounts[i])
	}
	modelPriceSyncMu.Lock()
	modelPriceSyncCfg = cfg
	modelPriceSyncMu.Unlock()

	encoded, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	if DB == nil {
		return nil
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	return UpdateOption(ModelPriceSyncConfigOptionKey, string(encoded))
}

func normalizeUpstreamAccountSource(src *UpstreamAccountSource) {
	if src == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(src.SyncMode))
	isLumiyaLike := strings.Contains(strings.ToLower(src.Name), "lumiya") ||
		strings.Contains(strings.ToLower(src.BaseURL), "lumiya") ||
		strings.Contains(strings.ToLower(src.BaseURL), "xn--1ys141f4ks") ||
		strings.Contains(src.BaseURL, "露米娅") ||
		strings.Contains(src.BaseURL, "露米婭")
	if mode == UpstreamSyncModeLumiya || (mode == "" && isLumiyaLike) {
		src.SyncMode = UpstreamSyncModeLumiya
	} else {
		src.SyncMode = UpstreamSyncModeStandard
	}
	if isLumiyaLike && strings.TrimSpace(src.AccessToken) == "uxGSJ4IdALqzBHMEbUYjicNe1qV/3Q==" {
		src.AccessToken = "8xfOUw1xHsz/dOVmfU4vuJx8T3GsFHk="
	}
	if isLumiyaLike && strings.TrimSpace(src.UserID) == "" {
		src.UserID = "502"
	}
}

func normalizeCronJobsList(jobs []CronJob) []CronJob {
	now := time.Now()
	cfg := GetModelPriceSyncConfig()
	out := make([]CronJob, 0, max(len(jobs), 4))
	hasBuiltInSync := false
	hasBuiltInGroupSync := false
	hasBuiltInChannelTest := false
	hasBuiltInChannelBalance := false
	seenIDs := make(map[string]struct{}, len(jobs))
	for _, j := range jobs {
		j.ID = strings.TrimSpace(j.ID)
		j.Name = strings.TrimSpace(j.Name)
		j.CronExpr = strings.TrimSpace(j.CronExpr)
		j.JobType = strings.TrimSpace(j.JobType)
		if j.ID == "" || j.Name == "" {
			continue
		}
		if _, exists := seenIDs[j.ID]; exists {
			continue
		}
		seenIDs[j.ID] = struct{}{}
		if j.CronExpr == "" {
			j.CronExpr = "0 * * * *"
		}
		if j.JobType == "" {
			j.JobType = CronJobTypeShellScript
		}
		if j.TimeoutSec <= 0 {
			j.TimeoutSec = 120
		}
		if j.LastStatus == "" {
			j.LastStatus = "idle"
		}
		if j.ID == BuiltInUpstreamSyncCronJobID {
			j.BuiltIn = true
			j.JobType = CronJobTypeUpstreamSync
			hasBuiltInSync = true
		}
		if j.ID == BuiltInUpstreamGroupSyncCronJobID {
			j.BuiltIn = true
			j.JobType = CronJobTypeUpstreamGroupSync
			hasBuiltInGroupSync = true
		}
		if j.ID == BuiltInChannelTestCronJobID {
			j.BuiltIn = true
			j.JobType = CronJobTypeChannelTest
			hasBuiltInChannelTest = true
		}
		if j.ID == BuiltInChannelBalanceCronJobID {
			j.BuiltIn = true
			j.JobType = CronJobTypeChannelBalance
			hasBuiltInChannelBalance = true
		}
		if j.Enabled {
			if j.NextRunAt <= 0 {
				baseTime := now
				if j.LastRunAt > 0 {
					baseTime = time.Unix(j.LastRunAt, 0)
				}
				if next, err := NextCronTime(j.CronExpr, baseTime); err == nil {
					j.NextRunAt = next.Unix()
				}
			}
		} else {
			j.NextRunAt = 0
		}
		out = append(out, j)
	}
	if !hasBuiltInSync {
		intervalMin := max(cfg.CronIntervalMinutes, 5)
		cronExpr := "0 * * * *"
		if intervalMin < 60 {
			cronExpr = fmt.Sprintf("*/%d * * * *", intervalMin)
		} else if intervalMin > 60 {
			cronExpr = fmt.Sprintf("@every %dm", intervalMin)
		}
		builtIn := CronJob{
			ID:          BuiltInUpstreamSyncCronJobID,
			Name:        "Channel Upstream Model & Price Sync",
			Description: "Sync upstream channel model lists and 1x base pricing automatically",
			Enabled:     cfg.CronEnabled,
			CronExpr:    cronExpr,
			JobType:     CronJobTypeUpstreamSync,
			TimeoutSec:  120,
			BuiltIn:     true,
			LastRunAt:   cfg.LastSyncTime,
			LastStatus:  "idle",
		}
		if builtIn.Enabled {
			baseTime := now
			if builtIn.LastRunAt > 0 {
				baseTime = time.Unix(builtIn.LastRunAt, 0)
			}
			if next, err := NextCronTime(builtIn.CronExpr, baseTime); err == nil {
				builtIn.NextRunAt = next.Unix()
			}
		}
		out = append([]CronJob{builtIn}, out...)
	}
	if !hasBuiltInGroupSync {
		builtInGroup := CronJob{
			ID:          BuiltInUpstreamGroupSyncCronJobID,
			Name:        "Upstream Account & Group Channel Sync",
			Description: "Sync upstream New API account groups into channels, auto-create upstream tokens, and remove channels for deleted upstream groups",
			Enabled:     false,
			CronExpr:    "0 * * * *",
			JobType:     CronJobTypeUpstreamGroupSync,
			TimeoutSec:  180,
			BuiltIn:     true,
			LastStatus:  "idle",
		}
		out = append([]CronJob{builtInGroup}, out...)
	}
	if !hasBuiltInChannelBalance {
		out = append(out, CronJob{
			ID:          BuiltInChannelBalanceCronJobID,
			Name:        "Update All Channel Balances",
			Description: "Periodically refresh remaining balances for all enabled channels",
			Enabled:     false,
			CronExpr:    "0 * * * *",
			JobType:     CronJobTypeChannelBalance,
			TimeoutSec:  180,
			BuiltIn:     true,
			LastStatus:  "idle",
		})
	}
	if !hasBuiltInChannelTest {
		out = append(out, CronJob{
			ID:          BuiltInChannelTestCronJobID,
			Name:        "Test All Enabled Channels",
			Description: "Periodically test all enabled channels to detect latency and availability issues",
			Enabled:     false,
			CronExpr:    "0 */6 * * *",
			JobType:     CronJobTypeChannelTest,
			TimeoutSec:  300,
			BuiltIn:     true,
			LastStatus:  "idle",
		})
	}
	return out
}

func GetCronJobs() []CronJob {
	cronJobsMu.RLock()
	cloned := slices.Clone(cronJobs)
	cronJobsMu.RUnlock()
	return normalizeCronJobsList(cloned)
}

func LoadCronJobsConfigFromJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		cronJobsMu.Lock()
		cronJobs = normalizeCronJobsList(defaultCronJobs())
		cronJobsMu.Unlock()
		return nil
	}
	var parsed []CronJob
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return err
	}
	normalized := normalizeCronJobsList(parsed)
	cronJobsMu.Lock()
	cronJobs = normalized
	cronJobsMu.Unlock()
	return nil
}

func SaveCronJobs(jobs []CronJob) error {
	normalized := normalizeCronJobsList(jobs)
	cronJobsMu.Lock()
	cronJobs = normalized
	cronJobsMu.Unlock()

	encoded, err := common.Marshal(normalized)
	if err != nil {
		return err
	}
	if DB == nil {
		return nil
	}
	return UpdateOption(CronJobsConfigOptionKey, string(encoded))
}

func ValidateCronExpr(expr string) error {
	_, err := NextCronTime(expr, time.Now())
	return err
}

// NextCronTime computes the next execution time strictly after `after` for a
// standard 5-field cron expression ("min hour dom month dow") or @every/@hourly/@daily/@weekly/@monthly.
func NextCronTime(expr string, after time.Time) (time.Time, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return time.Time{}, fmt.Errorf("cron expression cannot be empty")
	}
	lower := strings.ToLower(expr)
	switch lower {
	case "@hourly":
		expr = "0 * * * *"
	case "@daily", "@midnight":
		expr = "0 0 * * *"
	case "@weekly":
		expr = "0 0 * * 0"
	case "@monthly":
		expr = "0 0 1 * *"
	default:
		if durStr, ok := strings.CutPrefix(lower, "@every "); ok {
			durStr = strings.TrimSpace(durStr)
			if mins, err := strconv.Atoi(durStr); err == nil && mins > 0 {
				return after.Add(time.Duration(mins) * time.Minute), nil
			}
			d, err := time.ParseDuration(durStr)
			if err != nil || d < time.Minute {
				return time.Time{}, fmt.Errorf("invalid @every duration %q (minimum 1m)", durStr)
			}
			return after.Add(d), nil
		}
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected 5 cron fields (min hour dom month dow), got %d", len(fields))
	}

	minMask, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid minute field %q: %w", fields[0], err)
	}
	hourMask, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour field %q: %w", fields[1], err)
	}
	domMask, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day-of-month field %q: %w", fields[2], err)
	}
	monthMask, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month field %q: %w", fields[3], err)
	}
	dowMask, err := parseCronField(fields[4], 0, 7)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day-of-week field %q: %w", fields[4], err)
	}
	if (dowMask & (1 << 7)) != 0 {
		dowMask |= (1 << 0)
	}

	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(366 * 24 * time.Hour)
	for !t.After(limit) {
		if (monthMask & (1 << uint(t.Month()))) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if (domMask&(1<<uint(t.Day()))) == 0 || (dowMask&(1<<uint(t.Weekday()))) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if (hourMask & (1 << uint(t.Hour()))) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if (minMask & (1 << uint(t.Minute()))) == 0 {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("no matching time within 1 year for cron expression %q", expr)
}

func parseCronField(field string, minVal int, maxVal int) (uint64, error) {
	var mask uint64
	for part := range strings.SplitSeq(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, fmt.Errorf("empty cron segment")
		}
		step := 1
		rangePart := part
		if base, stepStr, hasSlash := strings.Cut(part, "/"); hasSlash {
			rangePart = base
			s, err := strconv.Atoi(stepStr)
			if err != nil || s <= 0 {
				return 0, fmt.Errorf("invalid step %q", stepStr)
			}
			step = s
		}
		var start, end int
		if rangePart == "*" {
			start = minVal
			end = maxVal
		} else if loStr, hiStr, hasDash := strings.Cut(rangePart, "-"); hasDash {
			var err1, err2 error
			start, err1 = strconv.Atoi(loStr)
			end, err2 = strconv.Atoi(hiStr)
			if err1 != nil || err2 != nil || start < minVal || end > maxVal || start > end {
				return 0, fmt.Errorf("invalid range %q", rangePart)
			}
		} else {
			v, err := strconv.Atoi(rangePart)
			if err != nil || v < minVal || v > maxVal {
				return 0, fmt.Errorf("value %q out of bounds [%d, %d]", rangePart, minVal, maxVal)
			}
			start = v
			if strings.Contains(part, "/") {
				end = maxVal
			} else {
				end = v
			}
		}
		for i := start; i <= end; i += step {
			mask |= (1 << uint(i))
		}
	}
	if mask == 0 {
		return 0, fmt.Errorf("no valid values in %q", field)
	}
	return mask, nil
}

func LoadChannelModelCostsFromJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var parsed map[int]map[string]ChannelModelCostEntry
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return err
	}
	channelModelCostMu.Lock()
	channelModelCostMap = parsed
	channelModelCostMu.Unlock()
	return nil
}

var (
	localTierVarFirstCoeffRe = regexp.MustCompile(`\b(p|c|cr|cc|cc1h)\s*\*\s*(\d+(?:\.\d+)?)`)
	localTierVarLastCoeffRe  = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*\*\s*\b(p|c|cr|cc|cc1h)`)
)

func deriveLocalExprBasePricing(expr string) (modelRatio, completionRatio, cacheRatio, createCacheRatio float64, ok bool) {
	var best map[string]float64
	rem := expr
	for {
		idx := strings.Index(rem, `tier("`)
		if idx < 0 {
			break
		}
		rem = rem[idx+len(`tier("`):]
		nameEnd := strings.Index(rem, `"`)
		if nameEnd < 0 {
			break
		}
		rest := strings.TrimLeft(rem[nameEnd+1:], " \t")
		if !strings.HasPrefix(rest, ",") {
			rem = rem[nameEnd+1:]
			continue
		}
		body := rest[1:]
		end := strings.Index(body, ")")
		if end < 0 {
			break
		}
		arg := strings.TrimSpace(body[:end])
		rem = body[end:]

		coeffs := make(map[string]float64)
		for _, m := range localTierVarFirstCoeffRe.FindAllStringSubmatch(arg, -1) {
			if _, exists := coeffs[m[1]]; !exists {
				if v, err := strconv.ParseFloat(m[2], 64); err == nil {
					coeffs[m[1]] = v
				}
			}
		}
		for _, m := range localTierVarLastCoeffRe.FindAllStringSubmatch(arg, -1) {
			if _, exists := coeffs[m[2]]; !exists {
				if v, err := strconv.ParseFloat(m[1], 64); err == nil {
					coeffs[m[2]] = v
				}
			}
		}
		if p, hasP := coeffs["p"]; hasP && p > 0 {
			if best == nil || p < best["p"] {
				best = coeffs
			}
		}
	}
	if best == nil {
		return 0, 0, 0, 0, false
	}
	p := best["p"]
	modelRatio = roundCost(p / 2.0)
	completionRatio = 1.0
	if c, hasC := best["c"]; hasC && c > 0 {
		completionRatio = roundCost(c / p)
	}
	if cr, hasCr := best["cr"]; hasCr {
		cacheRatio = roundCost(cr / p)
	}
	if cc, hasCc := best["cc"]; hasCc {
		createCacheRatio = roundCost(cc / p)
	}
	return modelRatio, completionRatio, cacheRatio, createCacheRatio, true
}

var channelTrailingMultiplierRe = regexp.MustCompile(`(?:^|[-_])(\d+(?:\.\d+)?)$`)

// InferChannelTrailingMultiplier extracts a trailing numeric multiplier from a channel name
// such as "lumiya-Gemini-0.5" -> 0.5 or "moyuu-codex_astra_临时-0.18" -> 0.18.
func InferChannelTrailingMultiplier(channelName string) (float64, bool) {
	m := channelTrailingMultiplierRe.FindStringSubmatch(strings.TrimSpace(channelName))
	if len(m) != 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v <= 0 || v > 100 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func resolveChannelMappedModelTarget(ch *Channel, modelName string) string {
	if ch == nil || ch.ModelMapping == nil {
		return ""
	}
	raw := strings.TrimSpace(*ch.ModelMapping)
	if raw == "" || raw == "{}" {
		return ""
	}
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(raw, &mapping); err != nil || len(mapping) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(modelName)
	if target, cyclic := followChannelModelMapping(mapping, trimmed); !cyclic && target != "" && target != trimmed {
		return strings.TrimSpace(target)
	}
	for k, v := range mapping {
		if strings.EqualFold(strings.TrimSpace(k), trimmed) {
			if target, cyclic := followChannelModelMapping(mapping, k); !cyclic && target != "" && !strings.EqualFold(target, trimmed) {
				return strings.TrimSpace(target)
			}
			vTrim := strings.TrimSpace(v)
			if vTrim != "" && !strings.EqualFold(vTrim, trimmed) {
				return vTrim
			}
		}
	}
	return ""
}

// ResolveChannelCurrencyAndMultiplier determines a channel's effective currency, display exchange rate,
// and base multiplier from sync config, existing upstream entries on the same channel, or channel name/type.
func ResolveChannelCurrencyAndMultiplier(ch *Channel, existingEntries map[string]ChannelModelCostEntry, syncCfg ModelPriceSyncConfig) (currency string, effRate float64, baseMult float64) {
	baseMult = 1.0
	currency = ""
	rawRate := 0.0

	if ch != nil && ch.Id > 0 {
		if c, ok := syncCfg.ChannelCurrencies[ch.Id]; ok && strings.TrimSpace(c) != "" && !strings.EqualFold(strings.TrimSpace(c), "AUTO") {
			currency = strings.ToUpper(strings.TrimSpace(c))
		}
		if r, ok := syncCfg.ChannelExchangeRates[ch.Id]; ok && r > 0 {
			rawRate = r
		}
	}
	if ch != nil {
		if currency == "" && ch.CurrencyCode != nil {
			if c := strings.TrimSpace(*ch.CurrencyCode); c != "" && !strings.EqualFold(c, "AUTO") {
				currency = strings.ToUpper(c)
			}
		}
		if rawRate <= 0 && ch.ExchangeRate != nil && *ch.ExchangeRate > 0 {
			rawRate = *ch.ExchangeRate
		}
	}

	for _, e := range existingEntries {
		if !e.FromUpstream {
			continue
		}
		if e.BaseMultiplier > 0 && baseMult == 1.0 {
			baseMult = e.BaseMultiplier
		}
		if currency == "" && strings.TrimSpace(e.Currency) != "" && !strings.EqualFold(strings.TrimSpace(e.Currency), "AUTO") {
			currency = strings.ToUpper(strings.TrimSpace(e.Currency))
		}
		if rawRate <= 0 && e.ExchangeRate > 0 {
			rawRate = e.ExchangeRate
		}
		break
	}

	if ch != nil {
		if baseMult == 1.0 {
			if inferred, ok := InferChannelTrailingMultiplier(ch.Name); ok {
				baseMult = inferred
			}
		}
		if currency == "" {
			upID, _ := ResolveChannelUpstreamSource(ch, nil, syncCfg)
			if upID != "" && upID != DirectUpstreamSourceID {
				for _, acc := range syncCfg.UpstreamAccounts {
					if UpstreamAccountKey(acc) == upID {
						accCurr := strings.ToUpper(strings.TrimSpace(acc.Currency))
						if accCurr != "" && accCurr != "AUTO" {
							currency = accCurr
						}
						if rawRate <= 0 && acc.ExchangeRate > 0 {
							rawRate = acc.ExchangeRate
						}
						break
					}
				}
			}
		}
		if currency == "" {
			defCurr := strings.ToUpper(strings.TrimSpace(syncCfg.DefaultCurrency))
			if defCurr != "" && defCurr != "AUTO" {
				currency = defCurr
			} else if ch.Type == constant.ChannelTypeNewAPI {
				currency = "CNY"
			} else {
				currency = "USD"
			}
		}
	}
	if currency == "" {
		currency = "USD"
	}
	effRate = ResolveEffectiveChannelExchangeRate(currency, rawRate)
	return currency, effRate, baseMult
}

func computeDefaultModelCostEntry(ch *Channel, modelName string, now int64, existingEntries ...map[string]ChannelModelCostEntry) ChannelModelCostEntry {
	var chEntries map[string]ChannelModelCostEntry
	if len(existingEntries) > 0 {
		chEntries = existingEntries[0]
	}
	syncCfg := GetModelPriceSyncConfig()
	curr, effRate, baseMult := ResolveChannelCurrencyAndMultiplier(ch, chEntries, syncCfg)

	// If the channel maps this model to an upstream target model (e.g. gemini-4-argon -> gemini-2.5-pro)
	// and we already have a synced upstream cost entry for that target model on this channel, inherit it!
	if mappedTarget := resolveChannelMappedModelTarget(ch, modelName); mappedTarget != "" && chEntries != nil {
		if targetEntry, ok := chEntries[mappedTarget]; ok && targetEntry.FromUpstream && (targetEntry.InputCost > 0 || targetEntry.RawInputCost > 0) {
			cloned := targetEntry
			cloned.Model = modelName
			cloned.UpdatedAt = now
			return cloned
		}
	}

	lookupModel := modelName
	if mappedTarget := resolveChannelMappedModelTarget(ch, modelName); mappedTarget != "" {
		if _, ok := ratio_setting.GetModelPrice(modelName, false); !ok {
			if mr, hasR, _ := ratio_setting.GetModelRatio(modelName); !hasR || isDefaultUnconfiguredModelRatio(modelName, 0, mr, ratio_setting.GetCompletionRatio(modelName)) {
				lookupModel = mappedTarget
			}
		}
	}

	entry := ChannelModelCostEntry{
		ChannelID:      ch.Id,
		ChannelName:    ch.Name,
		ChannelType:    ch.Type,
		Model:          modelName,
		BaseMultiplier: baseMult,
		Currency:       curr,
		ExchangeRate:   effRate,
		FromUpstream:   false,
		UpdatedAt:      now,
	}

	// For local fallback models (not in upstream /api/pricing, such as custom aliases like gemini-4-argon),
	// ratio_setting's 1x base price is already in local quota units, so we scale only by baseMult (e.g. 0.5x or 0.18x)
	// so the channel cost reflects the channel's upstream discount rather than 100% of the website's retail price.
	costScale := 1.0
	if baseMult > 0 && baseMult != 1.0 {
		costScale = baseMult
	}
	if ch != nil && ch.Type == constant.ChannelTypeNewAPI {
		unitMult := ResolveUpstreamToQuotaUnitMultiplier(curr, effRate)
		if unitMult > 0 {
			costScale = roundCost(costScale * unitMult)
		}
	}

	if price, ok := ratio_setting.GetModelPrice(lookupModel, false); ok && price > 0 {
		entry.QuotaType = 1
		entry.ModelPrice = price
		entry.RawModelPrice = roundCost(price * baseMult)
		entry.RawInputCost = entry.RawModelPrice
		entry.RawOutputCost = entry.RawModelPrice
		entry.InputCost = roundCost(price * costScale)
		entry.OutputCost = entry.InputCost
		return entry
	}
	if billing_setting.GetBillingMode(lookupModel) == billing_setting.BillingModeTieredExpr {
		if expr, hasExpr := billing_setting.GetBillingExpr(lookupModel); hasExpr && strings.TrimSpace(expr) != "" {
			if mr, cr, cacheR, createCacheR, ok := deriveLocalExprBasePricing(expr); ok {
				entry.QuotaType = 0
				entry.ModelRatio = mr
				entry.CompletionRatio = cr
				entry.CacheRatio = cacheR
				entry.CreateCacheRatio = createCacheR
				baseIn := roundCost(mr * 2.0)
				entry.RawInputCost = roundCost(baseIn * baseMult)
				entry.RawOutputCost = roundCost(entry.RawInputCost * cr)
				entry.InputCost = roundCost(baseIn * costScale)
				entry.OutputCost = roundCost(entry.InputCost * cr)
				return entry
			}
		}
	}
	modelRatio, _, _ := ratio_setting.GetModelRatio(lookupModel)
	compRatio := ratio_setting.GetCompletionRatio(lookupModel)
	if compRatio <= 0 {
		compRatio = 1.0
	}
	entry.QuotaType = 0
	entry.ModelRatio = modelRatio
	entry.CompletionRatio = compRatio
	if cacheR, ok := ratio_setting.GetCacheRatio(lookupModel); ok && cacheR > 0 {
		entry.CacheRatio = roundCost(cacheR)
	}
	if createCacheR, ok := ratio_setting.GetCreateCacheRatio(lookupModel); ok && createCacheR > 0 {
		entry.CreateCacheRatio = roundCost(createCacheR)
	}
	baseIn := roundCost(modelRatio * 2.0)
	entry.RawInputCost = roundCost(baseIn * baseMult)
	entry.RawOutputCost = roundCost(entry.RawInputCost * compRatio)
	entry.InputCost = roundCost(baseIn * costScale)
	entry.OutputCost = roundCost(entry.InputCost * compRatio)
	return entry
}

func roundCost(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return math.Round(v*1e6) / 1e6
}

// ResolveUpstreamToQuotaUnitMultiplier returns how many system USD quota units
// (1 unit = QuotaPerUnit = 1 USD) equal 1 unit of the upstream currency.
//
// All exchange rates in the unified currency system are expressed as
// "units of currency per 1 USD" (e.g. USD=1, CNY=7.3, TWD=32, JPY=150),
// sourced from operation_setting.GetEffectiveCurrencyRates() with optional
// per-channel overrides.
func ResolveUpstreamToQuotaUnitMultiplier(currency string, exchangeRate float64) float64 {
	ratePerUSD := ResolveEffectiveChannelExchangeRate(currency, exchangeRate)
	if ratePerUSD <= 0 {
		return 1.0
	}
	return 1.0 / ratePerUSD
}

// ResolveEffectiveChannelExchangeRate returns the unified exchange rate
// (units of currency per 1 USD) for a channel/upstream currency.
// Legacy TWD-based defaults (USD=35, CNY=5, TWD=1) automatically fall back
// to the site's unified multi-currency rate table.
func ResolveEffectiveChannelExchangeRate(currency string, exchangeRate float64) float64 {
	curr := strings.ToUpper(strings.TrimSpace(currency))
	if curr == "" || curr == "AUTO" || curr == "USD" {
		return 1.0
	}
	if curr == "RMB" {
		curr = "CNY"
	}
	if curr == "NTD" {
		curr = "TWD"
	}

	rates := operation_setting.GetEffectiveCurrencyRates()
	defaultRate := 1.0
	if entry, ok := rates[curr]; ok && entry.Rate > 0 {
		defaultRate = entry.Rate
	} else if curr == "CNY" {
		defaultRate = 7.3
	} else if curr == "TWD" {
		defaultRate = 32.0
	}

	switch curr {
	case "CNY":
		// Ignore legacy TWD-per-CNY default (5.0) so existing configs seamlessly use USD-based CNY rate.
		if exchangeRate > 0 && math.Abs(exchangeRate-5.0) > 1e-4 {
			return exchangeRate
		}
		return defaultRate
	case "TWD":
		// Ignore legacy TWD-per-TWD default (1.0) so existing configs seamlessly use USD-based TWD rate.
		if exchangeRate > 1.01 {
			return exchangeRate
		}
		return defaultRate
	default:
		if exchangeRate > 0 {
			return exchangeRate
		}
		return defaultRate
	}
}

// RecalculateAllChannelModelCosts pre-calculates and caches per-channel, per-model costs.
// Official API channels default to 1x multiplier, while New API channels with synced upstream
// pricing retain their upstream ratios and multipliers.
func RecalculateAllChannelModelCosts(persist bool) {
	if DB == nil {
		return
	}
	var channels []*Channel
	if err := DB.Omit("key").Find(&channels).Error; err != nil {
		return
	}

	now := time.Now().Unix()
	channelModelCostMu.Lock()
	prevMap := channelModelCostMap
	nextMap := make(map[int]map[string]ChannelModelCostEntry, len(channels))

	for _, ch := range channels {
		if ch == nil || ch.Id <= 0 {
			continue
		}
		models := ch.GetModels()
		chMap := make(map[string]ChannelModelCostEntry, len(models))
		oldChMap := prevMap[ch.Id]

		for _, m := range models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			if prev, exists := oldChMap[m]; exists && prev.FromUpstream {
				prev.ChannelName = ch.Name
				prev.ChannelType = ch.Type
				if prev.BaseMultiplier <= 0 {
					prev.BaseMultiplier = 1.0
				}
				if prev.ExchangeRate <= 0 {
					prev.ExchangeRate = 1.0
				}
				prev.ExchangeRate = ResolveEffectiveChannelExchangeRate(prev.Currency, prev.ExchangeRate)
				unitMult := ResolveUpstreamToQuotaUnitMultiplier(prev.Currency, prev.ExchangeRate)
				if prev.QuotaType == 1 && (prev.ModelPrice > 0 || prev.RawModelPrice > 0) {
					rawP := prev.RawModelPrice
					if rawP <= 0 {
						rawP = prev.ModelPrice
					}
					prev.RawModelPrice = rawP
					prev.ModelPrice = rawP
					prev.RawInputCost = roundCost(rawP * prev.BaseMultiplier)
					prev.RawOutputCost = prev.RawInputCost
					prev.InputCost = roundCost(prev.RawInputCost * unitMult)
					prev.OutputCost = prev.InputCost
				} else {
					comp := prev.CompletionRatio
					if comp <= 0 {
						comp = 1.0
					}
					rawIn := prev.RawInputCost
					if rawIn <= 0 {
						rawIn = roundCost(prev.ModelRatio * 2.0 * prev.BaseMultiplier)
					} else if prev.BaseMultiplier > 0 {
						prev.ModelRatio = roundCost(rawIn / (2.0 * prev.BaseMultiplier))
					}
					prev.RawInputCost = rawIn
					prev.RawOutputCost = roundCost(rawIn * comp)
					prev.InputCost = roundCost(rawIn * unitMult)
					prev.OutputCost = roundCost(prev.InputCost * comp)
				}
				chMap[m] = prev
				continue
			}
			chMap[m] = computeDefaultModelCostEntry(ch, m, now, oldChMap)
		}
		nextMap[ch.Id] = chMap
	}
	channelModelCostMap = nextMap
	snapshot := cloneChannelModelCostMap(nextMap)
	channelModelCostMu.Unlock()

	if persist {
		if encoded, err := common.Marshal(snapshot); err == nil {
			_ = UpdateOption(ChannelModelCostsOptionKey, string(encoded))
		}
	}
}

// SetChannelUpstreamModelCosts updates the pre-calculated costs for a channel from upstream pricing data.
func SetChannelUpstreamModelCosts(channelID int, entries map[string]ChannelModelCostEntry, persist bool) {
	channelModelCostMu.Lock()
	if channelModelCostMap == nil {
		channelModelCostMap = make(map[int]map[string]ChannelModelCostEntry)
	}
	channelModelCostMap[channelID] = make(map[string]ChannelModelCostEntry, len(entries))
	for modelName, entry := range entries {
		channelModelCostMap[channelID][modelName] = entry
	}
	snapshot := cloneChannelModelCostMap(channelModelCostMap)
	channelModelCostMu.Unlock()

	if persist && DB != nil {
		if encoded, err := common.Marshal(snapshot); err == nil {
			_ = UpdateOption(ChannelModelCostsOptionKey, string(encoded))
		}
	}
}

func GetChannelModelCost(channelID int, modelName string) (ChannelModelCostEntry, bool) {
	channelModelCostMu.RLock()
	defer channelModelCostMu.RUnlock()
	if chMap, ok := channelModelCostMap[channelID]; ok {
		if entry, exists := chMap[modelName]; exists {
			return entry, true
		}
		normalized := ratio_setting.RoutingMatchModelName(modelName)
		if normalized != "" && normalized != modelName {
			if entry, exists := chMap[normalized]; exists {
				return entry, true
			}
		}
	}
	return ChannelModelCostEntry{}, false
}

func GetAllChannelModelCosts() map[int]map[string]ChannelModelCostEntry {
	channelModelCostMu.RLock()
	defer channelModelCostMu.RUnlock()
	return cloneChannelModelCostMap(channelModelCostMap)
}

// cloneChannelModelCostMap copies both map levels so the copy can be read or
// marshaled after the lock is released while writers keep updating the
// shared per-channel maps.
func cloneChannelModelCostMap(src map[int]map[string]ChannelModelCostEntry) map[int]map[string]ChannelModelCostEntry {
	out := make(map[int]map[string]ChannelModelCostEntry, len(src))
	for chID, mMap := range src {
		out[chID] = maps.Clone(mMap)
	}
	return out
}

// RecordChannelModelTelemetry records latency, TTFT, TPS, and success rate for a (channel, model) pair.
func RecordChannelModelTelemetry(channelID int, modelName string, success bool, ttftMs int64, hasTtft bool, outputTokens int64, generationMs int64) {
	if channelID <= 0 || strings.TrimSpace(modelName) == "" {
		return
	}
	key := channelModelKey{channelID: channelID, model: strings.TrimSpace(modelName)}
	actual, _ := channelTelemetryMap.LoadOrStore(key, &channelModelTelemetryState{
		emaSuccessRate: 1.0,
	})
	state := actual.(*channelModelTelemetryState)

	state.mu.Lock()
	defer state.mu.Unlock()

	state.requestCount++
	successVal := 0.0
	if success {
		state.successCount++
		successVal = 1.0
	}
	// The EMA starts from a healthy prior of 1.0 so a single failure cannot
	// drop a new channel straight to zero.
	now := time.Now()
	state.emaSuccessRate = recoverChannelSuccessRate(state.emaSuccessRate, state.lastSampleAt, now)
	state.emaSuccessRate = 0.8*state.emaSuccessRate + 0.2*successVal
	state.lastSampleAt = now

	if hasTtft && ttftMs >= 0 {
		state.ttftSumMs += ttftMs
		state.ttftCount++
		if state.emaTtftMs <= 0 {
			state.emaTtftMs = float64(ttftMs)
		} else {
			state.emaTtftMs = 0.75*state.emaTtftMs + 0.25*float64(ttftMs)
		}
	}

	if success && outputTokens > 0 && generationMs > 0 {
		tps := float64(outputTokens) * 1000.0 / float64(generationMs)
		if tps > 0 && !math.IsNaN(tps) && !math.IsInf(tps, 0) {
			if state.tpsCount == 0 || tps < state.tpsMin {
				state.tpsMin = tps
			}
			if state.tpsCount == 0 || tps > state.tpsMax {
				state.tpsMax = tps
			}
			state.tpsSum += tps
			state.tpsCount++
			if state.emaTps <= 0 {
				state.emaTps = tps
			} else {
				state.emaTps = 0.75*state.emaTps + 0.25*tps
			}
		}
	}
}

func GetChannelModelTelemetry(channelID int, modelName string) ChannelModelTelemetrySnapshot {
	key := channelModelKey{channelID: channelID, model: strings.TrimSpace(modelName)}
	val, ok := channelTelemetryMap.Load(key)
	if !ok {
		normalized := ratio_setting.RoutingMatchModelName(modelName)
		if normalized != "" && normalized != modelName {
			val, ok = channelTelemetryMap.Load(channelModelKey{channelID: channelID, model: normalized})
		}
	}
	if !ok {
		return ChannelModelTelemetrySnapshot{
			ChannelID:      channelID,
			Model:          modelName,
			Availability:   100.0,
			EmaSuccessRate: 1.0,
		}
	}
	state := val.(*channelModelTelemetryState)
	state.mu.Lock()
	defer state.mu.Unlock()

	avail := 100.0
	if state.requestCount > 0 {
		avail = math.Round((float64(state.successCount)/float64(state.requestCount))*10000.0) / 100.0
	}
	tpsAvg := 0.0
	if state.tpsCount > 0 {
		tpsAvg = math.Round((state.tpsSum/float64(state.tpsCount))*100.0) / 100.0
	}
	avgTtftSec := 0.0
	if state.ttftCount > 0 {
		avgTtftSec = math.Round((float64(state.ttftSumMs)/float64(state.ttftCount)/1000.0)*100.0) / 100.0
	}
	return ChannelModelTelemetrySnapshot{
		ChannelID:      channelID,
		Model:          modelName,
		RequestCount:   state.requestCount,
		SuccessCount:   state.successCount,
		Availability:   avail,
		TpsMin:         math.Round(state.tpsMin*100.0) / 100.0,
		TpsAvg:         tpsAvg,
		TpsMax:         math.Round(state.tpsMax*100.0) / 100.0,
		AvgTtftSec:     avgTtftSec,
		EmaSuccessRate: recoverChannelSuccessRate(state.emaSuccessRate, state.lastSampleAt, time.Now()),
		EmaTps:         state.emaTps,
		EmaTtftMs:      state.emaTtftMs,
	}
}

func scoreChannelForSchedule(mode string, ch *Channel, modelName string) float64 {
	telemetry := GetChannelModelTelemetry(ch.Id, modelName)
	costEntry, hasCost := GetChannelModelCost(ch.Id, modelName)
	if !hasCost {
		costEntry = computeDefaultModelCostEntry(ch, modelName, 0)
	}

	availRatio := telemetry.EmaSuccessRate
	if telemetry.RequestCount == 0 {
		availRatio = 1.0
	}
	tps := telemetry.EmaTps
	if tps <= 0 {
		tps = telemetry.TpsAvg
	}
	if tps <= 0 {
		tps = 80.0 // neutral baseline before first sample
	}
	ttftSec := telemetry.AvgTtftSec
	if ttftSec <= 0 {
		ttftSec = 1.0 // neutral 1.0s baseline before first sample
	}

	// Effective composite cost (1x input + 3x output weighted per 1M tokens)
	compositeCost := costEntry.InputCost + 3.0*costEntry.OutputCost
	if compositeCost <= 0 {
		compositeCost = 0.001
	}

	priorityBonus := float64(ch.GetPriority()) * 1e-4

	switch mode {
	case SchedulingModeLowerCost:
		// Scale by health so a flaky cheap channel gradually yields to a
		// healthy one; severely degraded channels (< 50% EMA success) are
		// penalized heavily.
		healthPenalty := availRatio
		if availRatio < 0.5 {
			healthPenalty = 0.01 * availRatio
		}
		return (1000.0/compositeCost)*healthPenalty + availRatio*0.1 + priorityBonus

	case SchedulingModeSpeed:
		healthPenalty := 1.0
		if availRatio < 0.5 {
			healthPenalty = 0.05 * availRatio
		}
		speedScore := (tps / (1.0 + ttftSec*0.5))
		return speedScore*healthPenalty + availRatio*5.0 + priorityBonus

	case SchedulingModeAvailability:
		// The recovering EMA dominates; the lifetime success rate only
		// separates channels with a similar recent record, so an old outage
		// cannot keep a channel demoted forever.
		return (availRatio*1000.0 + telemetry.Availability*0.5) - ttftSec*0.5 + tps*0.01 + priorityBonus

	case SchedulingModeBBR:
		// BBR-inspired congestion & delivery score:
		// - Windowed max bandwidth (TpsMax / EmaTps) and min propagation delay (AvgTtftSec)
		// - Cubic availability factor to rapidly drain failing channels while allowing 5% BBR probe recovery
		btlBw := max(telemetry.TpsMax, tps)
		deliveryRatio := tps / max(btlBw, 1.0)
		availFactor := availRatio * availRatio * availRatio
		if availRatio < 0.5 && rand.Float64() < 0.05 {
			// BBR ProbeBW: 5% chance to probe a degraded channel to detect recovery
			availFactor = 0.5
		}
		bbrScore := availFactor * (1.0 + tps/50.0) * (0.5 + 0.5*deliveryRatio) / (1.0 + ttftSec*0.35)
		return bbrScore*100.0 + priorityBonus

	default:
		return float64(ch.GetPriority())
	}
}

// selectChannelByGroupSchedule picks a channel by the group's scheduling mode.
// excludedChannelIDs lists channels already tried by this request; they are
// skipped so a retry moves to the next best channel instead of re-ranking a
// list whose order has shifted after the failure was recorded. When every
// candidate has been tried, or the caller cannot provide the list, retry
// indexes the ranked candidates instead.
func selectChannelByGroupSchedule(group string, modelName string, retry int, candidates []*Channel, excludedChannelIDs []int) *Channel {
	if len(excludedChannelIDs) > 0 {
		untried := make([]*Channel, 0, len(candidates))
		for _, ch := range candidates {
			if ch != nil && !slices.Contains(excludedChannelIDs, ch.Id) {
				untried = append(untried, ch)
			}
		}
		if len(untried) > 0 {
			candidates = untried
			retry = 0
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	mode := GetGroupSchedulingMode(group)
	if mode == SchedulingModeCostControl {
		if selected := selectChannelByCostControl(group, modelName, retry, candidates); selected != nil {
			return selected
		}
	}
	type scoredCandidate struct {
		channel *Channel
		score   float64
	}
	scored := make([]scoredCandidate, 0, len(candidates))
	for _, ch := range candidates {
		if ch == nil {
			continue
		}
		scored = append(scored, scoredCandidate{
			channel: ch,
			score:   scoreChannelForSchedule(mode, ch, modelName),
		})
	}
	if len(scored) == 0 {
		return nil
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if retry <= 0 {
		topScore := scored[0].score
		var tied []*Channel
		for _, item := range scored {
			if math.Abs(item.score-topScore) <= 1e-5 {
				tied = append(tied, item.channel)
			} else {
				break
			}
		}
		if len(tied) == 1 {
			return tied[0]
		}
		totalWeight := 0
		for _, ch := range tied {
			w := ch.GetWeight()
			if w <= 0 {
				w = 100
			}
			totalWeight += w
		}
		pick := rand.Intn(max(totalWeight, 1))
		for _, ch := range tied {
			w := ch.GetWeight()
			if w <= 0 {
				w = 100
			}
			pick -= w
			if pick < 0 {
				return ch
			}
		}
		return tied[0]
	}

	return scored[retry%len(scored)].channel
}

type ModelChannelGroupLogStat struct {
	ModelName        string `json:"model_name" gorm:"column:model_name"`
	ChannelID        int    `json:"channel_id" gorm:"column:channel_id"`
	UserGroup        string `json:"user_group" gorm:"column:user_group"`
	CallCount        int64  `json:"call_count" gorm:"column:call_count"`
	PromptTokens     int64  `json:"prompt_tokens" gorm:"column:prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens" gorm:"column:completion_tokens"`
	CacheTokens      int64  `json:"cache_tokens" gorm:"column:cache_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens" gorm:"column:cache_write_tokens"`
	TotalQuota       int64  `json:"total_quota" gorm:"column:total_quota"`
}

type UserModelChannelLogStat struct {
	UserID           int    `json:"user_id" gorm:"column:user_id"`
	Username         string `json:"username" gorm:"column:username"`
	ModelName        string `json:"model_name" gorm:"column:model_name"`
	ChannelID        int    `json:"channel_id" gorm:"column:channel_id"`
	CallCount        int64  `json:"call_count" gorm:"column:call_count"`
	PromptTokens     int64  `json:"prompt_tokens" gorm:"column:prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens" gorm:"column:completion_tokens"`
	CacheTokens      int64  `json:"cache_tokens" gorm:"column:cache_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens" gorm:"column:cache_write_tokens"`
	TotalQuota       int64  `json:"total_quota" gorm:"column:total_quota"`
}

type UserTopupLogStat struct {
	UserID        int   `json:"user_id" gorm:"column:user_id"`
	TopupLogQuota int64 `json:"topup_log_quota" gorm:"column:topup_log_quota"`
}

type UserOnlineTopupStat struct {
	UserID       int     `json:"user_id" gorm:"column:user_id"`
	OnlineAmount int64   `json:"online_amount" gorm:"column:online_amount"`
	OnlineMoney  float64 `json:"online_money" gorm:"column:online_money"`
}

func resolvedLogGroupCol() string {
	if logGroupCol != "" {
		return logGroupCol
	}
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		return `"group"`
	}
	return "`group`"
}

var (
	logCacheTokensRe           = regexp.MustCompile(`"cache_tokens"\s*:\s*(\d+)`)
	logCacheWriteTokensRe      = regexp.MustCompile(`"cache_write_tokens"\s*:\s*(\d+)`)
	logCacheCreationTokensRe   = regexp.MustCompile(`"cache_creation_tokens"\s*:\s*(\d+)`)
	logCacheCreationTokens5mRe = regexp.MustCompile(`"cache_creation_tokens_5m"\s*:\s*(\d+)`)
	logCacheCreationTokens1hRe = regexp.MustCompile(`"cache_creation_tokens_1h"\s*:\s*(\d+)`)
)

func parseLogOtherCacheTokens(other string) (cacheRead int64, cacheWrite int64) {
	if !strings.Contains(other, "cache_") {
		return 0, 0
	}
	if m := logCacheTokensRe.FindStringSubmatch(other); len(m) == 2 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v > 0 {
			cacheRead = v
		}
	}
	var cw5m, cw1h int64
	if m := logCacheCreationTokens5mRe.FindStringSubmatch(other); len(m) == 2 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v > 0 {
			cw5m = v
		}
	}
	if m := logCacheCreationTokens1hRe.FindStringSubmatch(other); len(m) == 2 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v > 0 {
			cw1h = v
		}
	}
	if cw5m > 0 || cw1h > 0 {
		cacheWrite = cw5m + cw1h
	} else if m := logCacheWriteTokensRe.FindStringSubmatch(other); len(m) == 2 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v > 0 {
			cacheWrite = v
		}
	} else if m := logCacheCreationTokensRe.FindStringSubmatch(other); len(m) == 2 {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v > 0 {
			cacheWrite = v
		}
	}
	return cacheRead, cacheWrite
}

func UpstreamAccountKey(acc UpstreamAccountSource) string {
	if id := strings.TrimSpace(acc.ID); id != "" {
		return id
	}
	if name := strings.ToLower(strings.TrimSpace(acc.Name)); name != "" {
		return "name:" + name
	}
	return ""
}

func cleanUpstreamBaseURLForMatch(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "露米娅.com", "lumiya.vip")
	s = strings.ReplaceAll(s, "露米婭.com", "lumiya.vip")
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		s = "https://" + s
	}
	if parsed, err := url.Parse(s); err == nil && parsed.Host != "" {
		scheme := strings.ToLower(parsed.Scheme)
		host := strings.ToLower(parsed.Host)
		if host == "xn--1ys141f4ks.com" || host == "www.lumiya.vip" {
			host = "lumiya.vip"
		}
		if strings.Contains(host, "lumiya.vip") {
			scheme = "https"
		}
		return scheme + "://" + host
	}
	s = strings.ToLower(strings.TrimRight(s, "/"))
	for _, suffix := range []string{"/v1", "/api", "/console", "/pricing", "/panel", "/token", "/personal"} {
		if trimmed, ok := strings.CutSuffix(s, suffix); ok {
			s = trimmed
		}
	}
	return strings.TrimRight(s, "/")
}

// ResolveChannelUpstreamSource resolves which CRON UpstreamAccountSource (or "direct") owns a channel.
func ResolveChannelUpstreamSource(ch *Channel, policy *GroupCostControlPolicy, cfg ModelPriceSyncConfig) (string, string) {
	if ch == nil {
		return DirectUpstreamSourceID, "Direct"
	}

	if policy != nil {
		if rule, ok := policy.ChannelRules[ch.Id]; ok {
			override := strings.TrimSpace(rule.UpstreamID)
			if override != "" {
				if strings.EqualFold(override, DirectUpstreamSourceID) {
					return DirectUpstreamSourceID, "Direct"
				}
				for _, acc := range cfg.UpstreamAccounts {
					key := UpstreamAccountKey(acc)
					if key != "" && (key == override || strings.EqualFold(strings.TrimSpace(acc.Name), override)) {
						name := strings.TrimSpace(acc.Name)
						if name == "" {
							name = key
						}
						return key, name
					}
				}
			}
		}
	}

	if meta, ok := cfg.AutoProvisionedChannels[ch.Id]; ok {
		metaID := strings.TrimSpace(meta.SourceID)
		metaName := strings.TrimSpace(meta.SourceName)
		for _, acc := range cfg.UpstreamAccounts {
			key := UpstreamAccountKey(acc)
			if key == "" {
				continue
			}
			if (metaID != "" && key == metaID) || (metaName != "" && strings.EqualFold(strings.TrimSpace(acc.Name), metaName)) {
				name := strings.TrimSpace(acc.Name)
				if name == "" {
					name = key
				}
				return key, name
			}
		}
		if metaID != "" {
			if metaName == "" {
				metaName = metaID
			}
			return metaID, metaName
		}
	}

	chBase := cleanUpstreamBaseURLForMatch(ch.GetBaseURL())
	chTag := strings.TrimSpace(ch.GetTag())
	chNameLower := strings.ToLower(strings.TrimSpace(ch.Name))

	for _, acc := range cfg.UpstreamAccounts {
		key := UpstreamAccountKey(acc)
		if key == "" {
			continue
		}
		accName := strings.TrimSpace(acc.Name)
		if accName == "" {
			accName = key
		}
		accBase := cleanUpstreamBaseURLForMatch(acc.BaseURL)
		if chBase != "" && accBase != "" && chBase == accBase {
			return key, accName
		}
		expTag := strings.TrimSpace(acc.ChannelTag)
		if expTag == "" && strings.TrimSpace(acc.Name) != "" {
			expTag = "auto:" + strings.TrimSpace(acc.Name)
		}
		if chTag != "" && expTag != "" && strings.EqualFold(chTag, expTag) {
			return key, accName
		}
		accNameLower := strings.ToLower(strings.TrimSpace(acc.Name))
		if accNameLower != "" {
			if chNameLower == accNameLower || strings.HasPrefix(chNameLower, accNameLower+"-") || strings.HasPrefix(chNameLower, accNameLower+"_") {
				return key, accName
			}
		}
	}

	return DirectUpstreamSourceID, "Direct"
}

func isDefaultUnconfiguredModelRatio(modelName string, quotaType int, modelRatio float64, completionRatio float64) bool {
	if quotaType != 0 {
		return false
	}
	if math.Abs(modelRatio-37.5) > 1e-6 {
		return false
	}
	if completionRatio != 0 && math.Abs(completionRatio-1.0) > 1e-6 {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "gpt-4.5")
}

func resolveBaseModelPricesUSD(modelName string) (quotaType int, inPrice float64, outPrice float64, cacheRatio float64, createCacheRatio float64) {
	if price, ok := ratio_setting.GetModelPrice(modelName, false); ok && price > 0 {
		p := roundCost(price)
		return 1, p, p, 1.0, 1.0
	}
	if billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
		if expr, hasExpr := billing_setting.GetBillingExpr(modelName); hasExpr && strings.TrimSpace(expr) != "" {
			if mr, cr, cacheR, createCacheR, ok := deriveLocalExprBasePricing(expr); ok && mr > 0 {
				inP := roundCost(mr * 2.0)
				outP := roundCost(inP * max(cr, 1.0))
				return 0, inP, outP, cacheR, createCacheR
			}
		}
	}
	mr, hasRatio, _ := ratio_setting.GetModelRatio(modelName)
	cr := ratio_setting.GetCompletionRatio(modelName)
	if cr <= 0 {
		cr = 1.0
	}
	if !hasRatio || isDefaultUnconfiguredModelRatio(modelName, 0, mr, cr) {
		return 0, 0, 0, 0, 0
	}
	if r, ok := ratio_setting.GetCacheRatio(modelName); ok && r > 0 {
		cacheRatio = roundCost(r)
	}
	if w, ok := ratio_setting.GetCreateCacheRatio(modelName); ok && w > 0 {
		createCacheRatio = roundCost(w)
	}
	inP := roundCost(mr * 2.0)
	outP := roundCost(inP * cr)
	return 0, inP, outP, cacheRatio, createCacheRatio
}

func resolveChannelModelUnitPricesUSD(channelID int, modelName string, syncCfg ModelPriceSyncConfig) (quotaType int, inUSD float64, outUSD float64, cacheRatio float64, createCacheRatio float64) {
	if entry, hasEntry := GetChannelModelCost(channelID, modelName); hasEntry {
		curr := strings.ToUpper(strings.TrimSpace(entry.Currency))
		rate := entry.ExchangeRate
		if chCurr, ok := syncCfg.ChannelCurrencies[channelID]; ok && strings.TrimSpace(chCurr) != "" {
			curr = strings.ToUpper(strings.TrimSpace(chCurr))
		}
		if chRate, ok := syncCfg.ChannelExchangeRates[channelID]; ok && chRate > 0 {
			rate = chRate
		}
		if curr == "" {
			curr = "USD"
		}
		effRate := ResolveEffectiveChannelExchangeRate(curr, rate)
		unitMult := ResolveUpstreamToQuotaUnitMultiplier(curr, effRate)

		rawIn := entry.RawInputCost
		rawOut := entry.RawOutputCost
		if rawIn <= 0 {
			mult := entry.BaseMultiplier
			if mult <= 0 {
				mult = 1.0
			}
			if entry.QuotaType == 1 && entry.ModelPrice > 0 {
				rawIn = roundCost(entry.ModelPrice * mult)
				rawOut = rawIn
			} else if entry.ModelRatio > 0 && !isDefaultUnconfiguredModelRatio(modelName, entry.QuotaType, entry.ModelRatio, entry.CompletionRatio) {
				rawIn = roundCost(entry.ModelRatio * 2.0 * mult)
				rawOut = roundCost(rawIn * max(entry.CompletionRatio, 1.0))
			} else if entry.InputCost > 0 && !isDefaultUnconfiguredModelRatio(modelName, entry.QuotaType, entry.ModelRatio, entry.CompletionRatio) {
				rawIn = entry.InputCost
				rawOut = entry.OutputCost
			}
		}
		if entry.FromUpstream && rawIn > 0 {
			inUSD = roundCost(rawIn * unitMult)
			outUSD = roundCost(rawOut * unitMult)
		} else if !isDefaultUnconfiguredModelRatio(modelName, entry.QuotaType, entry.ModelRatio, entry.CompletionRatio) {
			inUSD = entry.InputCost
			outUSD = entry.OutputCost
			if entry.BaseMultiplier <= 0 || math.Abs(entry.BaseMultiplier-1.0) < 1e-6 {
				if inferred, ok := InferChannelTrailingMultiplier(entry.ChannelName); ok && inferred > 0 && inferred != 1.0 {
					inUSD = roundCost(inUSD * inferred)
					outUSD = roundCost(outUSD * inferred)
				}
			}
		}
		if inUSD > 0 || outUSD > 0 {
			return entry.QuotaType, inUSD, outUSD, entry.CacheRatio, entry.CreateCacheRatio
		}
	}
	qt, inP, outP, cacheR, createCacheR := resolveBaseModelPricesUSD(modelName)
	if channelID > 0 && (inP > 0 || outP > 0) {
		if ch := lookupChannelOutsideRouting(channelID); ch != nil {
			_, _, baseMult := ResolveChannelCurrencyAndMultiplier(ch, nil, syncCfg)
			if baseMult > 0 && baseMult != 1.0 {
				inP = roundCost(inP * baseMult)
				outP = roundCost(outP * baseMult)
			}
		}
	}
	return qt, inP, outP, cacheR, createCacheR
}

func CalculateChannelModelUsageCost(channelID int, modelName string, callCount int64, promptTokens int64, completionTokens int64, cacheReadTokens int64, cacheWriteTokens int64) float64 {
	if callCount <= 0 && promptTokens <= 0 && completionTokens <= 0 {
		return 0
	}
	syncCfg := GetModelPriceSyncConfig()
	quotaType, inUSD, outUSD, cacheRatio, createCacheRatio := resolveChannelModelUnitPricesUSD(channelID, modelName, syncCfg)
	if quotaType == 1 && inUSD > 0 {
		return roundCost(float64(callCount) * inUSD)
	}
	if inUSD <= 0 && outUSD <= 0 {
		return 0
	}
	cr := max(cacheReadTokens, 0)
	cw := max(cacheWriteTokens, 0)
	nonCachedPrompt := max(promptTokens-cr-cw, 0)
	inCost := (float64(nonCachedPrompt) / 1_000_000.0) * inUSD
	if cr > 0 {
		r := cacheRatio
		if r <= 0 {
			r = 1.0
		}
		inCost += (float64(cr) / 1_000_000.0) * inUSD * r
	}
	if cw > 0 {
		w := createCacheRatio
		if w <= 0 {
			w = 1.0
		}
		inCost += (float64(cw) / 1_000_000.0) * inUSD * w
	}
	outCost := (float64(max(completionTokens, 0)) / 1_000_000.0) * outUSD
	return roundCost(inCost + outCost)
}

func EstimateGroupModelUnitRevenueAndCost(group string, ch *Channel, modelName string) (sellComposite float64, costComposite float64) {
	groupRatio := 1.0
	if gr, ok := ratio_setting.GetGroupGroupRatio(group, group); ok && gr >= 0 {
		groupRatio = gr
	} else if gr := ratio_setting.GetGroupRatio(group); gr >= 0 {
		groupRatio = gr
	}

	baseQT, baseIn, baseOut, _, _ := resolveBaseModelPricesUSD(modelName)
	if baseQT == 1 && baseIn > 0 {
		sellComposite = baseIn * groupRatio
	} else if baseIn > 0 || baseOut > 0 {
		sellComposite = (baseIn + 3.0*baseOut) * groupRatio
	} else {
		sellComposite = max(groupRatio, 0.001)
	}

	chID := 0
	if ch != nil {
		chID = ch.Id
	}
	syncCfg := GetModelPriceSyncConfig()
	chQT, chIn, chOut, _, _ := resolveChannelModelUnitPricesUSD(chID, modelName, syncCfg)
	if chQT == 1 && chIn > 0 {
		costComposite = chIn
	} else if chIn > 0 || chOut > 0 {
		costComposite = chIn + 3.0*chOut
	} else if ch != nil {
		fallback := computeDefaultModelCostEntry(ch, modelName, 0)
		if fallback.QuotaType == 1 && fallback.InputCost > 0 {
			costComposite = fallback.InputCost
		} else {
			costComposite = fallback.InputCost + 3.0*fallback.OutputCost
		}
	}
	if costComposite <= 0 {
		costComposite = 0.0001
	}
	return sellComposite, costComposite
}

func ResolveCostControlWindowStart(window string, now time.Time) int64 {
	switch NormalizeCostControlWindow(window) {
	case CostControlWindowToday:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	case CostControlWindow7d:
		return now.Add(-7 * 24 * time.Hour).Unix()
	case CostControlWindow30d:
		return now.Add(-30 * 24 * time.Hour).Unix()
	default:
		return now.Add(-24 * time.Hour).Unix()
	}
}

func computeAllowedCostAndDivertProb(policy GroupCostControlPolicy, revenueUSD float64, costUSD float64) (allowedCost float64, cushion float64, isOverCost bool, divertProb float64) {
	ratioBound := revenueUSD * (policy.MaxCostRatioPct / 100.0)
	if policy.MinNetProfitUSD < 0 {
		allowedCost = ratioBound - policy.MinNetProfitUSD
	} else if policy.MinNetProfitUSD > 0 {
		allowedCost = min(ratioBound, max(0.0, revenueUSD-policy.MinNetProfitUSD))
	} else {
		allowedCost = ratioBound
	}
	cushion = allowedCost - costUSD
	isOverCost = costUSD > 1e-9 && costUSD > allowedCost+1e-9
	if isOverCost {
		deficit := costUSD - allowedCost
		scale := max(allowedCost, revenueUSD, 0.05)
		sens := policy.OverflowSensitivity
		if sens <= 0 {
			sens = 1.0
		}
		rawSeverity := (deficit / scale) * sens
		divertProb = min(1.0, 0.35+0.65*(rawSeverity/(0.35+rawSeverity)))
	}
	return allowedCost, cushion, isOverCost, divertProb
}

func roundProfit6(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1e6) / 1e6
}

func roundProfitPct2(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

func ResetRealtimeProfitTracker() {
	groupRealtimeProfitMu.Lock()
	groupRealtimeProfitStates = make(map[string]*groupRealtimeProfitState)
	groupRealtimeProfitMu.Unlock()
}

// lookupChannelOutsideRouting resolves a channel by ID when NOT holding channelSyncLock.
func lookupChannelOutsideRouting(channelID int) *Channel {
	if channelID <= 0 {
		return nil
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		ch := channelsIDM[channelID]
		channelSyncLock.RUnlock()
		if ch != nil {
			return ch
		}
	}
	if DB != nil {
		var ch Channel
		if err := DB.Omit("key").Where("id = ?", channelID).First(&ch).Error; err == nil && ch.Id > 0 {
			return &ch
		}
	}
	return nil
}

func AggregateGroupCostControlUsage(group string, startTs int64, endTs int64) ([]ModelChannelGroupLogStat, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, nil
	}
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, nil
	}
	grpCol := resolvedLogGroupCol()
	tx := logDB.Model(&Log{}).
		Select("model_name, channel_id, "+grpCol+" as user_group, COUNT(*) as call_count, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(quota), 0) as total_quota").
		Where("type = ? AND "+grpCol+" = ?", LogTypeConsume, group)
	if startTs > 0 {
		tx = tx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		tx = tx.Where("created_at <= ?", endTs)
	}
	var stats []ModelChannelGroupLogStat
	if err := tx.Group("model_name, channel_id, " + grpCol).Scan(&stats).Error; err != nil {
		return nil, err
	}

	type rawCacheLog struct {
		ModelName string `gorm:"column:model_name"`
		ChannelID int    `gorm:"column:channel_id"`
		Other     string `gorm:"column:other"`
	}
	var cacheRows []rawCacheLog
	cacheTx := logDB.Model(&Log{}).
		Select("model_name, channel_id, other").
		Where("type = ? AND "+grpCol+" = ? AND other LIKE ?", LogTypeConsume, group, "%cache_%")
	if startTs > 0 {
		cacheTx = cacheTx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		cacheTx = cacheTx.Where("created_at <= ?", endTs)
	}
	if err := cacheTx.Scan(&cacheRows).Error; err == nil && len(cacheRows) > 0 {
		type mcKey struct {
			model   string
			channel int
		}
		readMap := make(map[mcKey]int64)
		writeMap := make(map[mcKey]int64)
		for _, r := range cacheRows {
			cr, cw := parseLogOtherCacheTokens(r.Other)
			if cr > 0 || cw > 0 {
				k := mcKey{model: r.ModelName, channel: r.ChannelID}
				readMap[k] += cr
				writeMap[k] += cw
			}
		}
		for i := range stats {
			k := mcKey{model: stats[i].ModelName, channel: stats[i].ChannelID}
			stats[i].CacheTokens = readMap[k]
			stats[i].CacheWriteTokens = writeMap[k]
		}
	}
	return stats, nil
}

func hydrateGroupRealtimeProfitFromDB(group string) {
	group = strings.TrimSpace(group)
	if group == "" {
		return
	}
	policy := GetGroupCostControlPolicy(group)
	syncCfg := GetModelPriceSyncConfig()
	now := time.Now()
	windowStart := ResolveCostControlWindowStart(policy.Window, now)
	logVersion := GetLogWriteVersion()

	chMap := make(map[int]*Channel)
	if DB != nil {
		var channels []*Channel
		if err := DB.Omit("key").Find(&channels).Error; err == nil {
			for _, ch := range channels {
				if ch != nil && ch.Id > 0 {
					chMap[ch.Id] = ch
				}
			}
		}
	} else if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		for id, ch := range channelsIDM {
			chMap[id] = ch
		}
		channelSyncLock.RUnlock()
	}

	stats, err := AggregateGroupCostControlUsage(group, windowStart, 0)
	if err != nil {
		groupRealtimeProfitMu.Lock()
		if st, ok := groupRealtimeProfitStates[group]; ok {
			st.refreshing = false
		}
		groupRealtimeProfitMu.Unlock()
		return
	}

	quotaUnit := common.QuotaPerUnit
	if quotaUnit <= 0 {
		quotaUnit = 500000
	}

	byUpstream := make(map[string]RealtimeProfitCounter)
	byChannel := make(map[int]RealtimeProfitCounter)
	var totalCalls int64
	var totalRev, totalCost float64

	for _, st := range stats {
		rev := float64(st.TotalQuota) / quotaUnit
		cost := CalculateChannelModelUsageCost(st.ChannelID, st.ModelName, st.CallCount, st.PromptTokens, st.CompletionTokens, st.CacheTokens, st.CacheWriteTokens)

		totalCalls += st.CallCount
		totalRev += rev
		totalCost += cost

		ch := chMap[st.ChannelID]
		if ch == nil && st.ChannelID > 0 {
			ch = &Channel{Id: st.ChannelID}
		}
		upID, _ := ResolveChannelUpstreamSource(ch, &policy, syncCfg)

		upCnt := byUpstream[upID]
		upCnt.Calls += st.CallCount
		upCnt.RevenueUSD = roundProfit6(upCnt.RevenueUSD + rev)
		upCnt.CostUSD = roundProfit6(upCnt.CostUSD + cost)
		upCnt.NetProfitUSD = roundProfit6(upCnt.RevenueUSD - upCnt.CostUSD)
		byUpstream[upID] = upCnt

		if st.ChannelID > 0 {
			chCnt := byChannel[st.ChannelID]
			chCnt.Calls += st.CallCount
			chCnt.RevenueUSD = roundProfit6(chCnt.RevenueUSD + rev)
			chCnt.CostUSD = roundProfit6(chCnt.CostUSD + cost)
			chCnt.NetProfitUSD = roundProfit6(chCnt.RevenueUSD - chCnt.CostUSD)
			byChannel[st.ChannelID] = chCnt
		}
	}

	nowTs := now.Unix()
	groupRealtimeProfitMu.Lock()
	existing := groupRealtimeProfitStates[group]
	if existing != nil && existing.hasUnpersistedData && len(stats) == 0 {
		existing.refreshing = false
		existing.lastHydratedAt = nowTs
		groupRealtimeProfitMu.Unlock()
		return
	}
	groupRealtimeProfitStates[group] = &groupRealtimeProfitState{
		window:           policy.Window,
		windowStart:      windowStart,
		lastHydratedAt:   nowTs,
		updatedAt:        nowTs,
		syncedLogVersion: logVersion,
		hydrated:         true,
		refreshing:       false,
		totalCalls:       totalCalls,
		revenueUSD:       roundProfit6(totalRev),
		costUSD:          roundProfit6(totalCost),
		byUpstream:       byUpstream,
		byChannel:        byChannel,
	}
	groupRealtimeProfitMu.Unlock()
}

// RecordRealtimeConsumeProfit updates the in-memory group profit counters in O(1) on every settled request.
func RecordRealtimeConsumeProfit(group string, channelID int, modelName string, quota int, promptTokens int, completionTokens int, cacheReadTokens int64, cacheWriteTokens int64, persistedInLogDB bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return
	}
	policy := GetGroupCostControlPolicy(group)
	syncCfg := GetModelPriceSyncConfig()

	ch := lookupChannelOutsideRouting(channelID)
	if ch == nil && channelID > 0 {
		ch = &Channel{Id: channelID}
	}
	upID, _ := ResolveChannelUpstreamSource(ch, &policy, syncCfg)

	quotaUnit := common.QuotaPerUnit
	if quotaUnit <= 0 {
		quotaUnit = 500000
	}
	rev := float64(quota) / quotaUnit
	cost := CalculateChannelModelUsageCost(channelID, modelName, 1, int64(promptTokens), int64(completionTokens), cacheReadTokens, cacheWriteTokens)
	now := time.Now()
	nowTs := now.Unix()
	logVersion := GetLogWriteVersion()

	groupRealtimeProfitMu.Lock()
	st, exists := groupRealtimeProfitStates[group]
	if (!exists || !st.hydrated || st.window != policy.Window) && persistedInLogDB && (LOG_DB != nil || DB != nil) {
		groupRealtimeProfitMu.Unlock()
		hydrateGroupRealtimeProfitFromDB(group)
		return
	}
	if !exists || st.window != policy.Window {
		st = &groupRealtimeProfitState{
			window:         policy.Window,
			windowStart:    ResolveCostControlWindowStart(policy.Window, now),
			lastHydratedAt: nowTs,
			hydrated:       true,
			byUpstream:     make(map[string]RealtimeProfitCounter),
			byChannel:      make(map[int]RealtimeProfitCounter),
		}
		groupRealtimeProfitStates[group] = st
	}
	if !persistedInLogDB {
		st.hasUnpersistedData = true
	}
	st.hydrated = true
	st.updatedAt = nowTs
	st.syncedLogVersion = logVersion
	st.totalCalls++
	st.revenueUSD = roundProfit6(st.revenueUSD + rev)
	st.costUSD = roundProfit6(st.costUSD + cost)

	upCnt := st.byUpstream[upID]
	upCnt.Calls++
	upCnt.RevenueUSD = roundProfit6(upCnt.RevenueUSD + rev)
	upCnt.CostUSD = roundProfit6(upCnt.CostUSD + cost)
	upCnt.NetProfitUSD = roundProfit6(upCnt.RevenueUSD - upCnt.CostUSD)
	st.byUpstream[upID] = upCnt

	if channelID > 0 {
		chCnt := st.byChannel[channelID]
		chCnt.Calls++
		chCnt.RevenueUSD = roundProfit6(chCnt.RevenueUSD + rev)
		chCnt.CostUSD = roundProfit6(chCnt.CostUSD + cost)
		chCnt.NetProfitUSD = roundProfit6(chCnt.RevenueUSD - chCnt.CostUSD)
		st.byChannel[channelID] = chCnt
	}
	groupRealtimeProfitMu.Unlock()
}

func buildProfitSnapshotFromState(group string, policy GroupCostControlPolicy, st *groupRealtimeProfitState, now time.Time) GroupRealtimeProfitSnapshot {
	snap := GroupRealtimeProfitSnapshot{
		Group:           group,
		Window:          policy.Window,
		WindowStart:     ResolveCostControlWindowStart(policy.Window, now),
		UpdatedAt:       now.Unix(),
		MaxCostRatioPct: policy.MaxCostRatioPct,
		MinNetProfitUSD: policy.MinNetProfitUSD,
		ByUpstream:      make(map[string]RealtimeProfitCounter),
		ByChannel:       make(map[int]RealtimeProfitCounter),
	}
	if st == nil {
		return snap
	}
	snap.WindowStart = st.windowStart
	snap.UpdatedAt = st.updatedAt
	snap.TotalCalls = st.totalCalls
	snap.RevenueUSD = roundProfit6(st.revenueUSD)
	snap.CostUSD = roundProfit6(st.costUSD)
	snap.NetProfitUSD = roundProfit6(st.revenueUSD - st.costUSD)
	if st.revenueUSD > 1e-9 {
		snap.NetMarginPct = roundProfitPct2(((st.revenueUSD - st.costUSD) / st.revenueUSD) * 100.0)
		snap.CostRatioPct = roundProfitPct2((st.costUSD / st.revenueUSD) * 100.0)
	} else if st.costUSD > 1e-9 {
		snap.NetMarginPct = -100.0
		snap.CostRatioPct = 100.0
	}
	allowed, cushion, overCost, divertProb := computeAllowedCostAndDivertProb(policy, st.revenueUSD, st.costUSD)
	snap.AllowedCostUSD = roundProfit6(allowed)
	snap.ProfitCushionUSD = roundProfit6(cushion)
	snap.IsOverCost = overCost
	snap.DivertProbPct = roundProfitPct2(divertProb * 100.0)
	snap.ByUpstream = maps.Clone(st.byUpstream)
	if snap.ByUpstream == nil {
		snap.ByUpstream = make(map[string]RealtimeProfitCounter)
	}
	snap.ByChannel = maps.Clone(st.byChannel)
	if snap.ByChannel == nil {
		snap.ByChannel = make(map[int]RealtimeProfitCounter)
	}
	return snap
}

// GetGroupRealtimeProfitSnapshot returns the live net-profit snapshot for a group.
// When forceSync is false (on the relay hot path), it never blocks on DB or channelSyncLock.
func GetGroupRealtimeProfitSnapshot(group string, forceSync bool) GroupRealtimeProfitSnapshot {
	group = strings.TrimSpace(group)
	policy := GetGroupCostControlPolicy(group)
	now := time.Now()
	nowTs := now.Unix()
	hasDB := LOG_DB != nil || DB != nil

	if forceSync && hasDB {
		groupRealtimeProfitMu.RLock()
		st := groupRealtimeProfitStates[group]
		needSync := st == nil || !st.hydrated || st.window != policy.Window || st.syncedLogVersion != GetLogWriteVersion() || nowTs-st.lastHydratedAt >= 15
		if st != nil && st.hasUnpersistedData && !common.LogConsumeEnabled {
			needSync = false
		}
		groupRealtimeProfitMu.RUnlock()
		if needSync {
			hydrateGroupRealtimeProfitFromDB(group)
		}
	}

	groupRealtimeProfitMu.RLock()
	st := groupRealtimeProfitStates[group]
	if st != nil && st.hydrated && st.window == policy.Window {
		snap := buildProfitSnapshotFromState(group, policy, st, now)
		needAsyncRefresh := hasDB && common.LogConsumeEnabled && !st.hasUnpersistedData && !st.refreshing && (nowTs-st.lastHydratedAt >= 60)
		groupRealtimeProfitMu.RUnlock()
		if needAsyncRefresh {
			groupRealtimeProfitMu.Lock()
			if cur := groupRealtimeProfitStates[group]; cur != nil && !cur.refreshing && nowTs-cur.lastHydratedAt >= 60 {
				cur.refreshing = true
				go hydrateGroupRealtimeProfitFromDB(group)
			}
			groupRealtimeProfitMu.Unlock()
		}
		return snap
	}
	groupRealtimeProfitMu.RUnlock()

	if hasDB {
		groupRealtimeProfitMu.Lock()
		cur := groupRealtimeProfitStates[group]
		if cur == nil || cur.window != policy.Window {
			cur = &groupRealtimeProfitState{
				window:      policy.Window,
				windowStart: ResolveCostControlWindowStart(policy.Window, now),
				updatedAt:   nowTs,
				refreshing:  true,
				byUpstream:  make(map[string]RealtimeProfitCounter),
				byChannel:   make(map[int]RealtimeProfitCounter),
			}
			groupRealtimeProfitStates[group] = cur
			go hydrateGroupRealtimeProfitFromDB(group)
		} else if !cur.refreshing && !cur.hydrated {
			cur.refreshing = true
			go hydrateGroupRealtimeProfitFromDB(group)
		}
		snap := buildProfitSnapshotFromState(group, policy, cur, now)
		groupRealtimeProfitMu.Unlock()
		return snap
	}

	return buildProfitSnapshotFromState(group, policy, nil, now)
}

type costControlCandidate struct {
	channel       *Channel
	upstreamID    string
	upAlloc       CostControlUpstreamAlloc
	effPriority   int
	effWeight     int
	availRatio    float64
	isHealthy     bool
	healthFactor  float64
	sellComposite float64
	costComposite float64
}

func sampleWeightedCostControlCandidate(items []costControlCandidate, weightFn func(costControlCandidate) float64) *Channel {
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return items[0].channel
	}
	total := 0.0
	weights := make([]float64, len(items))
	for i, item := range items {
		w := weightFn(item)
		if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
			w = 0
		}
		weights[i] = w
		total += w
	}
	if total <= 0 {
		return items[rand.Intn(len(items))].channel
	}
	pick := rand.Float64() * total
	for i, w := range weights {
		pick -= w
		if pick <= 0 {
			return items[i].channel
		}
	}
	return items[len(items)-1].channel
}

func pickCostControlPreferredChannel(all []costControlCandidate) *Channel {
	var pool []costControlCandidate
	for _, c := range all {
		if c.upAlloc.Enabled && c.upAlloc.Weight > 0 && c.effWeight > 0 && c.isHealthy {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		for _, c := range all {
			if c.upAlloc.Enabled && c.upAlloc.Weight > 0 && c.effWeight > 0 {
				pool = append(pool, c)
			}
		}
	}
	if len(pool) == 0 {
		for _, c := range all {
			if c.isHealthy {
				pool = append(pool, c)
			}
		}
	}
	if len(pool) == 0 {
		pool = all
	}
	if len(pool) == 1 {
		return pool[0].channel
	}

	type upstreamBucket struct {
		id        string
		weight    float64
		maxHealth float64
		items     []costControlCandidate
	}
	bucketIdx := make(map[string]int)
	var buckets []upstreamBucket
	for _, c := range pool {
		idx, exists := bucketIdx[c.upstreamID]
		if !exists {
			upW := float64(max(c.upAlloc.Weight, 1))
			if !c.upAlloc.Enabled {
				upW = 1.0
			}
			idx = len(buckets)
			bucketIdx[c.upstreamID] = idx
			buckets = append(buckets, upstreamBucket{
				id:        c.upstreamID,
				weight:    upW,
				maxHealth: c.healthFactor,
				items:     []costControlCandidate{c},
			})
		} else {
			buckets[idx].items = append(buckets[idx].items, c)
			if c.healthFactor > buckets[idx].maxHealth {
				buckets[idx].maxHealth = c.healthFactor
			}
		}
	}

	chosenBucket := buckets[0]
	if len(buckets) > 1 {
		totalUpWeight := 0.0
		for _, b := range buckets {
			totalUpWeight += b.weight * max(b.maxHealth, 0.05)
		}
		if totalUpWeight > 0 {
			pick := rand.Float64() * totalUpWeight
			for _, b := range buckets {
				pick -= b.weight * max(b.maxHealth, 0.05)
				if pick <= 0 {
					chosenBucket = b
					break
				}
			}
		} else {
			chosenBucket = buckets[rand.Intn(len(buckets))]
		}
	}

	topPriority := chosenBucket.items[0].effPriority
	for _, c := range chosenBucket.items[1:] {
		if c.effPriority > topPriority {
			topPriority = c.effPriority
		}
	}
	var tier []costControlCandidate
	for _, c := range chosenBucket.items {
		if c.effPriority == topPriority {
			tier = append(tier, c)
		}
	}
	return sampleWeightedCostControlCandidate(tier, func(c costControlCandidate) float64 {
		return float64(max(c.effWeight, 1)) * c.healthFactor
	})
}

func selectChannelByCostControl(group string, modelName string, retry int, candidates []*Channel) *Channel {
	policy := GetGroupCostControlPolicy(group)
	syncCfg := GetModelPriceSyncConfig()
	snap := GetGroupRealtimeProfitSnapshot(group, false)

	items := make([]costControlCandidate, 0, len(candidates))
	for _, ch := range candidates {
		if ch == nil {
			continue
		}
		upID, _ := ResolveChannelUpstreamSource(ch, &policy, syncCfg)
		upAlloc, hasUp := policy.UpstreamAllocations[upID]
		if !hasUp {
			upAlloc = CostControlUpstreamAlloc{Enabled: true, Weight: 100}
		}
		effPrio := int(ch.GetPriority())
		effWeight := ch.GetWeight()
		if effWeight <= 0 {
			effWeight = 100
		}
		if rule, hasRule := policy.ChannelRules[ch.Id]; hasRule {
			if rule.Priority != nil {
				effPrio = *rule.Priority
			}
			if rule.Weight != nil {
				effWeight = max(*rule.Weight, 0)
			}
		}

		tel := GetChannelModelTelemetry(ch.Id, modelName)
		avail := tel.EmaSuccessRate
		if tel.RequestCount == 0 {
			avail = 1.0
		}
		healthy := avail >= 0.5 || rand.Float64() < 0.05
		healthFactor := max(avail*avail, 0.05)
		if avail < 0.5 && !healthy {
			healthFactor = 0.01 * max(avail, 0.05)
		}

		sellComp, costComp := EstimateGroupModelUnitRevenueAndCost(group, ch, modelName)
		items = append(items, costControlCandidate{
			channel:       ch,
			upstreamID:    upID,
			upAlloc:       upAlloc,
			effPriority:   effPrio,
			effWeight:     effWeight,
			availRatio:    avail,
			isHealthy:     healthy,
			healthFactor:  healthFactor,
			sellComposite: sellComp,
			costComposite: costComp,
		})
	}
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return items[0].channel
	}

	if retry > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			if snap.IsOverCost {
				if math.Abs(items[i].costComposite-items[j].costComposite) > 1e-6 {
					return items[i].costComposite < items[j].costComposite
				}
			}
			iActive := items[i].upAlloc.Enabled && items[i].upAlloc.Weight > 0 && items[i].effWeight > 0
			jActive := items[j].upAlloc.Enabled && items[j].upAlloc.Weight > 0 && items[j].effWeight > 0
			if iActive != jActive {
				return iActive
			}
			if items[i].effPriority != items[j].effPriority {
				return items[i].effPriority > items[j].effPriority
			}
			return float64(items[i].upAlloc.Weight*items[i].effWeight)*items[i].healthFactor >
				float64(items[j].upAlloc.Weight*items[j].effWeight)*items[j].healthFactor
		})
		return items[retry%len(items)].channel
	}

	if !snap.IsOverCost {
		return pickCostControlPreferredChannel(items)
	}

	_, _, _, divertProb := computeAllowedCostAndDivertProb(policy, snap.RevenueUSD, snap.CostUSD)
	if rand.Float64() >= divertProb {
		return pickCostControlPreferredChannel(items)
	}

	// Build recovery pool: prefer healthy channels whose unit cost is within targetUnitCost (generates positive margin),
	// falling back to the lowest-cost healthy candidates when every candidate exceeds targetUnitCost.
	var healthyPool []costControlCandidate
	for _, c := range items {
		if c.isHealthy {
			healthyPool = append(healthyPool, c)
		}
	}
	if len(healthyPool) == 0 {
		healthyPool = items
	}

	minCost := healthyPool[0].costComposite
	maxCost := healthyPool[0].costComposite
	for _, c := range healthyPool[1:] {
		if c.costComposite < minCost {
			minCost = c.costComposite
		}
		if c.costComposite > maxCost {
			maxCost = c.costComposite
		}
	}

	var recoveryPool []costControlCandidate
	for _, c := range healthyPool {
		targetUnitCost := c.sellComposite * (policy.MaxCostRatioPct / 100.0)
		if c.costComposite <= targetUnitCost+1e-9 {
			recoveryPool = append(recoveryPool, c)
		}
	}
	if len(recoveryPool) == 0 {
		cutoff := minCost * 1.15
		for _, c := range healthyPool {
			if c.costComposite <= cutoff+1e-9 {
				recoveryPool = append(recoveryPool, c)
			}
		}
	}
	if len(recoveryPool) == 0 {
		recoveryPool = healthyPool
	}

	return sampleWeightedCostControlCandidate(recoveryPool, func(c costControlCandidate) float64 {
		targetUnitCost := c.sellComposite * (policy.MaxCostRatioPct / 100.0)
		marginGain := max(targetUnitCost-c.costComposite, 0.0) + max(maxCost-c.costComposite, 0.0) + 0.01*max(targetUnitCost, minCost)
		prefFactor := 0.25
		if c.upAlloc.Enabled && c.upAlloc.Weight > 0 && c.effWeight > 0 {
			prefFactor = 1.0 + math.Log1p(float64(c.upAlloc.Weight)/50.0)*math.Log1p(float64(c.effWeight)/50.0) + float64(max(c.effPriority, 0))*0.02
		}
		return (marginGain / max(c.costComposite, 1e-4)) * c.healthFactor * prefFactor
	})
}

func AggregateModelChannelGroupUsage(startTs, endTs int64) ([]ModelChannelGroupLogStat, error) {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, nil
	}
	grpCol := resolvedLogGroupCol()
	tx := logDB.Model(&Log{}).
		Select("model_name, channel_id, "+grpCol+" as user_group, COUNT(*) as call_count, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(quota), 0) as total_quota").
		Where("type = ?", LogTypeConsume)
	if startTs > 0 {
		tx = tx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		tx = tx.Where("created_at <= ?", endTs)
	}
	var stats []ModelChannelGroupLogStat
	if err := tx.Group("model_name, channel_id, " + grpCol).Scan(&stats).Error; err != nil {
		return nil, err
	}

	type rawCacheLog struct {
		ModelName string `gorm:"column:model_name"`
		ChannelID int    `gorm:"column:channel_id"`
		UserGroup string `gorm:"column:user_group"`
		Other     string `gorm:"column:other"`
	}
	var cacheRows []rawCacheLog
	cacheTx := logDB.Model(&Log{}).
		Select("model_name, channel_id, "+grpCol+" as user_group, other").
		Where("type = ? AND other LIKE ?", LogTypeConsume, "%cache_%")
	if startTs > 0 {
		cacheTx = cacheTx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		cacheTx = cacheTx.Where("created_at <= ?", endTs)
	}
	if err := cacheTx.Scan(&cacheRows).Error; err == nil && len(cacheRows) > 0 {
		type grpKey struct {
			model   string
			channel int
			group   string
		}
		readMap := make(map[grpKey]int64)
		writeMap := make(map[grpKey]int64)
		for _, r := range cacheRows {
			cr, cw := parseLogOtherCacheTokens(r.Other)
			if cr > 0 || cw > 0 {
				k := grpKey{model: r.ModelName, channel: r.ChannelID, group: r.UserGroup}
				readMap[k] += cr
				writeMap[k] += cw
			}
		}
		for i := range stats {
			k := grpKey{model: stats[i].ModelName, channel: stats[i].ChannelID, group: stats[i].UserGroup}
			stats[i].CacheTokens = readMap[k]
			stats[i].CacheWriteTokens = writeMap[k]
		}
	}

	return stats, nil
}

func AggregateUserCostUsage(startTs, endTs int64) ([]UserModelChannelLogStat, map[int]int64, map[int]UserOnlineTopupStat, error) {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, nil, nil, nil
	}

	tx := logDB.Model(&Log{}).
		Select("user_id, MAX(username) as username, model_name, channel_id, COUNT(*) as call_count, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(quota), 0) as total_quota").
		Where("type = ?", LogTypeConsume)
	if startTs > 0 {
		tx = tx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		tx = tx.Where("created_at <= ?", endTs)
	}
	var consumeStats []UserModelChannelLogStat
	if err := tx.Group("user_id, model_name, channel_id").Scan(&consumeStats).Error; err != nil {
		return nil, nil, nil, err
	}

	type rawUserCacheLog struct {
		UserID    int    `gorm:"column:user_id"`
		ModelName string `gorm:"column:model_name"`
		ChannelID int    `gorm:"column:channel_id"`
		Other     string `gorm:"column:other"`
	}
	var userCacheRows []rawUserCacheLog
	uCacheTx := logDB.Model(&Log{}).
		Select("user_id, model_name, channel_id, other").
		Where("type = ? AND other LIKE ?", LogTypeConsume, "%cache_%")
	if startTs > 0 {
		uCacheTx = uCacheTx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		uCacheTx = uCacheTx.Where("created_at <= ?", endTs)
	}
	if err := uCacheTx.Scan(&userCacheRows).Error; err == nil && len(userCacheRows) > 0 {
		type uKey struct {
			uid     int
			model   string
			channel int
		}
		readMap := make(map[uKey]int64)
		writeMap := make(map[uKey]int64)
		for _, r := range userCacheRows {
			cr, cw := parseLogOtherCacheTokens(r.Other)
			if cr > 0 || cw > 0 {
				k := uKey{uid: r.UserID, model: r.ModelName, channel: r.ChannelID}
				readMap[k] += cr
				writeMap[k] += cw
			}
		}
		for i := range consumeStats {
			k := uKey{uid: consumeStats[i].UserID, model: consumeStats[i].ModelName, channel: consumeStats[i].ChannelID}
			consumeStats[i].CacheTokens = readMap[k]
			consumeStats[i].CacheWriteTokens = writeMap[k]
		}
	}

	topupLogTx := logDB.Model(&Log{}).
		Select("user_id, COALESCE(SUM(quota), 0) as topup_log_quota").
		Where("type = ?", LogTypeTopup)
	if startTs > 0 {
		topupLogTx = topupLogTx.Where("created_at >= ?", startTs)
	}
	if endTs > 0 {
		topupLogTx = topupLogTx.Where("created_at <= ?", endTs)
	}
	var topupLogs []UserTopupLogStat
	_ = topupLogTx.Group("user_id").Scan(&topupLogs).Error
	topupLogMap := make(map[int]int64, len(topupLogs))
	for _, r := range topupLogs {
		topupLogMap[r.UserID] = r.TopupLogQuota
	}

	onlineMap := make(map[int]UserOnlineTopupStat)
	if DB != nil {
		onlineTx := DB.Model(&TopUp{}).
			Select("user_id, COALESCE(SUM(amount), 0) as online_amount, COALESCE(SUM(money), 0) as online_money").
			Where("status = ?", "success")
		if startTs > 0 {
			onlineTx = onlineTx.Where("complete_time >= ? OR (complete_time = 0 AND create_time >= ?)", startTs, startTs)
		}
		if endTs > 0 {
			onlineTx = onlineTx.Where("(complete_time > 0 AND complete_time <= ?) OR (complete_time = 0 AND create_time <= ?)", endTs, endTs)
		}
		var onlineRows []UserOnlineTopupStat
		if err := onlineTx.Group("user_id").Scan(&onlineRows).Error; err == nil {
			for _, r := range onlineRows {
				onlineMap[r.UserID] = r
			}
		}
	}

	return consumeStats, topupLogMap, onlineMap, nil
}

// IsChannelNativeAPI returns true if a channel represents a direct/native upstream provider API
// rather than an aggregated NewAPI relay or explicitly tagged official/native channel.
func IsChannelNativeAPI(ch *Channel, cfg ModelPriceSyncConfig) bool {
	if ch == nil {
		return false
	}
	if ch.Type != constant.ChannelTypeNewAPI {
		return true
	}
	searchText := strings.ToLower(strings.TrimSpace(ch.Name) + " " + strings.TrimSpace(ch.GetTag()))
	if strings.Contains(searchText, "原生") || strings.Contains(searchText, "直連") || strings.Contains(searchText, "直连") || strings.Contains(searchText, "native") {
		return true
	}
	return false
}

// MatchChannelClassifierRules evaluates enabled classifier rules against a channel's name, tag,
// upstream group, and native API status.
func MatchChannelClassifierRules(ch *Channel, cfg ModelPriceSyncConfig) (matchedGroups []string, upstreamGroup string, isNative bool) {
	if ch == nil {
		return nil, "", false
	}
	upGroup := strings.TrimSpace(cfg.ChannelUpstreamGroups[ch.Id])
	if upGroup == "" {
		if meta, ok := cfg.AutoProvisionedChannels[ch.Id]; ok {
			upGroup = strings.TrimSpace(meta.UpstreamGroup)
		}
	}
	isNative = IsChannelNativeAPI(ch, cfg)

	rules := cfg.ClassifierRules
	if rules == nil {
		rules = defaultChannelClassifierRules()
	} else {
		rules = normalizeChannelClassifierRules(rules)
	}

	corpus := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(ch.Name),
		strings.TrimSpace(ch.GetTag()),
		upGroup,
	}, " "))

	seen := make(map[string]struct{})
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		target := strings.TrimSpace(r.TargetGroup)
		if target == "" {
			continue
		}
		matched := false
		if r.IncludeNativeAPI && isNative {
			matched = true
		}
		if !matched {
			for _, kw := range r.Keywords {
				kwLower := strings.ToLower(strings.TrimSpace(kw))
				if kwLower != "" && strings.Contains(corpus, kwLower) {
					matched = true
					break
				}
			}
		}
		if matched {
			if _, exists := seen[target]; !exists {
				seen[target] = struct{}{}
				matchedGroups = append(matchedGroups, target)
			}
		}
	}
	return matchedGroups, upGroup, isNative
}

// SyncClassifierGroupsToSystemSettings ensures that all enabled classifier rules have their
// target groups, ratios, and descriptions registered in system GroupRatio and UserUsableGroups.
func SyncClassifierGroupsToSystemSettings(rules []ChannelClassifierRule) ([]string, error) {
	normalized := normalizeChannelClassifierRules(rules)
	groupRatios := ratio_setting.GetGroupRatioCopy()
	if groupRatios == nil {
		groupRatios = make(map[string]float64)
	}
	if _, ok := groupRatios["default"]; !ok {
		groupRatios["default"] = 1.0
	}

	usableGroups := setting.GetUserUsableGroupsCopy()
	if usableGroups == nil {
		usableGroups = make(map[string]string)
	}
	if _, ok := usableGroups["default"]; !ok {
		usableGroups["default"] = "Default group"
	}

	var syncedGroups []string
	seen := make(map[string]struct{})
	for _, r := range normalized {
		if !r.Enabled {
			continue
		}
		g := strings.TrimSpace(r.TargetGroup)
		if g == "" {
			continue
		}
		ratio := r.GroupRatio
		if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			ratio = 1.0
		}
		groupRatios[g] = ratio
		desc := strings.TrimSpace(r.Description)
		if desc == "" {
			desc = strings.TrimSpace(r.Name)
		}
		if desc == "" {
			desc = g
		}
		usableGroups[g] = desc
		if _, exists := seen[g]; !exists {
			seen[g] = struct{}{}
			syncedGroups = append(syncedGroups, g)
		}
	}

	if ratioBytes, err := common.Marshal(groupRatios); err == nil {
		ratioJSON := string(ratioBytes)
		_ = ratio_setting.UpdateGroupRatioByJSONString(ratioJSON)
		if DB != nil {
			common.OptionMapRWMutex.Lock()
			if common.OptionMap == nil {
				common.OptionMap = make(map[string]string)
			}
			common.OptionMapRWMutex.Unlock()
			_ = UpdateOption("GroupRatio", ratioJSON)
		}
	} else {
		return syncedGroups, err
	}

	if usableBytes, err := common.Marshal(usableGroups); err == nil {
		usableJSON := string(usableBytes)
		_ = setting.UpdateUserUsableGroupsByJSONString(usableJSON)
		if DB != nil {
			common.OptionMapRWMutex.Lock()
			if common.OptionMap == nil {
				common.OptionMap = make(map[string]string)
			}
			common.OptionMapRWMutex.Unlock()
			_ = UpdateOption("UserUsableGroups", usableJSON)
		}
	} else {
		return syncedGroups, err
	}

	return syncedGroups, nil
}

// ComputeClassifiedChannelGroupString returns the updated comma-separated group string for a channel
// by ensuring "default" is present, preserving unmanaged manual groups, and replacing classifier-managed
// groups with the newly matched classifier groups.
func ComputeClassifiedChannelGroupString(existingGroup string, matchedGroups []string, allRules []ChannelClassifierRule) string {
	managedSet := make(map[string]struct{}, len(allRules))
	for _, r := range allRules {
		g := strings.TrimSpace(r.TargetGroup)
		if g != "" && g != "default" {
			managedSet[g] = struct{}{}
		}
	}

	finalGroups := make([]string, 0, 4+len(matchedGroups))
	seen := make(map[string]struct{}, 4+len(matchedGroups))
	addGroup := func(g string) {
		g = strings.TrimSpace(g)
		if g == "" {
			return
		}
		if _, ok := seen[g]; !ok {
			seen[g] = struct{}{}
			finalGroups = append(finalGroups, g)
		}
	}

	// "default" is always included so default group can draw from all channels
	addGroup("default")

	for part := range strings.SplitSeq(existingGroup, ",") {
		g := strings.TrimSpace(part)
		if g == "" || g == "default" {
			continue
		}
		if _, isManaged := managedSet[g]; !isManaged {
			addGroup(g)
		}
	}

	for _, mg := range matchedGroups {
		addGroup(mg)
	}

	return strings.Join(finalGroups, ",")
}

// ApplyChannelClassifierToChannels evaluates the classifier rules across the specified channels
// (or all channels when channelIDs is empty), updates channel groups and abilities in DB,
// syncs classifier group ratios/descriptions, and refreshes the channel cache.
func ApplyChannelClassifierToChannels(channelIDs []int) (ChannelClassificationSummary, error) {
	cfg := GetModelPriceSyncConfig()
	rules := normalizeChannelClassifierRules(cfg.ClassifierRules)
	syncedGroups, err := SyncClassifierGroupsToSystemSettings(rules)
	if err != nil {
		return ChannelClassificationSummary{}, err
	}

	if DB == nil {
		return ChannelClassificationSummary{
			SyncedGroups: syncedGroups,
			Items:        []ChannelClassificationItemResult{},
		}, nil
	}

	var channels []*Channel
	query := DB.Model(&Channel{})
	if len(channelIDs) > 0 {
		query = query.Where("id IN ?", channelIDs)
	}
	if err := query.Order("id asc").Find(&channels).Error; err != nil {
		return ChannelClassificationSummary{}, err
	}

	items := make([]ChannelClassificationItemResult, 0, len(channels))
	updatedCount := 0

	err = DB.Transaction(func(tx *gorm.DB) error {
		for _, ch := range channels {
			if ch == nil {
				continue
			}
			matched, upGroup, isNative := MatchChannelClassifierRules(ch, cfg)
			nextGroupStr := ComputeClassifiedChannelGroupString(ch.Group, matched, rules)
			changed := nextGroupStr != strings.TrimSpace(ch.Group)
			if changed {
				ch.Group = nextGroupStr
				if err := tx.Model(&Channel{}).Where("id = ?", ch.Id).Update("group", ch.Group).Error; err != nil {
					return err
				}
				if err := ch.UpdateAbilities(tx); err != nil {
					return err
				}
				updatedCount++
			}
			if matched == nil {
				matched = []string{}
			}
			items = append(items, ChannelClassificationItemResult{
				ChannelID:     ch.Id,
				ChannelName:   ch.Name,
				ChannelType:   ch.Type,
				UpstreamGroup: upGroup,
				IsNativeAPI:   isNative,
				MatchedGroups: matched,
				FinalGroups:   nextGroupStr,
				Changed:       changed,
			})
		}
		return nil
	})
	if err != nil {
		return ChannelClassificationSummary{}, err
	}

	if updatedCount > 0 {
		InitChannelCache()
	}

	return ChannelClassificationSummary{
		TotalChannels:   len(channels),
		UpdatedChannels: updatedCount,
		SyncedGroups:    syncedGroups,
		Items:           items,
	}, nil
}
