package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelDefaultBaseURLsRequireReadPermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/default_base_urls", authz.ChannelRead, controller.GetChannelDefaultBaseURLs)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerChannelRoutes(engine.Group("/api"))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/channel/default_base_urls", nil))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestChannelStatusRoutesUseExpectedPermissions(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodGet, "/priority/order", authz.ChannelRead, controller.GetChannelPriorityOrder)
	assertChannelRoutePermission(t, http.MethodPost, "/priority/reorder", authz.ChannelWrite, controller.ReorderChannelPriority)
	assertChannelRoutePermission(t, http.MethodGet, "/:id/vllm/status", authz.ChannelRead, controller.GetVLLMChannelStatus)
	assertChannelRoutePermission(t, http.MethodGet, "/:id/sglang/status", authz.ChannelRead, controller.GetSGLangChannelStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/:id/status", authz.ChannelOperate, controller.UpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/status/batch", authz.ChannelOperate, controller.BatchUpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
	assertChannelRoutePermission(t, http.MethodGet, "/group_scheduling", authz.ChannelRead, controller.GetGroupScheduling)
	assertChannelRoutePermission(t, http.MethodPut, "/group_scheduling", authz.ChannelWrite, controller.UpdateGroupScheduling)
	assertChannelRoutePermission(t, http.MethodGet, "/model_price_table", authz.ChannelRead, controller.GetModelPriceTable)
	assertChannelRoutePermission(t, http.MethodPost, "/model_price_table/apply", authz.ChannelWrite, controller.ApplyModelPriceTable)
	assertChannelRoutePermission(t, http.MethodGet, "/cost_management/models", authz.ChannelRead, controller.GetModelCostAnalysis)
	assertChannelRoutePermission(t, http.MethodGet, "/cost_management/users", authz.ChannelRead, controller.GetUserCostAnalysis)
	assertChannelRoutePermission(t, http.MethodGet, "/model_health", authz.ChannelRead, controller.GetModelHealthAnalysis)
	assertChannelRoutePermission(t, http.MethodGet, "/classifier", authz.ChannelRead, controller.GetChannelClassifier)
	assertChannelRoutePermission(t, http.MethodPost, "/classifier", authz.ChannelWrite, controller.SaveChannelClassifier)
	assertChannelRoutePermission(t, http.MethodPut, "/classifier", authz.ChannelWrite, controller.SaveChannelClassifier)
	assertChannelRoutePermission(t, http.MethodDelete, "/classifier/:id", authz.ChannelWrite, controller.DeleteChannelClassifierRule)
	assertChannelRoutePermission(t, http.MethodPost, "/classifier/apply", authz.ChannelOperate, controller.ApplyChannelClassifierNow)
}

func TestChannelDeleteRoutesUseSensitiveWritePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelSensitiveWrite, controller.DeleteChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/batch", authz.ChannelSensitiveWrite, controller.DeleteChannelBatch)
	assertChannelRoutePermission(t, http.MethodDelete, "/disabled", authz.ChannelSensitiveWrite, controller.DeleteDisabledChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/tag", authz.ChannelWrite, controller.EditTagChannels)
	assertChannelRoutePermission(t, http.MethodPost, "/batch/tag", authz.ChannelWrite, controller.BatchSetChannelTag)
}

func TestPluginLoginRoutesUseExpectedPermissions(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodPost, "/:id/plugin_login", authz.ChannelSensitiveWrite, controller.StartPluginLogin)
	assertChannelRoutePermission(t, http.MethodPost, "/:id/plugin_login/continue", authz.ChannelSensitiveWrite, controller.ContinuePluginLogin)
	assertChannelRoutePermission(t, http.MethodPost, "/plugin_login/callback", authz.ChannelSensitiveWrite, controller.CompletePluginLoginCallback)
	assertChannelRoutePermission(t, http.MethodGet, "/:id/plugin_credentials", authz.ChannelRead, controller.GetPluginCredentials)
}

func TestChannelStatusRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerChannelRoutes(api)
	})
}

func assertChannelRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}

func TestChannelAndManagementAPIRoutesEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.User{},
		&model.UserSession{},
		&model.UserAccessToken{},
		&model.Channel{},
		&model.Ability{},
		&model.Log{},
		&model.TopUp{},
		&model.AuditLog{},
		&model.CasbinRule{},
		&model.AuthzRole{},
	))

	prevDB, prevLogDB := model.DB, model.LOG_DB
	prevMainType, prevLogType := common.MainDatabaseType(), common.LogDatabaseType()
	prevRedis := common.RedisEnabled
	prevMemCache := common.MemoryCacheEnabled
	prevSessionSecret := common.SessionSecret
	prevSyncCfg := model.GetModelPriceSyncConfig()
	prevModes := model.GetGroupSchedulingModes()
	prevPolicies := model.GetAllGroupCostControlPolicies()
	prevModelRatioJSON := ratio_setting.ModelRatio2JSONString()
	prevCompRatioJSON := ratio_setting.CompletionRatio2JSONString()
	prevPathScan := system_setting.GetPathScanSetting()

	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, model.InitLogDB())
	common.RedisEnabled = false
	common.MemoryCacheEnabled = true
	common.SessionSecret = "router-e2e-verification-secret"
	require.NoError(t, authz.Init(db))
	model.ResetRealtimeProfitTracker()

	t.Cleanup(func() {
		system_setting.SetPathScanSetting(prevPathScan)
		model.ResetRealtimeProfitTracker()
		_ = ratio_setting.UpdateModelRatioByJSONString(prevModelRatioJSON)
		_ = ratio_setting.UpdateCompletionRatioByJSONString(prevCompRatioJSON)
		if encodedModes, mErr := common.Marshal(prevModes); mErr == nil {
			_ = model.LoadGroupSchedulingConfigFromJSON(string(encodedModes))
		}
		if encodedPolicies, pErr := common.Marshal(prevPolicies); pErr == nil {
			_ = model.LoadGroupCostControlConfigFromJSON(string(encodedPolicies))
		}
		model.DB = nil
		_ = model.SaveModelPriceSyncConfig(prevSyncCfg)
		common.SessionSecret = prevSessionSecret
		common.MemoryCacheEnabled = prevMemCache
		common.RedisEnabled = prevRedis
		common.SetDatabaseTypes(prevMainType, prevLogType)
		model.DB, model.LOG_DB = prevDB, prevLogDB
	})

	rootUser := &model.User{
		Username:    "root-e2e",
		Password:    "placeholder",
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       5_000_000,
		UsedQuota:   900_000,
		AuthVersion: 1,
		AffCode:     "roote2e",
	}
	require.NoError(t, db.Create(rootUser).Error)
	loginTokens, err := service.CreateLoginSession(rootUser.Id, "password", "127.0.0.1", "e2e-test-runner")
	require.NoError(t, err)

	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-sonnet-4-5":1.5}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"claude-sonnet-4-5":5}`))

	baseURL := "https://api.moyuu.example.com"
	ch1 := &model.Channel{
		Name:    "moyuu-claude-0.2",
		Type:    1,
		Key:     "sk-test-moyuu",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
		Models:  "claude-sonnet-4-5",
		Group:   "default",
	}
	require.NoError(t, db.Create(ch1).Error)
	require.NoError(t, ch1.AddAbilities(db))
	model.InitChannelCache()

	nowTs := time.Now().Unix()
	require.NoError(t, db.Create(&model.Log{
		UserId:           rootUser.Id,
		CreatedAt:        nowTs - 10,
		Type:             model.LogTypeConsume,
		Username:         rootUser.Username,
		ModelName:        "claude-sonnet-4-5",
		Quota:            900_000,
		PromptTokens:     8000,
		CompletionTokens: 400,
		UseTime:          10,
		IsStream:         true,
		ChannelId:        ch1.Id,
		Group:            "default",
		Other:            `{"frt":2000}`,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId:    rootUser.Id,
		CreatedAt: nowTs - 5,
		Type:      model.LogTypeError,
		Username:  rootUser.Username,
		ModelName: "claude-sonnet-4-5",
		ChannelId: ch1.Id,
		Group:     "default",
	}).Error)

	engine := gin.New()
	SetApiRouter(engine)

	doAuthJSON := func(method string, target string, payload any) *httptest.ResponseRecorder {
		var bodyReader *bytes.Reader
		if payload != nil {
			b, mErr := common.Marshal(payload)
			require.NoError(t, mErr)
			bodyReader = bytes.NewReader(b)
		} else {
			bodyReader = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, target, bodyReader)
		req.Header.Set("Authorization", "Bearer "+loginTokens.AccessToken)
		req.Header.Set("X-Auth-Session", loginTokens.Session.SID)
		req.Header.Set("New-Api-User", strconv.Itoa(rootUser.Id))
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	assertOKSuccess := func(name string, rec *httptest.ResponseRecorder) map[string]any {
		require.Equalf(t, http.StatusOK, rec.Code, "%s HTTP status: body=%s", name, rec.Body.String())
		var parsed map[string]any
		require.NoErrorf(t, common.Unmarshal(rec.Body.Bytes(), &parsed), "%s JSON body=%s", name, rec.Body.String())
		assert.Equalf(t, true, parsed["success"], "%s success field: body=%s", name, rec.Body.String())
		return parsed
	}

	// 1. Public status endpoint
	assertOKSuccess("GET /api/status", doAuthJSON(http.MethodGet, "/api/status", nil))

	// 2. Core channel list & search endpoints
	assertOKSuccess("GET /api/channel/", doAuthJSON(http.MethodGet, "/api/channel/?p=1&page_size=10", nil))
	assertOKSuccess("GET /api/channel/search", doAuthJSON(http.MethodGet, "/api/channel/search?keyword=moyuu", nil))

	// 3. Upstream accounts CRUD
	assertOKSuccess("POST /api/channel/upstream_accounts", doAuthJSON(http.MethodPost, "/api/channel/upstream_accounts", map[string]any{
		"id":           "src-moyuu",
		"name":         "Moyuu",
		"base_url":     "https://api.moyuu.example.com",
		"access_token": "tok-demo",
		"currency":     "USD",
		"enabled":      true,
	}))
	assertOKSuccess("GET /api/channel/upstream_accounts", doAuthJSON(http.MethodGet, "/api/channel/upstream_accounts", nil))

	// 4. Group scheduling & Cost Control Scheduling GET/PUT
	assertOKSuccess("GET /api/channel/group_scheduling", doAuthJSON(http.MethodGet, "/api/channel/group_scheduling", nil))
	putSchedParsed := assertOKSuccess("PUT /api/channel/group_scheduling", doAuthJSON(http.MethodPut, "/api/channel/group_scheduling", map[string]any{
		"group":               "default",
		"mode":                "costcontrol",
		"enabled_channel_ids": []int{ch1.Id},
		"cost_control": map[string]any{
			"window":               "24h",
			"max_cost_ratio_pct":   80.0,
			"min_net_profit_usd":   0.0,
			"overflow_sensitivity": 1.8,
			"upstream_allocations": map[string]any{
				"src-moyuu": map[string]any{"enabled": true, "weight": 100},
			},
			"channel_rules": map[string]any{
				strconv.Itoa(ch1.Id): map[string]any{"priority": 10, "weight": 100},
			},
		},
	}))
	schedData, _ := putSchedParsed["data"].(map[string]any)
	require.NotNil(t, schedData)
	assert.NotEmpty(t, schedData["groups"])
	assert.NotEmpty(t, schedData["upstream_sources"])

	// 5. Model price table & CRON sync / CRON jobs endpoints
	assertOKSuccess("GET /api/channel/model_price_table", doAuthJSON(http.MethodGet, "/api/channel/model_price_table", nil))
	assertOKSuccess("POST /api/channel/model_price_table/apply", doAuthJSON(http.MethodPost, "/api/channel/model_price_table/apply", map[string]any{
		"items": []map[string]any{
			{
				"channel_id": ch1.Id,
				"model":      "claude-sonnet-4-5",
			},
		},
	}))
	assertOKSuccess("GET /api/channel/cron_sync_config", doAuthJSON(http.MethodGet, "/api/channel/cron_sync_config", nil))
	assertOKSuccess("GET /api/channel/cron_jobs", doAuthJSON(http.MethodGet, "/api/channel/cron_jobs", nil))

	// 6. Cost management analysis endpoints
	assertOKSuccess("GET /api/channel/cost_management/models", doAuthJSON(http.MethodGet, "/api/channel/cost_management/models", nil))
	assertOKSuccess("GET /api/channel/cost_management/users", doAuthJSON(http.MethodGet, "/api/channel/cost_management/users", nil))

	// 7. Model health telemetry endpoint (/api/channel/model_health)
	healthRec := doAuthJSON(http.MethodGet, "/api/channel/model_health", nil)
	require.Equal(t, http.StatusOK, healthRec.Code)
	var healthResp struct {
		Success bool `json:"success"`
		Data    struct {
			Models   []controller.ModelHealthAnalysisRow  `json:"models"`
			Channels []controller.ChannelHealthSummaryRow `json:"channels"`
			Summary  controller.ModelHealthSummary        `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(healthRec.Body.Bytes(), &healthResp))
	require.True(t, healthResp.Success)
	require.Len(t, healthResp.Data.Models, 1)
	require.Len(t, healthResp.Data.Channels, 1)
	assert.Equal(t, int64(2), healthResp.Data.Summary.RequestCount)
	assert.Equal(t, int64(1), healthResp.Data.Summary.SuccessCount)
	assert.Equal(t, int64(1), healthResp.Data.Summary.ErrorCount)
	assert.Equal(t, 50.0, healthResp.Data.Summary.Availability)
	// 8000 prompt / 2.0s TTFT = 4000 tok/s prefill; 400 completion / (10-2)s = 50 tok/s decode; 400 / 10s = 40 tok/s total
	assert.Equal(t, 4000.0, healthResp.Data.Models[0].PrefillTpsMedian)
	assert.Equal(t, 50.0, healthResp.Data.Models[0].DecodeTpsAvg)
	assert.Equal(t, 40.0, healthResp.Data.Models[0].TotalTpsAvg)

	// 8. Network behavior path-scan wildcard & Nginx regex options + bans management
	assertOKSuccess("PUT /api/option/ (wildcard & regex path scan rules)", doAuthJSON(http.MethodPut, "/api/option/", map[string]any{
		"key":   "path_scan_setting.path_scan_direct_paths",
		"value": "/*.env*\n/@fs/*\n" + `(?:\.(?:git|svn)|wp-admin|xmlrpc\.php)`,
	}))
	assertOKSuccess("GET /api/network-security/bans", doAuthJSON(http.MethodGet, "/api/network-security/bans", nil))
	assertOKSuccess("DELETE /api/network-security/bans", doAuthJSON(http.MethodDelete, "/api/network-security/bans", nil))
}
