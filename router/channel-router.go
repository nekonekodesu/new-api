package router

import (
	"net/http"
	"path"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

type permissionRoute struct {
	method     string
	path       string
	permission authz.Permission
	handler    gin.HandlerFunc
}

// handlePermissionRoute registers a Casbin-guarded route and declares the same
// permission as the access token scope that route requires.
func handlePermissionRoute(group *gin.RouterGroup, method, relativePath string, permission authz.Permission, handlers ...gin.HandlerFunc) {
	middleware.DeclareAccessTokenPermissionRoute(method, joinPaths(group.BasePath(), relativePath), permission)
	group.Handle(method, relativePath, append([]gin.HandlerFunc{middleware.RequirePermission(permission)}, handlers...)...)
}

// joinPaths matches gin's route path joining, which keeps a trailing slash
// from the relative path, so declared keys equal c.FullPath().
func joinPaths(absolutePath, relativePath string) string {
	if relativePath == "" {
		return absolutePath
	}
	finalPath := path.Join(absolutePath, relativePath)
	if relativePath[len(relativePath)-1] == '/' && finalPath[len(finalPath)-1] != '/' {
		return finalPath + "/"
	}
	return finalPath
}

func registerChannelRoutes(apiRouter *gin.RouterGroup) {
	channelRoute := apiRouter.Group("/channel")
	channelRoute.Use(middleware.AdminAuth())

	channelRoute.POST("/:id/key",
		middleware.RootAuth(),
		middleware.CriticalRateLimit(),
		middleware.DisableCache(),
		middleware.SecureVerificationRequired(),
		controller.GetChannelKey,
	)

	for _, route := range channelPermissionRoutes {
		handlePermissionRoute(channelRoute, route.method, route.path, route.permission, route.handler)
	}
}

var channelPermissionRoutes = []permissionRoute{
	{method: http.MethodGet, path: "/priority/order", permission: authz.ChannelRead, handler: controller.GetChannelPriorityOrder},
	{method: http.MethodPost, path: "/priority/reorder", permission: authz.ChannelWrite, handler: controller.ReorderChannelPriority},
	{method: http.MethodGet, path: "/", permission: authz.ChannelRead, handler: controller.GetAllChannels},
	{method: http.MethodGet, path: "/search", permission: authz.ChannelRead, handler: controller.SearchChannels},
	{method: http.MethodGet, path: "/models", permission: authz.ChannelRead, handler: controller.ChannelListModels},
	{method: http.MethodGet, path: "/default_base_urls", permission: authz.ChannelRead, handler: controller.GetChannelDefaultBaseURLs},
	{method: http.MethodGet, path: "/models_enabled", permission: authz.ChannelRead, handler: controller.EnabledListModels},
	{method: http.MethodGet, path: "/ops", permission: authz.ChannelRead, handler: controller.GetChannelOps},
	{method: http.MethodGet, path: "/:id", permission: authz.ChannelRead, handler: controller.GetChannel},
	{method: http.MethodGet, path: "/test", permission: authz.ChannelOperate, handler: controller.TestAllChannels},
	{method: http.MethodGet, path: "/test/:id", permission: authz.ChannelOperate, handler: controller.TestChannel},
	{method: http.MethodGet, path: "/update_balance", permission: authz.ChannelOperate, handler: controller.UpdateAllChannelsBalance},
	{method: http.MethodGet, path: "/update_balance/:id", permission: authz.ChannelOperate, handler: controller.UpdateChannelBalance},
	{method: http.MethodGet, path: "/update_balance/task/:task_id", permission: authz.ChannelOperate, handler: controller.GetChannelBalanceTask},
	{method: http.MethodPost, path: "/", permission: authz.ChannelSensitiveWrite, handler: controller.AddChannel},
	{method: http.MethodPut, path: "/", permission: authz.ChannelWrite, handler: controller.UpdateChannel},
	{method: http.MethodPost, path: "/status/batch", permission: authz.ChannelOperate, handler: controller.BatchUpdateChannelStatus},
	{method: http.MethodPost, path: "/:id/status", permission: authz.ChannelOperate, handler: controller.UpdateChannelStatus},
	{method: http.MethodDelete, path: "/disabled", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteDisabledChannel},
	{method: http.MethodPost, path: "/tag/disabled", permission: authz.ChannelOperate, handler: controller.DisableTagChannels},
	{method: http.MethodPost, path: "/tag/enabled", permission: authz.ChannelOperate, handler: controller.EnableTagChannels},
	{method: http.MethodPut, path: "/tag", permission: authz.ChannelWrite, handler: controller.EditTagChannels},
	{method: http.MethodDelete, path: "/:id", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteChannel},
	{method: http.MethodPost, path: "/batch", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteChannelBatch},
	{method: http.MethodPost, path: "/fix", permission: authz.ChannelOperate, handler: controller.FixChannelsAbilities},
	{method: http.MethodGet, path: "/fetch_models/:id", permission: authz.ChannelOperate, handler: controller.FetchUpstreamModels},
	{method: http.MethodGet, path: "/:id/vllm/status", permission: authz.ChannelRead, handler: controller.GetVLLMChannelStatus},
	{method: http.MethodGet, path: "/:id/sglang/status", permission: authz.ChannelRead, handler: controller.GetSGLangChannelStatus},
	{method: http.MethodPost, path: "/fetch_models", permission: authz.ChannelSensitiveWrite, handler: controller.FetchModels},
	{method: http.MethodPost, path: "/:id/codex/refresh", permission: authz.ChannelSensitiveWrite, handler: controller.RefreshCodexChannelCredential},
	{method: http.MethodPost, path: "/:id/plugin_login", permission: authz.ChannelSensitiveWrite, handler: controller.StartPluginLogin},
	{method: http.MethodPost, path: "/:id/plugin_login/continue", permission: authz.ChannelSensitiveWrite, handler: controller.ContinuePluginLogin},
	{method: http.MethodPost, path: "/plugin_login/callback", permission: authz.ChannelSensitiveWrite, handler: controller.CompletePluginLoginCallback},
	{method: http.MethodGet, path: "/:id/plugin_credentials", permission: authz.ChannelRead, handler: controller.GetPluginCredentials},
	{method: http.MethodGet, path: "/:id/codex/usage", permission: authz.ChannelRead, handler: controller.GetCodexChannelUsage},
	{method: http.MethodGet, path: "/:id/codex/usage/reset-credits", permission: authz.ChannelRead, handler: controller.GetCodexChannelRateLimitResetCredits},
	{method: http.MethodPost, path: "/:id/codex/usage/reset", permission: authz.ChannelOperate, handler: controller.ResetCodexChannelUsage},
	{method: http.MethodPost, path: "/ollama/pull", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaPullModel},
	{method: http.MethodPost, path: "/ollama/pull/stream", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaPullModelStream},
	{method: http.MethodDelete, path: "/ollama/delete", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaDeleteModel},
	{method: http.MethodGet, path: "/ollama/version/:id", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaVersion},
	{method: http.MethodPost, path: "/batch/tag", permission: authz.ChannelWrite, handler: controller.BatchSetChannelTag},
	{method: http.MethodGet, path: "/tag/models", permission: authz.ChannelRead, handler: controller.GetTagModels},
	{method: http.MethodPost, path: "/copy/:id", permission: authz.ChannelSensitiveWrite, handler: controller.CopyChannel},
	{method: http.MethodPost, path: "/multi_key/manage", permission: authz.ChannelOperate, handler: controller.ManageMultiKeys},
	{method: http.MethodPost, path: "/upstream_updates/apply", permission: authz.ChannelWrite, handler: controller.ApplyChannelUpstreamModelUpdates},
	{method: http.MethodPost, path: "/upstream_updates/apply_all", permission: authz.ChannelWrite, handler: controller.ApplyAllChannelUpstreamModelUpdates},
	{method: http.MethodPost, path: "/upstream_updates/detect", permission: authz.ChannelOperate, handler: controller.DetectChannelUpstreamModelUpdates},
	{method: http.MethodPost, path: "/upstream_updates/detect_all", permission: authz.ChannelOperate, handler: controller.DetectAllChannelUpstreamModelUpdates},
	{method: http.MethodGet, path: "/group_scheduling", permission: authz.ChannelRead, handler: controller.GetGroupScheduling},
	{method: http.MethodPut, path: "/group_scheduling", permission: authz.ChannelWrite, handler: controller.UpdateGroupScheduling},
	{method: http.MethodGet, path: "/model_price_table", permission: authz.ChannelRead, handler: controller.GetModelPriceTable},
	{method: http.MethodPost, path: "/model_price_table/apply", permission: authz.ChannelWrite, handler: controller.ApplyModelPriceTable},
	{method: http.MethodGet, path: "/cron_sync_config", permission: authz.ChannelRead, handler: controller.GetCronSyncConfig},
	{method: http.MethodPut, path: "/cron_sync_config", permission: authz.ChannelWrite, handler: controller.UpdateCronSyncConfig},
	{method: http.MethodPost, path: "/cron_sync_run", permission: authz.ChannelOperate, handler: controller.RunCronSyncNow},
	{method: http.MethodGet, path: "/cron_jobs", permission: authz.ChannelRead, handler: controller.GetCronJobs},
	{method: http.MethodPost, path: "/cron_jobs", permission: authz.ChannelSensitiveWrite, handler: controller.SaveCronJob},
	{method: http.MethodPut, path: "/cron_jobs", permission: authz.ChannelSensitiveWrite, handler: controller.SaveCronJob},
	{method: http.MethodDelete, path: "/cron_jobs/:id", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteCronJob},
	{method: http.MethodPost, path: "/cron_jobs/:id/run", permission: authz.ChannelOperate, handler: controller.RunCronJobNow},
	{method: http.MethodGet, path: "/upstream_accounts", permission: authz.ChannelRead, handler: controller.GetUpstreamAccounts},
	{method: http.MethodPost, path: "/upstream_accounts", permission: authz.ChannelSensitiveWrite, handler: controller.SaveUpstreamAccount},
	{method: http.MethodPut, path: "/upstream_accounts", permission: authz.ChannelSensitiveWrite, handler: controller.SaveUpstreamAccount},
	{method: http.MethodDelete, path: "/upstream_accounts/:id", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteUpstreamAccount},
	{method: http.MethodPost, path: "/upstream_accounts/sync", permission: authz.ChannelOperate, handler: controller.SyncUpstreamAccountsNow},
	{method: http.MethodGet, path: "/classifier", permission: authz.ChannelRead, handler: controller.GetChannelClassifier},
	{method: http.MethodPost, path: "/classifier", permission: authz.ChannelWrite, handler: controller.SaveChannelClassifier},
	{method: http.MethodPut, path: "/classifier", permission: authz.ChannelWrite, handler: controller.SaveChannelClassifier},
	{method: http.MethodDelete, path: "/classifier/:id", permission: authz.ChannelWrite, handler: controller.DeleteChannelClassifierRule},
	{method: http.MethodPost, path: "/classifier/apply", permission: authz.ChannelOperate, handler: controller.ApplyChannelClassifierNow},
	{method: http.MethodGet, path: "/cost_management/models", permission: authz.ChannelRead, handler: controller.GetModelCostAnalysis},
	{method: http.MethodGet, path: "/cost_management/users", permission: authz.ChannelRead, handler: controller.GetUserCostAnalysis},
	{method: http.MethodGet, path: "/model_health", permission: authz.ChannelRead, handler: controller.GetModelHealthAnalysis},
}
