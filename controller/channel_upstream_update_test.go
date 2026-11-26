package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAdvancedCustomModelListChannel(baseURL string, key string, upstreamPath string, auth *dto.AdvancedCustomRouteAuth) *model.Channel {
	config := &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{
			{
				IncomingPath: dto.AdvancedCustomModelListPath,
				UpstreamPath: upstreamPath,
				Converter:    "none",
				Auth:         auth,
			},
		},
	}
	channel := &model.Channel{
		Type:    constant.ChannelTypeAdvancedCustom,
		Key:     key,
		BaseURL: &baseURL,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: config})
	return channel
}

func TestParseOpenAIModelIDsStrictResponseContract(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      []string
		wantError string
	}{
		{name: "malformed JSON", body: `{"data":`, wantError: "invalid OpenAI Models response"},
		{name: "missing data", body: `{"object":"list"}`, wantError: "data is required"},
		{name: "null data", body: `{"data":null}`, wantError: "data is required"},
		{name: "empty data", body: `{"data":[]}`, wantError: "no valid model IDs"},
		{name: "all IDs empty", body: `{"data":[{"id":""},{"id":"   "}]}`, wantError: "no valid model IDs"},
		{
			name: "filters empty IDs and normalizes valid IDs",
			body: `{"data":[{"id":" gpt-4.1 "},{"id":""},{"id":"gpt-4.1"},{"id":"o3"}]}`,
			want: []string{"gpt-4.1", "o3"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			models, err := parseOpenAIModelIDs([]byte(test.body))
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, models)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, models)
		})
	}
}

func TestFetchAdvancedCustomModelsAppliesHeaderOverrideAfterRouteAuth(t *testing.T) {
	type receivedRequest struct {
		Headers http.Header
		Host    string
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- receivedRequest{Headers: r.Header.Clone(), Host: r.Host}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1"}]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "secret-key", "/provider/models", &dto.AdvancedCustomRouteAuth{
		Type:  dto.AdvancedCustomAuthTypeHeader,
		Name:  "X-Route-Key",
		Value: "route-{api_key}",
	})
	headerOverride := `{
		"X-Route-Key":"global-{api_key}",
		"X-Static":"static-value",
		"X-Client":"{client_header:X-Client}",
		"Host":"models.example.test",
		"*":""
	}`
	channel.HeaderOverride = &headerOverride

	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-4.1"}, models)

	request := <-received
	require.Equal(t, "global-secret-key", request.Headers.Get("X-Route-Key"))
	require.Equal(t, "static-value", request.Headers.Get("X-Static"))
	require.Empty(t, request.Headers.Get("X-Client"))
	require.Equal(t, "models.example.test", request.Host)
}

func TestFetchAdvancedCustomModelsUsesEnabledSavedMultiKey(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1-mini"}]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "disabled-key\nenabled-key", "/v1/models", nil)
	channel.ChannelInfo = model.ChannelInfo{
		IsMultiKey: true,
		MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusManuallyDisabled,
			1: common.ChannelStatusEnabled,
		},
	}

	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-4.1-mini"}, models)
	require.Equal(t, "Bearer enabled-key", <-authorization)
}

func TestFetchAdvancedCustomModelsRejectsNonOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"data":[{"id":"must-not-be-used"}]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "secret-key", "/v1/models", nil)
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.ErrorContains(t, err, "status code: 502")
	require.Nil(t, models)
}

func TestFetchAdvancedCustomModelsRedactsQueryKeyFromTransportErrors(t *testing.T) {
	const secret = "secret key/+"
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	channel := newAdvancedCustomModelListChannel(baseURL, secret, "/v1/models", &dto.AdvancedCustomRouteAuth{
		Type:  dto.AdvancedCustomAuthTypeQuery,
		Name:  "custom-token",
		Value: "prefix-{api_key}",
	})

	_, err := fetchChannelUpstreamModelIDs(channel)
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
	require.NotContains(t, err.Error(), "custom-token")
	require.NotContains(t, err.Error(), "prefix-")

	direct := sanitizeFetchModelsError(&url.Error{
		Op:  http.MethodGet,
		URL: baseURL + "/v1/models?custom-token=prefix-" + url.QueryEscape(secret),
		Err: errors.New("connection refused"),
	}, secret)
	require.EqualError(t, direct, "connection refused")

	queryValue := "prefix-" + secret
	queryError := sanitizeAdvancedCustomRequestError(
		errors.New("dial "+queryValue+": connection refused"),
		queryValue,
		baseURL+"/v1/models?custom-token="+url.QueryEscape(queryValue),
	)
	require.NotContains(t, queryError.Error(), queryValue)
	require.EqualError(t, queryError, "dial [REDACTED]: connection refused")
}

func TestFetchOrdinaryOpenAIModelsKeepsExistingEmptyDataBehavior(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list"}`))
	}))
	defer server.Close()

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "ordinary-key",
		BaseURL: &baseURL,
	}
	models, err := fetchChannelUpstreamModelIDs(channel)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestFetchModelsAdvancedCustomCreatePreview(t *testing.T) {
	receivedAuthorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthorization <- r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"preview-model"}]}`))
	}))
	defer server.Close()

	config := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
		IncomingPath: dto.AdvancedCustomModelListPath,
		UpstreamPath: "/preview/models",
		Converter:    "none",
	}}}
	configBytes, err := common.Marshal(config)
	require.NoError(t, err)
	rawConfig := string(configBytes)
	baseURL := server.URL
	emptyProxy := ""
	req := fetchModelsRequest{
		BaseURL:        &baseURL,
		Type:           constant.ChannelTypeAdvancedCustom,
		Key:            "create-preview-key",
		AdvancedCustom: &rawConfig,
		Proxy:          &emptyProxy,
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchModels(ctx)

	var response struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"preview-model"}, response.Data)
	require.Equal(t, "Bearer create-preview-key", <-receivedAuthorization)
}

func TestFetchModelsAdvancedCustomEditPreviewUsesSavedKeyAndExplicitClears(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	receivedHeaders := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders <- r.Header.Clone()
		_, _ = w.Write([]byte(`{"data":[{"id":"edited-preview-model"}]}`))
	}))
	defer server.Close()

	savedChannel := newAdvancedCustomModelListChannel("http://127.0.0.1:1", "disabled-saved-key\nenabled-saved-key", "/saved/models", nil)
	savedChannel.Name = "saved advanced channel"
	savedChannel.Models = "old-model"
	savedChannel.ChannelInfo = model.ChannelInfo{
		IsMultiKey: true,
		MultiKeyStatusList: map[int]int{
			0: common.ChannelStatusManuallyDisabled,
			1: common.ChannelStatusEnabled,
		},
	}
	savedHeaderOverride := `{"X-Saved":"must-not-be-sent"}`
	savedChannel.HeaderOverride = &savedHeaderOverride
	savedChannel.SetSetting(dto.ChannelSettings{Proxy: "http://127.0.0.1:1"})
	require.NoError(t, db.Create(savedChannel).Error)

	preserved, err := buildAdvancedCustomModelPreviewChannel(fetchModelsRequest{ChannelID: savedChannel.Id})
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:1", preserved.GetBaseURL())
	require.Equal(t, savedHeaderOverride, *preserved.HeaderOverride)
	require.Equal(t, "http://127.0.0.1:1", preserved.GetSetting().Proxy)

	previewConfig := dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
		IncomingPath: dto.AdvancedCustomModelListPath,
		UpstreamPath: "/edited/models",
		Converter:    "none",
	}}}
	configBytes, err := common.Marshal(previewConfig)
	require.NoError(t, err)
	rawConfig := string(configBytes)
	baseURL := server.URL
	explicitEmpty := ""
	req := fetchModelsRequest{
		ChannelID:      savedChannel.Id,
		BaseURL:        &baseURL,
		Type:           constant.ChannelTypeAdvancedCustom,
		Key:            "explicit-request-key",
		AdvancedCustom: &rawConfig,
		HeaderOverride: &explicitEmpty,
		Proxy:          &explicitEmpty,
	}
	cleared, err := buildAdvancedCustomModelPreviewChannel(fetchModelsRequest{
		ChannelID:      savedChannel.Id,
		BaseURL:        &explicitEmpty,
		AdvancedCustom: &rawConfig,
		HeaderOverride: &explicitEmpty,
		Proxy:          &explicitEmpty,
	})
	require.NoError(t, err)
	require.NotNil(t, cleared.BaseURL)
	require.Empty(t, *cleared.BaseURL)
	require.NotNil(t, cleared.HeaderOverride)
	require.Empty(t, *cleared.HeaderOverride)
	require.Empty(t, cleared.GetSetting().Proxy)

	body, err := common.Marshal(req)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	FetchModels(ctx)

	var response struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	require.Equal(t, []string{"edited-preview-model"}, response.Data)
	require.NotContains(t, recorder.Body.String(), "enabled-saved-key")
	require.NotContains(t, recorder.Body.String(), "explicit-request-key")

	headers := <-receivedHeaders
	require.Equal(t, "Bearer explicit-request-key", headers.Get("Authorization"), "a request that redirects the preview must authenticate with its own key, never the saved one")
	require.Empty(t, headers.Get("X-Saved"))
}

func TestFailedAdvancedCustomDetectionDoesNotStageFullRemoval(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	channel := newAdvancedCustomModelListChannel(server.URL, "secret-key", "/v1/models", nil)
	channel.Name = "empty discovery response"
	channel.Models = "gpt-4.1,o3"
	settings := channel.GetOtherSettings()
	settings.UpstreamModelUpdateCheckEnabled = true
	settings.UpstreamModelUpdateAutoSyncEnabled = true
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(channel).Error)

	modelsChanged, autoAdded, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, true)
	require.ErrorContains(t, err, "no valid model IDs")
	require.False(t, modelsChanged)
	require.Zero(t, autoAdded)
	require.Empty(t, settings.UpstreamModelUpdateLastDetectedModels)
	require.Empty(t, settings.UpstreamModelUpdateLastRemovedModels)

	reloaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	persistedSettings := reloaded.GetOtherSettings()
	require.Empty(t, persistedSettings.UpstreamModelUpdateLastDetectedModels)
	require.Empty(t, persistedSettings.UpstreamModelUpdateLastRemovedModels)
	require.Equal(t, "gpt-4.1,o3", reloaded.Models)
}

func TestFetchModelsUsesSharedChannelFetchBehavior(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "first-key" {
			t.Errorf("unexpected x-api-key header: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":" claude-sonnet "},{"id":"claude-sonnet"}]}`))
	}))
	t.Cleanup(server.Close)

	body, err := common.Marshal(map[string]any{
		"base_url": server.URL,
		"type":     constant.ChannelTypeAnthropic,
		"key":      "first-key\nsecond-key",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	FetchModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"success":true,"message":"","data":["claude-sonnet"]}`, recorder.Body.String())
}

func TestFetchNewAPIModelsUsesOpenAIContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer new-api-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data":[{"id":"gpt-5"},{"id":" gpt-5-mini "}]}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeNewAPI,
		Key:     "new-api-key",
		BaseURL: &baseURL,
	}

	models, err := fetchChannelUpstreamModelIDs(channel)

	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5", "gpt-5-mini"}, models)
}

func TestNormalizeModelNames(t *testing.T) {
	result := normalizeModelNames([]string{
		" gpt-4o ",
		"",
		"gpt-4o",
		"gpt-4.1",
		"   ",
	})

	require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, result)
}

func TestMergeModelNames(t *testing.T) {
	result := mergeModelNames(
		[]string{"gpt-4o", "gpt-4.1"},
		[]string{"gpt-4.1", " gpt-4.1-mini ", "gpt-4o"},
	)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini"}, result)
}

func TestSubtractModelNames(t *testing.T) {
	result := subtractModelNames(
		[]string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini"},
		[]string{"gpt-4.1", "not-exists"},
	)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1-mini"}, result)
}

func TestIntersectModelNames(t *testing.T) {
	result := intersectModelNames(
		[]string{"gpt-4o", "gpt-4.1", "gpt-4.1", "not-exists"},
		[]string{"gpt-4.1", "gpt-4o-mini", "gpt-4o"},
	)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, result)
}

func TestApplySelectedModelChanges(t *testing.T) {
	t.Run("add and remove together", func(t *testing.T) {
		result := applySelectedModelChanges(
			[]string{"gpt-4o", "gpt-4.1", "claude-3"},
			[]string{"gpt-4.1-mini"},
			[]string{"claude-3"},
		)

		require.Equal(t, []string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini"}, result)
	})

	t.Run("add wins when conflict with remove", func(t *testing.T) {
		result := applySelectedModelChanges(
			[]string{"gpt-4o"},
			[]string{"gpt-4.1"},
			[]string{"gpt-4.1"},
		)

		require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, result)
	})
}

func TestCollectPendingApplyUpstreamModelChanges(t *testing.T) {
	settings := dto.ChannelOtherSettings{
		UpstreamModelUpdateLastDetectedModels: []string{" gpt-4o ", "gpt-4o", "gpt-4.1"},
		UpstreamModelUpdateLastRemovedModels:  []string{" old-model ", "", "old-model"},
	}

	pendingAddModels, pendingRemoveModels := collectPendingApplyUpstreamModelChanges(settings)

	require.Equal(t, []string{"gpt-4o", "gpt-4.1"}, pendingAddModels)
	require.Equal(t, []string{"old-model"}, pendingRemoveModels)
}

func TestNormalizeChannelModelMapping(t *testing.T) {
	modelMapping := `{
		" alias-model ": " upstream-model ",
		"": "invalid",
		"invalid-target": ""
	}`
	channel := &model.Channel{
		ModelMapping: &modelMapping,
	}

	result := normalizeChannelModelMapping(channel)
	require.Equal(t, map[string]string{
		"alias-model": "upstream-model",
	}, result)
}

func TestCollectPendingUpstreamModelChangesFromModels_WithModelMapping(t *testing.T) {
	pendingAddModels, pendingRemoveModels := collectPendingUpstreamModelChangesFromModels(
		[]string{"alias-model", "gpt-4o", "stale-model"},
		[]string{"gpt-4o", "gpt-4.1", "mapped-target"},
		[]string{"gpt-4.1"},
		map[string]string{
			"alias-model": "mapped-target",
		},
	)

	require.Equal(t, []string{}, pendingAddModels)
	require.Equal(t, []string{"stale-model"}, pendingRemoveModels)
}

func TestCollectPendingUpstreamModelChangesFromModels_WithIgnoredRegexPatterns(t *testing.T) {
	pendingAddModels, pendingRemoveModels := collectPendingUpstreamModelChangesFromModels(
		[]string{"gpt-4o"},
		[]string{"gpt-4o", "claude-3-5-sonnet", "sora-video", "gpt-4.1"},
		[]string{"regex:^sora-.*$", "gpt-4.1"},
		nil,
	)

	require.Equal(t, []string{"claude-3-5-sonnet"}, pendingAddModels)
	require.Equal(t, []string{}, pendingRemoveModels)
}

func TestBuildUpstreamModelUpdateTaskNotificationContent_OmitOverflowDetails(t *testing.T) {
	channelSummaries := make([]upstreamModelUpdateChannelSummary, 0, 12)
	for i := range 12 {
		channelSummaries = append(channelSummaries, upstreamModelUpdateChannelSummary{
			ChannelName: "channel-" + string(rune('A'+i)),
			AddCount:    i + 1,
			RemoveCount: i,
		})
	}

	require.NoError(t, i18n.Init())
	content := buildUpstreamModelUpdateTaskNotificationContent(
		i18n.LangEn,
		24,
		12,
		56,
		21,
		9,
		[]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		channelSummaries,
		[]string{
			"gpt-4.1", "gpt-4.1-mini", "o3", "o4-mini", "gemini-2.5-pro", "claude-3.7-sonnet",
			"qwen-max", "deepseek-r1", "llama-3.3-70b", "mistral-large", "command-r-plus", "doubao-pro-32k",
			"hunyuan-large",
		},
		[]string{
			"gpt-3.5-turbo", "claude-2.1", "gemini-1.5-pro", "mixtral-8x7b", "qwen-plus", "glm-4",
			"yi-large", "moonshot-v1", "doubao-lite",
		},
	)

	require.Contains(t, content, i18n.Translate(i18n.LangEn, i18n.MsgChannelUpstreamUpdateMoreChannels, map[string]any{"Count": 4}))
	require.Contains(t, content, i18n.Translate(i18n.LangEn, i18n.MsgChannelUpstreamUpdateMoreOmitted, map[string]any{"Count": 1}))
	require.Contains(t, content, i18n.Translate(i18n.LangEn, i18n.MsgChannelUpstreamUpdateFailedChannels, map[string]any{
		"Shown": 10,
		"Total": 12,
		"Ids":   "1, 2, 3, 4, 5, 6, 7, 8, 9, 10",
	}))
	require.Contains(t, content, i18n.Translate(i18n.LangEn, i18n.MsgChannelUpstreamUpdateMoreOmitted, map[string]any{"Count": 2}))
}

func TestShouldSendUpstreamModelUpdateNotification(t *testing.T) {
	channelUpstreamModelUpdateNotifyState.Lock()
	channelUpstreamModelUpdateNotifyState.lastNotifiedAt = 0
	channelUpstreamModelUpdateNotifyState.lastChangedChannels = 0
	channelUpstreamModelUpdateNotifyState.lastFailedChannels = 0
	channelUpstreamModelUpdateNotifyState.Unlock()

	baseTime := int64(2000000)

	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime, 6, 0))
	require.False(t, shouldSendUpstreamModelUpdateNotification(baseTime+3600, 6, 0))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+3600, 7, 0))
	require.False(t, shouldSendUpstreamModelUpdateNotification(baseTime+7200, 7, 0))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+8000, 0, 3))
	require.False(t, shouldSendUpstreamModelUpdateNotification(baseTime+9000, 0, 3))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+10000, 0, 4))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+90000, 7, 0))
	require.True(t, shouldSendUpstreamModelUpdateNotification(baseTime+90001, 0, 0))
}

func TestDetectAllChannelUpstreamModelUpdatesRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeModelUpdate, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/upstream-models/detect-all", nil)

	DetectAllChannelUpstreamModelUpdates(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "A model update task is already running or queued")
}

func TestUpstreamTopupMultiplierOnlyAffectsCostCalculation(t *testing.T) {
	prevCfg := model.GetModelPriceSyncConfig()
	t.Cleanup(func() {
		_ = model.SaveModelPriceSyncConfig(prevCfg)
	})

	cfg := model.GetModelPriceSyncConfig()
	cfg.UpstreamAccounts = []model.UpstreamAccountSource{
		{
			ID:              "upstream-test-2x",
			Name:            "nearToken",
			BaseURL:         "https://neartoken.cc",
			Currency:        "USD",
			ExchangeRate:    1.0,
			TopupMultiplier: 2.0,
			Enabled:         true,
		},
	}
	cfg.AutoProvisionedChannels = map[int]model.AutoProvisionedChannelMeta{
		164: {
			ChannelID:     164,
			ChannelName:   "nearToken-default-1",
			SourceID:      "upstream-test-2x",
			SourceName:    "nearToken",
			UpstreamGroup: "default",
			AutoCreated:   true,
		},
	}
	cfg.ChannelTopupMultipliers = map[int]float64{
		164: 2.0,
	}
	require.NoError(t, model.SaveModelPriceSyncConfig(cfg))

	// Nominal upstream price: $100 per 1M input tokens (ModelRatio = 50), 1x group ratio
	model.SetChannelUpstreamModelCosts(164, map[string]model.ChannelModelCostEntry{
		"test-topup-2x-model": {
			ChannelID:       164,
			ChannelName:     "nearToken-default-1",
			ChannelType:     constant.ChannelTypeNewAPI,
			Model:           "test-topup-2x-model",
			QuotaType:       0,
			ModelRatio:      50.0,
			CompletionRatio: 1.0,
			RawInputCost:    100.0,
			RawOutputCost:   100.0,
			InputCost:       100.0,
			OutputCost:      100.0,
			BaseMultiplier:  1.0,
			Currency:        "USD",
			ExchangeRate:    1.0,
			FromUpstream:    true,
		},
	}, false)

	// 1. 1x authoritative selling price must remain 100 ($100/1M => ModelRatio = 50), NOT discounted by TopupMultiplier
	qt, _, mr, cr, _, _ := model.ResolveAuthoritativeModelPricingUSD("test-topup-2x-model")
	require.Equal(t, 0, qt)
	require.InDelta(t, 50.0, mr, 1e-6)
	require.InDelta(t, 1.0, cr, 1e-6)

	// 2. Actual cost calculation for 1M prompt tokens must be divided by TopupMultiplier (100 / 2 = 50)
	actualCost := model.CalculateChannelModelUsageCost(164, "test-topup-2x-model", 1, 1_000_000, 0, 0, 0)
	require.InDelta(t, 50.0, actualCost, 1e-6)

	// 3. Resolved top-up multiplier for channel 164 is 2.0
	resolvedMult := model.ResolveChannelTopupMultiplier(164, nil, model.GetModelPriceSyncConfig())
	require.InDelta(t, 2.0, resolvedMult, 1e-6)
}

func TestDeletedChannelFrozenCostSnapshotAndSyncConfigCleanup(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	prevLogDB := model.LOG_DB
	prevCfg := model.GetModelPriceSyncConfig()
	prevLogConsume := common.LogConsumeEnabled
	prevDataExport := common.DataExportEnabled
	model.LOG_DB = db
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = prevLogDB
		common.LogConsumeEnabled = prevLogConsume
		common.DataExportEnabled = prevDataExport
		_ = model.SaveModelPriceSyncConfig(prevCfg)
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.TopUp{}, &model.Option{}, &model.Ability{}))

	user := &model.User{Username: "alice", DisplayName: "Alice", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)

	curr := "USD"
	usedCh := &model.Channel{
		Name:         "nearToken-default-1",
		Type:         constant.ChannelTypeNewAPI,
		Key:          "sk-used",
		Models:       "test-frozen-cost-model",
		Group:        "default",
		Status:       common.ChannelStatusEnabled,
		CurrencyCode: &curr,
	}
	require.NoError(t, db.Create(usedCh).Error)

	unusedCh := &model.Channel{
		Name:   "lumiya-empty-1",
		Type:   constant.ChannelTypeNewAPI,
		Key:    "sk-unused",
		Models: "",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(unusedCh).Error)

	cfg := model.GetModelPriceSyncConfig()
	cfg.ChannelCurrencies[usedCh.Id] = "USD"
	cfg.ChannelExchangeRates[usedCh.Id] = 1.0
	cfg.ChannelTopupMultipliers[usedCh.Id] = 2.0
	cfg.ChannelUpstreamGroups[usedCh.Id] = "default"
	cfg.PreferredChannels["test-frozen-cost-model"] = usedCh.Id
	cfg.AutoProvisionedChannels[usedCh.Id] = model.AutoProvisionedChannelMeta{
		ChannelID:     usedCh.Id,
		ChannelName:   usedCh.Name,
		SourceID:      "src-1",
		SourceName:    "nearToken",
		UpstreamGroup: "default",
		AutoCreated:   true,
	}
	cfg.ChannelTopupMultipliers[unusedCh.Id] = 2.0
	cfg.AutoProvisionedChannels[unusedCh.Id] = model.AutoProvisionedChannelMeta{
		ChannelID:     unusedCh.Id,
		ChannelName:   unusedCh.Name,
		SourceID:      "src-2",
		SourceName:    "lumiya",
		UpstreamGroup: "empty",
		AutoCreated:   true,
	}
	require.NoError(t, model.SaveModelPriceSyncConfig(cfg))

	model.SetChannelUpstreamModelCosts(usedCh.Id, map[string]model.ChannelModelCostEntry{
		"test-frozen-cost-model": {
			ChannelID:       usedCh.Id,
			ChannelName:     usedCh.Name,
			ChannelType:     constant.ChannelTypeNewAPI,
			Model:           "test-frozen-cost-model",
			QuotaType:       0,
			ModelRatio:      50.0,
			CompletionRatio: 1.0,
			RawInputCost:    100.0,
			RawOutputCost:   100.0,
			InputCost:       100.0,
			OutputCost:      100.0,
			BaseMultiplier:  1.0,
			Currency:        "USD",
			ExchangeRate:    1.0,
			FromUpstream:    true,
		},
	}, false)

	// Record a consume log for 1M prompt tokens (revenue = $100 = 50,000,000 quota, frozen actual cost = $50)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Set("username", user.Username)
	model.RecordConsumeLog(ginCtx, user.Id, model.RecordConsumeLogParams{
		ChannelId:    usedCh.Id,
		ModelName:    "test-frozen-cost-model",
		PromptTokens: 1_000_000,
		Quota:        int(100 * common.QuotaPerUnit),
		Group:        "default",
		Other:        model.NewLogOther(),
	})

	// Delete both channels
	require.NoError(t, usedCh.Delete())
	require.NoError(t, unusedCh.Delete())

	// Active config maps must have ZERO leftovers for both deleted channels
	afterCfg := model.GetModelPriceSyncConfig()
	_, hasAuto := afterCfg.AutoProvisionedChannels[usedCh.Id]
	_, hasMult := afterCfg.ChannelTopupMultipliers[usedCh.Id]
	_, hasCurr := afterCfg.ChannelCurrencies[usedCh.Id]
	_, hasPref := afterCfg.PreferredChannels["test-frozen-cost-model"]
	require.False(t, hasAuto)
	require.False(t, hasMult)
	require.False(t, hasCurr)
	require.False(t, hasPref)
	_, hasUnusedMult := afterCfg.ChannelTopupMultipliers[unusedCh.Id]
	require.False(t, hasUnusedMult)

	// Used channel must be archived; unused channel (0 logs) must NOT be archived
	snap, hasSnap := model.GetArchivedChannelSnapshot(usedCh.Id)
	require.True(t, hasSnap)
	require.Equal(t, "nearToken-default-1", snap.ChannelName)
	require.InDelta(t, 2.0, snap.TopupMultiplier, 1e-6)
	_, hasUnusedSnap := model.GetArchivedChannelSnapshot(unusedCh.Id)
	require.False(t, hasUnusedSnap)

	// GetModelCostAnalysis must preserve frozen cost ($50) and 50% margin after channel deletion
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/cost_management/models?keyword=test-frozen-cost-model", nil)
	GetModelCostAnalysis(c)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Models []ModelCostAnalysisRow `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data.Models, 1)
	row := resp.Data.Models[0]
	require.InDelta(t, 100.0, row.RevenueUSD, 1e-4)
	require.InDelta(t, 50.0, row.CostUSD, 1e-4)
	require.InDelta(t, 50.0, row.ProfitUSD, 1e-4)
	require.InDelta(t, 50.0, row.MarginPct, 1e-2)
	require.Len(t, row.ChannelUsages, 1)
	require.Equal(t, "nearToken-default-1 (Deleted)", row.ChannelUsages[0].ChannelName)
}

func TestSyncSingleUpstreamAccountSkipsEmptyGroupsWritesCurrencyAndCleansRemovedGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	prevLogDB := model.LOG_DB
	prevCfg := model.GetModelPriceSyncConfig()
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = prevLogDB
		_ = model.SaveModelPriceSyncConfig(prevCfg)
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Option{}, &model.Ability{}))

	var createdTokenGroups []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"desc":"Default","ratio":1},"empty-img":{"desc":"Empty Image Group","ratio":1}}}`))
		case "/api/pricing":
			// phantom-removed only exists in group_ratio (not in self/groups) and empty-img has 0 models
			_, _ = w.Write([]byte(`{
				"success":true,
				"data":[{"model_name":"gpt-4o","quota_type":0,"model_ratio":1.25,"completion_ratio":4,"enable_groups":["default"]}],
				"group_ratio":{"default":1,"empty-img":1,"phantom-removed":0.5},
				"usable_group":{"default":"Default","empty-img":"Empty Image Group"}
			}`))
		case "/api/token/":
			if r.Method == http.MethodPost {
				bodyBytes, _ := io.ReadAll(r.Body)
				var payload struct {
					Group string `json:"group"`
				}
				_ = common.Unmarshal(bodyBytes, &payload)
				createdTokenGroups = append(createdTokenGroups, payload.Group)
				_, _ = w.Write([]byte(`{"success":true}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":1,"name":"new_api-default-1","key":"sk-valid-default-key","status":1,"group":"default","unlimited_quota":true}]}}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":5000000,"used_quota":0}}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	baseURL := upstream.URL
	tag := "auto:testup"
	// Pre-existing empty-model channel and disappeared phantom-group channel
	staleEmptyCh := &model.Channel{
		Name:    "testup-empty-img-1",
		Type:    constant.ChannelTypeNewAPI,
		BaseURL: &baseURL,
		Key:     "sk-empty",
		Group:   "default",
		Models:  "",
		Tag:     &tag,
		Status:  common.ChannelStatusEnabled,
	}
	staleRemovedCh := &model.Channel{
		Name:    "testup-phantom-removed-0.5",
		Type:    constant.ChannelTypeNewAPI,
		BaseURL: &baseURL,
		Key:     "sk-removed",
		Group:   "default",
		Models:  "old-model",
		Tag:     &tag,
		Status:  common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(staleEmptyCh).Error)
	require.NoError(t, db.Create(staleRemovedCh).Error)

	source := model.UpstreamAccountSource{
		ID:                   "src-sync-test",
		Name:                 "testup",
		BaseURL:              upstream.URL,
		SyncMode:             model.UpstreamSyncModeStandard,
		UserID:               "1",
		AccessToken:          "tok-secret",
		Currency:             "CNY",
		TopupMultiplier:      2.0,
		CleanRemovedGroups:   true,
		AdoptExistingChannel: true,
		Enabled:              true,
	}
	cfg := model.GetModelPriceSyncConfig()
	cfg.AutoProvisionedChannels[staleEmptyCh.Id] = model.AutoProvisionedChannelMeta{
		ChannelID:     staleEmptyCh.Id,
		ChannelName:   staleEmptyCh.Name,
		SourceID:      source.ID,
		SourceName:    source.Name,
		UpstreamGroup: "empty-img",
		AutoCreated:   true,
	}
	cfg.AutoProvisionedChannels[staleRemovedCh.Id] = model.AutoProvisionedChannelMeta{
		ChannelID:     staleRemovedCh.Id,
		ChannelName:   staleRemovedCh.Name,
		SourceID:      source.ID,
		SourceName:    source.Name,
		UpstreamGroup: "phantom-removed",
		AutoCreated:   true,
	}
	cfg.ChannelTopupMultipliers[staleEmptyCh.Id] = 2.0
	cfg.ChannelTopupMultipliers[staleRemovedCh.Id] = 2.0
	cfg.AutoSyncPrices = false

	require.NoError(t, syncSingleUpstreamAccountSource(t.Context(), &source, &cfg))

	// No upstream token should be created for empty-img or phantom-removed
	require.Empty(t, createdTokenGroups)

	// Only testup-default-1 should remain in DB, with currency_code = "CNY" and models = "gpt-4o"
	var remaining []*model.Channel
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "testup-default-1", remaining[0].Name)
	require.Equal(t, "gpt-4o", remaining[0].Models)
	require.NotNil(t, remaining[0].CurrencyCode)
	require.Equal(t, "CNY", *remaining[0].CurrencyCode)

	// Both stale channels must be purged from config maps
	finalCfg := model.GetModelPriceSyncConfig()
	_, hasEmpty := finalCfg.ChannelTopupMultipliers[staleEmptyCh.Id]
	_, hasRemoved := finalCfg.ChannelTopupMultipliers[staleRemovedCh.Id]
	require.False(t, hasEmpty)
	require.False(t, hasRemoved)
	require.InDelta(t, 2.0, finalCfg.ChannelTopupMultipliers[remaining[0].Id], 1e-6)
}

func TestUpstreamAccountsNoHardcodedCredentialsSecretRedactionAndRBACIsolation(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	prevLogDB := model.LOG_DB
	prevCfg := model.GetModelPriceSyncConfig()
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = prevLogDB
		_ = model.SaveModelPriceSyncConfig(prevCfg)
	})
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Ability{}))

	var seenAuth sync.Map
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		seenAuth.Store(r.Header.Get("New-Api-User"), r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"default":{"desc":"Default","ratio":1}}}`))
		case "/api/pricing":
			_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"gpt-4o-mini","quota_type":0,"model_ratio":1,"completion_ratio":2,"enable_groups":["default"]}],"group_ratio":{"default":1},"usable_group":{"default":"Default"}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"id":1,"name":"tok-default","key":"sk-up-1","status":1,"group":"default","unlimited_quota":true}],"total":1}}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":2500000,"used_quota":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	const (
		sentinelTokA = "fake-secret-token-alpha-918273645"
		sentinelTokB = "fake-secret-token-bravo-556677889"
	)

	// 1. Missing required UserID or AccessToken reports pending_config without filling defaults
	missingUser := model.UpstreamAccountSource{
		ID:          "src-missing-uid",
		Name:        "lumiya-fictional",
		BaseURL:     upstream.URL,
		SyncMode:    model.UpstreamSyncModeLumiya,
		AccessToken: sentinelTokA,
		Enabled:     true,
	}
	cfg := model.GetModelPriceSyncConfig()
	err := syncSingleUpstreamAccountSource(t.Context(), &missingUser, &cfg)
	require.Error(t, err)
	assert.Equal(t, "pending_config", missingUser.LastStatus)
	assert.Empty(t, missingUser.UserID)

	// 2. Two fictional accounts preserve distinct UserID and AccessToken across save, JSON reload, and sync
	cfg.UpstreamAccounts = []model.UpstreamAccountSource{
		{
			ID:          "src-alpha",
			Name:        "lumiya-alpha",
			BaseURL:     upstream.URL,
			SyncMode:    model.UpstreamSyncModeLumiya,
			UserID:      "7001",
			AccessToken: sentinelTokA,
			Currency:    "CNY",
			Enabled:     true,
		},
		{
			ID:          "src-bravo",
			Name:        "standard-bravo",
			BaseURL:     upstream.URL,
			SyncMode:    model.UpstreamSyncModeStandard,
			UserID:      "8002",
			AccessToken: sentinelTokB,
			Currency:    "USD",
			Enabled:     true,
		},
	}
	cfg.AutoProvisionedChannels = map[int]model.AutoProvisionedChannelMeta{
		99: {ChannelID: 99, ChannelName: "auto-ch-99", SourceID: "src-alpha", SourceName: "lumiya-alpha", UpstreamGroup: "default", AutoCreated: true},
	}
	require.NoError(t, model.SaveModelPriceSyncConfig(cfg))
	require.NoError(t, model.LoadModelPriceSyncConfigFromJSON(common.OptionMap[model.ModelPriceSyncConfigOptionKey]))

	reloaded := model.GetModelPriceSyncConfig()
	require.Len(t, reloaded.UpstreamAccounts, 2)
	assert.Equal(t, "7001", reloaded.UpstreamAccounts[0].UserID)
	assert.Equal(t, sentinelTokA, reloaded.UpstreamAccounts[0].AccessToken)
	assert.Equal(t, "8002", reloaded.UpstreamAccounts[1].UserID)
	assert.Equal(t, sentinelTokB, reloaded.UpstreamAccounts[1].AccessToken)

	_, syncErr := ExecuteUpstreamAccountsProvisionSync(t.Context(), nil)
	require.NoError(t, syncErr)
	authA, okA := seenAuth.Load("7001")
	authB, okB := seenAuth.Load("8002")
	require.True(t, okA)
	require.True(t, okB)
	assert.Equal(t, "Bearer "+sentinelTokA, authA)
	assert.Equal(t, "Bearer "+sentinelTokB, authB)

	// 3. No API response (including GetOptions JSON string) may contain sentinelTokA or sentinelTokB
	assertNoSentinelLeak := func(name string, handler gin.HandlerFunc, method string, target string, body string) string {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		c.Request = httptest.NewRequest(method, target, reader)
		if body != "" {
			c.Request.Header.Set("Content-Type", "application/json")
		}
		handler(c)
		rawBody := rec.Body.String()
		assert.NotContains(t, rawBody, sentinelTokA, "%s leaked sentinelTokA", name)
		assert.NotContains(t, rawBody, sentinelTokB, "%s leaked sentinelTokB", name)
		return rawBody
	}

	assertNoSentinelLeak("GetModelPriceTable", GetModelPriceTable, http.MethodGet, "/api/channel/model_price_table", "")
	assertNoSentinelLeak("ApplyModelPriceTable", ApplyModelPriceTable, http.MethodPost, "/api/channel/model_price_table/apply", `{"highest_price_priority":true}`)
	assertNoSentinelLeak("GetCronSyncConfig", GetCronSyncConfig, http.MethodGet, "/api/channel/cron_sync_config", "")
	upAccBody := assertNoSentinelLeak("GetUpstreamAccounts", GetUpstreamAccounts, http.MethodGet, "/api/channel/upstream_accounts", "")
	assert.Contains(t, upAccBody, `"has_access_token":true`)
	assert.NotContains(t, upAccBody, `"access_token"`)
	optBody := assertNoSentinelLeak("GetOptions", GetOptions, http.MethodGet, "/api/option/", "")
	assert.Contains(t, optBody, model.ModelPriceSyncConfigOptionKey)

	// 4. ChannelWrite general update (even if legacy client posts masked/tampered upstream_accounts) cannot alter UpstreamAccounts or AutoProvisionedChannels
	beforeWrite := model.GetModelPriceSyncConfig()
	legacyPayload := `{
		"cny_exchange_rate": 7.15,
		"twd_exchange_rate": 31.5,
		"upstream_accounts": [{"id":"src-alpha","name":"hijacked","base_url":"https://evil.example","user_id":"999","access_token":"tampered-token"}],
		"auto_provisioned_channels": {}
	}`
	assertNoSentinelLeak("UpdateCronSyncConfig", UpdateCronSyncConfig, http.MethodPut, "/api/channel/cron_sync_config", legacyPayload)
	assertNoSentinelLeak("RunCronSyncNow", RunCronSyncNow, http.MethodPost, "/api/channel/cron_sync_run", `{"channel_ids":[999999]}`)

	afterWrite := model.GetModelPriceSyncConfig()
	assert.InDelta(t, 7.15, afterWrite.CNYExchangeRate, 1e-6)
	assert.InDelta(t, 31.5, afterWrite.TWDExchangeRate, 1e-6)
	assert.Equal(t, beforeWrite.UpstreamAccounts, afterWrite.UpstreamAccounts, "ChannelWrite endpoint must preserve UpstreamAccounts byte-for-byte")
	assert.Equal(t, beforeWrite.AutoProvisionedChannels, afterWrite.AutoProvisionedChannels, "ChannelWrite endpoint must preserve AutoProvisionedChannels")

	// 5. Concurrent ChannelWrite rate update + ChannelSensitiveWrite SaveUpstreamAccount preserve both changes
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		assertNoSentinelLeak("ConcurrentUpdateCronSync", UpdateCronSyncConfig, http.MethodPut, "/api/channel/cron_sync_config", `{"twd_exchange_rate": 32.8}`)
	}()
	go func() {
		defer wg.Done()
		saveBody := fmt.Sprintf(`{"id":"src-charlie","name":"charlie","base_url":%q,"user_id":"9003","access_token":"fake-secret-charlie-112233","currency":"USD","enabled":true}`, upstream.URL)
		assertNoSentinelLeak("ConcurrentSaveUpstreamAccount", SaveUpstreamAccount, http.MethodPost, "/api/channel/upstream_accounts", saveBody)
	}()
	wg.Wait()

	merged := model.GetModelPriceSyncConfig()
	assert.InDelta(t, 32.8, merged.TWDExchangeRate, 1e-6)
	require.Len(t, merged.UpstreamAccounts, 3)
}

func TestUpstreamSyncPreservesChannelsOnTransientFailureAndHandlesDeleteFailure(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	prevLogDB := model.LOG_DB
	prevCfg := model.GetModelPriceSyncConfig()
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = prevLogDB
		_ = model.SaveModelPriceSyncConfig(prevCfg)
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Option{}, &model.Ability{}))

	var failPricingAndV1 atomic.Bool
	var groupRemovedAuthoritatively atomic.Bool

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self/groups":
			if groupRemovedAuthoritatively.Load() {
				_, _ = w.Write([]byte(`{"success":true,"data":{"keep":{"desc":"Keep","ratio":1}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"keep":{"desc":"Keep","ratio":1},"transient":{"desc":"Transient","ratio":1}}}`))
		case "/api/pricing":
			if failPricingAndV1.Load() {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"success":false,"message":"bad gateway"}`))
				return
			}
			if groupRemovedAuthoritatively.Load() {
				_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"gpt-4o","quota_type":0,"model_ratio":1,"completion_ratio":2,"enable_groups":["keep"]}],"group_ratio":{"keep":1},"usable_group":{"keep":"Keep"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"gpt-4o","quota_type":0,"model_ratio":1,"completion_ratio":2,"enable_groups":["keep","transient"]}],"group_ratio":{"keep":1,"transient":1},"usable_group":{"keep":"Keep","transient":"Transient"}}`))
		case "/api/token/":
			_, _ = w.Write([]byte(`{"success":true,"data":{"items":[
				{"id":1,"name":"tok-keep","key":"sk-keep-plain-token-key-0001","status":1,"group":"keep","unlimited_quota":true},
				{"id":2,"name":"tok-transient","key":"sk-transient-plain-token-key-0002","status":1,"group":"transient","unlimited_quota":true}
			],"total":2}}`))
		case "/v1/models":
			if failPricingAndV1.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":5000000,"used_quota":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	source := model.UpstreamAccountSource{
		ID:                 "src-resilience",
		Name:               "resup",
		BaseURL:            upstream.URL,
		SyncMode:           model.UpstreamSyncModeStandard,
		UserID:             "1",
		AccessToken:        "fake-resilience-token",
		Currency:           "USD",
		CleanRemovedGroups: false,
		Enabled:            true,
	}
	cfg := model.GetModelPriceSyncConfig()
	require.NoError(t, syncSingleUpstreamAccountSource(t.Context(), &source, &cfg))
	cfg = model.GetModelPriceSyncConfig()

	var baseline []*model.Channel
	require.NoError(t, db.Order("id ASC").Find(&baseline).Error)
	require.Len(t, baseline, 2)

	// 1. Transient failure under BOTH CleanRemovedGroups=false and CleanRemovedGroups=true preserves channels, models, status, and links
	failPricingAndV1.Store(true)
	for _, cleanRemoved := range []bool{false, true} {
		source.CleanRemovedGroups = cleanRemoved
		err := syncSingleUpstreamAccountSource(t.Context(), &source, &cfg)
		require.Error(t, err)
		assert.Equal(t, "partial", source.LastStatus)

		var current []*model.Channel
		require.NoError(t, db.Order("id ASC").Find(&current).Error)
		require.Len(t, current, 2)
		for _, ch := range current {
			assert.Equal(t, "gpt-4o", ch.Models)
			assert.Equal(t, common.ChannelStatusEnabled, ch.Status)
			_, hasLink := model.GetModelPriceSyncConfig().AutoProvisionedChannels[ch.Id]
			assert.True(t, hasLink)
		}
	}

	// 2. Authoritative group removal + simulated DB delete failure must keep association until delete succeeds on retry
	failPricingAndV1.Store(false)
	groupRemovedAuthoritatively.Store(true)
	source.CleanRemovedGroups = true

	var transientCh model.Channel
	require.NoError(t, db.Where("name = ?", "resup-transient-1").First(&transientCh).Error)

	const failDeleteCb = "test:fail_channel_delete"
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(failDeleteCb, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "channels" {
			tx.AddError(errors.New("simulated db delete lock"))
		}
	}))
	delErr := syncSingleUpstreamAccountSource(t.Context(), &source, &cfg)
	require.NoError(t, db.Callback().Delete().Remove(failDeleteCb))
	require.Error(t, delErr)
	assert.Equal(t, "partial", source.LastStatus)
	assert.Empty(t, source.RemovedChannels)
	_, stillLinked := model.GetModelPriceSyncConfig().AutoProvisionedChannels[transientCh.Id]
	assert.True(t, stillLinked, "failed DB delete must not orphan channel association")

	// 3. Next sync after DB recovers deletes the removed group cleanly and marks success
	cfg = model.GetModelPriceSyncConfig()
	require.NoError(t, syncSingleUpstreamAccountSource(t.Context(), &source, &cfg))
	assert.Equal(t, "success", source.LastStatus)
	assert.Equal(t, []string{"resup-transient-1"}, source.RemovedChannels)
	_, stillLinked = model.GetModelPriceSyncConfig().AutoProvisionedChannels[transientCh.Id]
	assert.False(t, stillLinked)
}

func TestUpstreamTokenPaginationAndFiniteBalanceResolution(t *testing.T) {
	// 1. Offline unit check for finite exhausted, finite positive, unlimited, and missing token states
	selfBal := upstreamSelfQuotaInfo{Quota: 4_000_000, QuotaPerUnit: 500_000, BalanceUSD: 8.0, HasValidQuota: true}

	bal, ok := resolveUpstreamChannelBalanceUSD(upstreamTokenRecord{UnlimitedQuota: false, RemainQuota: 0}, true, selfBal)
	require.True(t, ok)
	assert.Equal(t, 0.0, bal, "exhausted finite token must report 0 rather than account balance")

	bal, ok = resolveUpstreamChannelBalanceUSD(upstreamTokenRecord{UnlimitedQuota: false, RemainQuota: 500_000}, true, selfBal)
	require.True(t, ok)
	assert.Equal(t, 1.0, bal)

	bal, ok = resolveUpstreamChannelBalanceUSD(upstreamTokenRecord{UnlimitedQuota: true, RemainQuota: 0}, true, selfBal)
	require.True(t, ok)
	assert.Equal(t, 8.0, bal)

	bal, ok = resolveUpstreamChannelBalanceUSD(upstreamTokenRecord{}, false, selfBal)
	assert.False(t, ok, "missing token data must report unknown rather than account balance")
	assert.Equal(t, 0.0, bal)

	// 2. Multi-page (1-indexed p=1..3), duplicate-page, and mid-stream failure pagination + no duplicate token creation
	db := setupModelListControllerTestDB(t)
	prevLogDB := model.LOG_DB
	prevCfg := model.GetModelPriceSyncConfig()
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = prevLogDB
		_ = model.SaveModelPriceSyncConfig(prevCfg)
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Option{}, &model.Ability{}))

	allItems := make([]upstreamTokenRecord, 205)
	for i := range allItems {
		grp := fmt.Sprintf("grp-%03d", i+1)
		remain := int64(500_000)
		if grp == "grp-205" {
			remain = 0 // exhausted finite token on page 3!
		}
		allItems[i] = upstreamTokenRecord{
			ID:             i + 1,
			Name:           "tok-" + grp,
			Key:            "sk-plain-token-key-value-" + grp,
			Status:         1,
			Group:          grp,
			UnlimitedQuota: false,
			RemainQuota:    remain,
		}
	}

	var mode atomic.Int32 // 0 = normal 1-indexed pages, 1 = duplicate page 1, 2 = fail page 2
	var createdGroups []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"success":true,"data":{"grp-001":{"desc":"G1","ratio":1},"grp-205":{"desc":"G205","ratio":1}}}`))
		case "/api/pricing":
			_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"gpt-4o","quota_type":0,"model_ratio":1,"completion_ratio":2,"enable_groups":["grp-001","grp-205"]}],"group_ratio":{"grp-001":1,"grp-205":1},"usable_group":{"grp-001":"G1","grp-205":"G205"}}`))
		case "/api/token/":
			if r.Method == http.MethodPost {
				bodyBytes, _ := io.ReadAll(r.Body)
				var p struct {
					Group string `json:"group"`
				}
				_ = common.Unmarshal(bodyBytes, &p)
				createdGroups = append(createdGroups, p.Group)
				_, _ = w.Write([]byte(`{"success":true}`))
				return
			}
			// Simulate New-API common/page_info.go:41 where p<=1 both return page 1 (1-indexed)
			pNum, _ := strconv.Atoi(r.URL.Query().Get("p"))
			if pNum <= 0 {
				pNum = 1
			}
			curMode := mode.Load()
			if curMode == 1 {
				pNum = 1 // server ignores p and repeats page 1
			} else if curMode == 2 && pNum >= 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			start := (pNum - 1) * 100
			end := min(start+100, len(allItems))
			var slice []upstreamTokenRecord
			if start < len(allItems) {
				slice = allItems[start:end]
			}
			raw, _ := common.Marshal(map[string]any{
				"success": true,
				"data":    map[string]any{"items": slice, "total": len(allItems)},
			})
			_, _ = w.Write(raw)
		case "/api/status":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":4000000,"used_quota":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	// Normal 3-page 1-indexed list finds grp-205 on page 3, creates zero extra tokens, and persists 0 balance for exhausted grp-205
	toks, complete, err := fetchUpstreamTokensList(t.Context(), upstream.URL, "fake-tok", "1", model.UpstreamSyncModeStandard)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Len(t, toks, 205)

	source := model.UpstreamAccountSource{
		ID: "src-paged", Name: "paged", BaseURL: upstream.URL,
		SyncMode: model.UpstreamSyncModeStandard, UserID: "1", AccessToken: "fake-tok", Currency: "USD", Enabled: true,
	}
	cfg := model.GetModelPriceSyncConfig()
	require.NoError(t, syncSingleUpstreamAccountSource(t.Context(), &source, &cfg))
	assert.Empty(t, createdGroups, "must find grp-205 on page 3 without creating duplicate tokens")

	var ch205 model.Channel
	require.NoError(t, db.Where("name = ?", "paged-grp-205-1").First(&ch205).Error)
	assert.Equal(t, 0.0, ch205.Balance, "persisted exhausted finite token channel must read back 0 USD")

	// Duplicate page and mid-stream failure mark incomplete and do NOT create duplicate tokens for grp-205
	for _, m := range []int32{1, 2} {
		mode.Store(m)
		_, comp, fErr := fetchUpstreamTokensList(t.Context(), upstream.URL, "fake-tok", "1", model.UpstreamSyncModeStandard)
		require.NoError(t, fErr)
		assert.False(t, comp)

		cfg = model.GetModelPriceSyncConfig()
		syncErr := syncSingleUpstreamAccountSource(t.Context(), &source, &cfg)
		require.Error(t, syncErr)
		assert.Empty(t, createdGroups, "incomplete token pagination must not create duplicate tokens")
	}
}

func TestGetUserCostAnalysisSeparatesUnloggedHistoricalQuotaFromCoveredProfit(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	prevLogDB := model.LOG_DB
	prevQuotaUnit := common.QuotaPerUnit
	model.LOG_DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		model.LOG_DB = prevLogDB
		common.QuotaPerUnit = prevQuotaUnit
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.TopUp{}, &model.Option{}, &model.Ability{}))

	// User 1: historical UsedQuota = $20 (10_000_000 quota), but ZERO consume logs -> insufficient data, 0 profit
	require.NoError(t, db.Create(&model.User{
		Id: 101, Username: "unlogged-user", AffCode: "aff101", Group: "default", Status: 1, Quota: 0, UsedQuota: 10_000_000,
	}).Error)
	// User 2: complete logs ($10 revenue = 5_000_000 quota, frozen upstream cost = $4) -> $6 profit (60%)
	require.NoError(t, db.Create(&model.User{
		Id: 102, Username: "covered-user", AffCode: "aff102", Group: "default", Status: 1, Quota: 0, UsedQuota: 5_000_000,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: 102, Username: "covered-user", Type: model.LogTypeConsume, ModelName: "gpt-4o",
		Quota: 5_000_000, PromptTokens: 1000, CompletionTokens: 1000, ChannelId: 1,
		Other: `{"upstream_cost_usd":4}`, CreatedAt: 1700000000,
	}).Error)
	// User 3: authoritative zero-cost logs ($5 revenue = 2_500_000 quota, frozen upstream cost = $0) -> $5 profit (100%)
	require.NoError(t, db.Create(&model.User{
		Id: 103, Username: "zerocost-user", AffCode: "aff103", Group: "default", Status: 1, Quota: 0, UsedQuota: 2_500_000,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: 103, Username: "zerocost-user", Type: model.LogTypeConsume, ModelName: "free-model",
		Quota: 2_500_000, PromptTokens: 500, CompletionTokens: 500, ChannelId: 1,
		Other: `{"upstream_cost_usd":0}`, CreatedAt: 1700000000,
	}).Error)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/cost_management/users", nil)
	GetUserCostAnalysis(c)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Users   []UserCostAnalysisRow `json:"users"`
			Summary struct {
				TotalConsumedUSD    float64 `json:"total_consumed_usd"`
				CoveredRevenueUSD   float64 `json:"covered_revenue_usd"`
				UncoveredRevenueUSD float64 `json:"uncovered_revenue_usd"`
				TotalCostUSD        float64 `json:"total_cost_usd"`
				TotalProfitUSD      float64 `json:"total_profit_usd"`
				OverallMarginPct    float64 `json:"overall_margin_pct"`
				CostDataStatus      string  `json:"cost_data_status"`
			} `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	byUser := make(map[int]UserCostAnalysisRow, len(resp.Data.Users))
	for _, row := range resp.Data.Users {
		byUser[row.UserID] = row
	}

	assert.Equal(t, "insufficient", byUser[101].CostDataStatus)
	assert.InDelta(t, 20.0, byUser[101].ConsumedUSD, 1e-6)
	assert.InDelta(t, 0.0, byUser[101].CoveredRevenueUSD, 1e-6)
	assert.InDelta(t, 20.0, byUser[101].UncoveredRevenueUSD, 1e-6)
	assert.InDelta(t, 0.0, byUser[101].GrossProfitUSD, 1e-6)
	assert.InDelta(t, 0.0, byUser[101].GrossMarginPct, 1e-6)

	assert.Equal(t, "complete", byUser[102].CostDataStatus)
	assert.InDelta(t, 6.0, byUser[102].GrossProfitUSD, 1e-6)
	assert.InDelta(t, 60.0, byUser[102].GrossMarginPct, 1e-6)

	assert.Equal(t, "complete", byUser[103].CostDataStatus)
	assert.InDelta(t, 5.0, byUser[103].GrossProfitUSD, 1e-6)
	assert.InDelta(t, 100.0, byUser[103].GrossMarginPct, 1e-6, "genuine zero-cost logs must still display 100%% margin")

	assert.Equal(t, "partial", resp.Data.Summary.CostDataStatus)
	assert.InDelta(t, 35.0, resp.Data.Summary.TotalConsumedUSD, 1e-6)
	assert.InDelta(t, 15.0, resp.Data.Summary.CoveredRevenueUSD, 1e-6)
	assert.InDelta(t, 20.0, resp.Data.Summary.UncoveredRevenueUSD, 1e-6)
	assert.InDelta(t, 4.0, resp.Data.Summary.TotalCostUSD, 1e-6)
	assert.InDelta(t, 11.0, resp.Data.Summary.TotalProfitUSD, 1e-6, "unlogged historical consumption must not inflate total gross profit")
}
