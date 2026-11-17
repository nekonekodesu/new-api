package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelSwitchesGroupWhenEveryChannelWasTried(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	common.RetryTimes = 5
	const modelName = "auto-groups-exhausted-model"
	createChannelSelectAutoGroupsChannel(t, db, 2301, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2302, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	retry := 0
	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: modelName, Retry: &retry}

	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2301, first.Id)
	AppendUsedChannel(ctx, first.Id)

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2302, second.Id, "a retry budget larger than the group must not keep retrying its only channel")
	assert.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelIgnoresCooldownOncePerRequest(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "cooldown-bypass-model"
	createChannelSelectAutoGroupsChannel(t, db, 2401, "default", modelName)
	model.InitChannelCache()
	enabled, window, threshold, seconds := common.RetryCooldownEnabled, common.RetryCooldownWindowSeconds, common.RetryCooldownFailureThreshold, common.RetryCooldownSeconds
	common.RetryCooldownEnabled, common.RetryCooldownWindowSeconds, common.RetryCooldownFailureThreshold, common.RetryCooldownSeconds = true, 10, 1, 30
	model.ResetChannelCooldowns()
	t.Cleanup(func() {
		common.RetryCooldownEnabled, common.RetryCooldownWindowSeconds, common.RetryCooldownFailureThreshold, common.RetryCooldownSeconds = enabled, window, threshold, seconds
		model.ResetChannelCooldowns()
	})
	require.True(t, model.RecordChannelCooldownFailure(2401, modelName))

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, Retry: &retry}

	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first, "the first pass that finds only cooling channels routes again ignoring the cooldown")
	assert.Equal(t, 2401, first.Id)
	AppendUsedChannel(ctx, first.Id)

	param.IncreaseRetry()
	second, _, err := CacheGetRandomSatisfiedChannel(param)
	assert.ErrorIs(t, err, ErrChannelsCoolingDown)
	assert.Nil(t, second)
}

func TestChannelCooldownFinalRoundVisitsEveryCandidate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		auto       bool
		crossGroup bool
		want       []int
	}{
		{name: "single group", want: []int{2501, 2502, 2503}},
		{name: "auto cross group", auto: true, crossGroup: true, want: []int{2501, 2502, 2503}},
		{name: "auto same group", auto: true, want: []int{2501, 2502}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			common.RetryTimes = 0
			const name = "cooldown-round"
			createChannelSelectAutoGroupsChannel(t, db, 2501, "default", name)
			createChannelSelectAutoGroupsChannel(t, db, 2502, "default", name)
			group := "default"
			if tc.auto {
				group = "vip"
			}
			createChannelSelectAutoGroupsChannel(t, db, 2503, group, name)
			model.InitChannelCache()
			enabled, threshold := common.RetryCooldownEnabled, common.RetryCooldownFailureThreshold
			common.RetryCooldownEnabled, common.RetryCooldownFailureThreshold = true, 1
			model.ResetChannelCooldowns()
			t.Cleanup(func() {
				common.RetryCooldownEnabled, common.RetryCooldownFailureThreshold = enabled, threshold
				model.ResetChannelCooldowns()
			})
			for _, id := range []int{2501, 2502, 2503} {
				require.True(t, model.RecordChannelCooldownFailure(id, name))
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: name}
			if tc.auto {
				param.TokenGroup = "auto"
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
				common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, tc.crossGroup)
			}
			var visited []int
			for range len(tc.want) {
				require.True(t, param.CanAttempt(), "the final round must finish even with no ordinary retries")
				channel, _, err := CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				require.NotNil(t, channel)
				visited = append(visited, channel.Id)
				AppendUsedChannel(ctx, channel.Id)
				require.Positive(t, param.RemainingRetries())
				param.IncreaseRetry()
			}
			assert.ElementsMatch(t, tc.want, visited)
			require.True(t, param.CanAttempt(), "selection must report final-round exhaustion")
			channel, _, err := CacheGetRandomSatisfiedChannel(param)
			assert.ErrorIs(t, err, ErrChannelsCoolingDown)
			assert.Nil(t, channel)
		})
	}
}

func TestChannelSelectionDoesNotReuseAffinityAfterRetryCounterReset(t *testing.T) {
	for _, finalRound := range []bool{false, true} {
		t.Run(fmt.Sprintf("final_round=%t", finalRound), func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			const name = "affinity-retry-model"
			createChannelSelectAutoGroupsChannel(t, db, 2601, "default", name)
			createChannelSelectAutoGroupsChannel(t, db, 2602, "vip", name)
			model.InitChannelCache()
			affinity := operation_setting.GetChannelAffinitySetting()
			previous := *affinity
			t.Cleanup(func() { *affinity = previous })
			rule := operation_setting.ChannelAffinityRule{
				Name: t.Name(), ModelRegex: []string{"^affinity-retry-model$"}, SessionMode: "prefer",
				KeySources:      []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
				IncludeRuleName: true,
			}
			affinity.Enabled, affinity.Rules = true, []operation_setting.ChannelAffinityRule{rule}
			key := buildChannelAffinityCacheKeySuffix(rule, name, "auto", t.Name())
			cache := getChannelAffinityCache()
			require.NoError(t, cache.SetWithTTL(key, 2601, time.Minute))
			t.Cleanup(func() { _, err := cache.DeleteMany([]string{key}); assert.NoError(t, err) })
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			ctx.Request.Header.Set("X-Session", t.Name())
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
			common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
			param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: name}
			first, _, selectErr := SelectChannelForRequest(ctx, name, param)
			require.Nil(t, selectErr)
			require.NotNil(t, first)
			require.Equal(t, 2601, first.Id)
			AppendUsedChannel(ctx, first.Id)
			// Moving to a new auto group resets its retry budget to zero. A
			// final round must also ignore a recovered, already-tried binding.
			common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, 1)
			common.SetContextKey(ctx, constant.ContextKeyChannelCooldownBypassed, finalRound)
			if finalRound {
				ctx.Set("channel_cooldown_round_tried", []int{2601})
			}
			second, group, selectErr := SelectChannelForRequest(ctx, name, param)
			require.Nil(t, selectErr)
			require.NotNil(t, second)
			assert.Equal(t, 2602, second.Id)
			assert.Equal(t, "vip", group)
		})
	}
}
