package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSystemTaskListFiltersAndPaginationResponse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}))
	require.NoError(t, db.Create(&[]model.SystemTask{
		{TaskID: "older", Type: model.SystemTaskTypeModelUpdate, Status: model.SystemTaskStatusFailed},
		{TaskID: "newer", Type: model.SystemTaskTypeModelUpdate, Status: model.SystemTaskStatusFailed},
		{TaskID: "other-status", Type: model.SystemTaskTypeModelUpdate, Status: model.SystemTaskStatusSucceeded},
		{TaskID: "other-type", Type: model.SystemTaskTypeChannelTest, Status: model.SystemTaskStatusFailed},
	}).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/system-task/list?scope=history&type=model_update&status=failed&offset=1&limit=1", nil)
	ListSystemTasks(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                       `json:"success"`
		Total   int64                      `json:"total"`
		Data    []model.SystemTaskResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.EqualValues(t, 2, response.Total)
	require.Len(t, response.Data, 1)
	assert.Equal(t, "older", response.Data[0].TaskID)
}

func TestSystemTaskInvalidFiltersAreRejected(t *testing.T) {
	for _, tc := range []struct {
		query   string
		handler gin.HandlerFunc
	}{
		{"?scope=invalid", ListSystemTasks},
		{"?status=unknown", ListSystemTasks},
		{"?offset=-1", ListSystemTasks},
		{"?limit=invalid", ListSystemTasks},
		{"?status=unknown", DeleteSystemTaskHistory},
		{"?scope=invalid", DeleteSystemTaskHistory},
	} {
		t.Run(tc.query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/"+tc.query, nil)
			tc.handler(c)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.JSONEq(t, `{"success":false,"message":"invalid system task filters"}`, recorder.Body.String())
		})
	}
}

func TestUnifiedCronAndSystemTaskDeduplication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	origAutoTest := operation_setting.GetMonitorSetting().AutoTestChannelEnabled
	origJobs := model.GetCronJobs()
	t.Cleanup(func() {
		operation_setting.GetMonitorSetting().AutoTestChannelEnabled = origAutoTest
		_ = model.SaveCronJobs(origJobs)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.SystemTask{}, &model.SystemTaskLock{}))

	operation_setting.GetMonitorSetting().AutoTestChannelEnabled = true
	assert.True(t, channelTestHandler{}.Enabled())

	// Enabling builtin-channel-test and builtin-upstream-sync in CronJobs makes the fixed-interval
	// SystemTaskRunner handlers yield so the same job is never scheduled twice.
	jobs := model.GetCronJobs()
	for i := range jobs {
		if jobs[i].ID == model.BuiltInChannelTestCronJobID || jobs[i].ID == model.BuiltInUpstreamSyncCronJobID {
			jobs[i].Enabled = true
		}
	}
	require.NoError(t, model.SaveCronJobs(jobs))
	assert.False(t, channelTestHandler{}.Enabled(), "channelTestHandler must yield when builtin-channel-test is enabled in CronJobs")
	assert.False(t, modelUpdateHandler{}.Enabled(), "modelUpdateHandler must yield when builtin-upstream-sync is enabled in CronJobs")

	// SyncChannelCurrencyCode keeps Channel.CurrencyCode and ModelPriceSyncConfig.ChannelCurrencies aligned.
	cny := "cny"
	model.SyncChannelCurrencyCode(42, &cny)
	assert.Equal(t, "CNY", model.GetModelPriceSyncConfig().ChannelCurrencies[42])
	model.SyncChannelCurrencyCode(42, nil)
	assert.Empty(t, model.GetModelPriceSyncConfig().ChannelCurrencies[42])
}
