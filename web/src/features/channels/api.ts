/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { getGroups as getUserGroups } from '@/features/users/api'
import { api, type ApiRequestConfig } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { PriorityMove, PriorityOrder } from './lib/channel-priority-order'
import type { InferenceStatus } from './lib/inference-status'
import type {
  PluginCredentialSummary,
  PluginLoginInput,
  PluginLoginStep,
} from './lib/plugin-login'
import type {
  AddChannelRequest,
  BatchDeleteParams,
  BatchSetTagParams,
  Channel,
  ChannelBalanceResponse,
  ChannelOpsResponse,
  ChannelTestResponse,
  CopyChannelParams,
  CopyChannelResponse,
  FetchModelsResponse,
  GetChannelResponse,
  GetChannelsParams,
  GetChannelsResponse,
  MultiKeyManageParams,
  MultiKeyStatusResponse,
  SearchChannelsParams,
  SearchChannelsResponse,
  TagOperationParams,
} from './types'

const channelActionConfig = (
  config: ApiRequestConfig = {}
): ApiRequestConfig => ({
  ...config,
  skipBusinessError: true,
  skipErrorHandler: true,
})

export async function getChannelPriorityOrder(): Promise<PriorityOrder> {
  const response = await api.get<{ success: boolean; data: PriorityOrder }>(
    '/api/channel/priority/order',
    { disableDuplicate: true }
  )
  return requireServerSuccess(response.data).data
}

export async function reorderChannelPriority(
  move: PriorityMove
): Promise<PriorityOrder> {
  const response = await api.post<{ success: boolean; data: PriorityOrder }>(
    '/api/channel/priority/reorder',
    move
  )
  return requireServerSuccess(response.data).data
}

export async function getInferenceStatus(
  channelId: number,
  provider: 'vllm' | 'sglang',
  signal?: AbortSignal
): Promise<InferenceStatus> {
  const response = await api.get<{ success: boolean; data: InferenceStatus }>(
    `/api/channel/${channelId}/${provider}/status`,
    { signal, disableDuplicate: true }
  )
  return requireServerSuccess(response.data).data
}

export type TaskPluginOption = {
  sortPriority?: number
  website?: string
  key: string
  name: string
  description?: Record<string, string> | null
  icon?: string
  hasIcon?: boolean
  baseUrl?: string
  models: string[]
  channelTypes?: number[] | null
  upstreams?: string[] | null
  task?: boolean
  provider?: boolean
  login?: boolean
  refresh?: boolean
  transform?: boolean
  transform_hooks?: string[] | null
  optional?: boolean
  balance?: boolean
  responsesWebsocket?: boolean
  channelConfig?: {
    type?: string
    properties?: Record<string, unknown>
    [key: string]: unknown
  } | null
  configSchema?: import('@/features/task-plugins/types').RequestPluginConfigSchema | null
  keyHint?: string | Record<string, string>
  name_i18n?: Record<string, string>
  relayPaths?: string[]
  samples?: import('@/features/task-plugins/types').RequestPluginSample[]
}

export async function getTaskPluginOptions(): Promise<TaskPluginOption[]> {
  const response = await api.get<{
    success: boolean
    data: TaskPluginOption[] | null
  }>('/api/task_plugin_options')
  const items = requireServerSuccess(response.data).data
  if (!Array.isArray(items)) return []
  return items.map((item) => ({
    ...item,
    models: Array.isArray(item.models) ? item.models : [],
    channelTypes: Array.isArray(item.channelTypes) ? item.channelTypes : [],
    upstreams: Array.isArray(item.upstreams) ? item.upstreams : [],
    samples: Array.isArray(item.samples) ? item.samples : [],
  }))
}

export async function startPluginLogin(
  channelId: number,
  keyIndex: number | null,
  add = false
): Promise<{ flow: string; step: PluginLoginStep }> {
  const response = await api.post<{ success: boolean; data: { flow: string; step: PluginLoginStep } }>(
    `/api/channel/${channelId}/plugin_login`,
    add ? { add: true } : { key_index: keyIndex },
    channelActionConfig()
  )
  return requireServerSuccess(response.data).data
}

export async function continuePluginLogin(
  channelId: number,
  flow: string,
  input: PluginLoginInput
): Promise<PluginLoginStep> {
  const response = await api.post<{ success: boolean; data: { step: PluginLoginStep } }>(
    `/api/channel/${channelId}/plugin_login/continue`,
    { flow, input },
    channelActionConfig()
  )
  return requireServerSuccess(response.data).data.step
}

export async function submitPluginLoginCallback(
  plugin: string,
  params: Record<string, string>
): Promise<{ channel_id: number; step: PluginLoginStep }> {
  const response = await api.post<{ success: boolean; data: { channel_id: number; step: PluginLoginStep } }>(
    '/api/channel/plugin_login/callback',
    { plugin, params },
    channelActionConfig()
  )
  return requireServerSuccess(response.data).data
}

export async function getPluginCredentials(
  channelId: number
): Promise<PluginCredentialSummary[]> {
  const response = await api.get<{ success: boolean; data: { items: PluginCredentialSummary[] | null } }>(
    `/api/channel/${channelId}/plugin_credentials`,
    channelActionConfig()
  )
  return requireServerSuccess(response.data).data.items ?? []
}

export type CodexUsageResponse = {
  success: boolean
  message?: string
  upstream_status?: number
  data?: Record<string, unknown>
}

export type CodexResetCreditsResponse = CodexUsageResponse

export type CodexUsageResetResponse = CodexUsageResponse

export type CodexCredentialRefreshResponse = {
  success: boolean
  message?: string
  data?: {
    expires_at?: string
    last_refresh?: string
    account_id?: string
    email?: string
    channel_id?: number
    channel_type?: number
    channel_name?: string
  }
}

// ============================================================================
// Base Channel CRUD Operations
// ============================================================================

/**
 * Get paginated list of channels
 */
export async function getChannels(
  params: GetChannelsParams = {}
): Promise<GetChannelsResponse> {
  const res = await api.get('/api/channel', { params })
  return res.data
}

/**
 * Search channels with filters
 */
export async function searchChannels(
  params: SearchChannelsParams
): Promise<SearchChannelsResponse> {
  const res = await api.get('/api/channel/search', { params })
  return res.data
}

/**
 * Get single channel by ID
 */
export async function getChannel(id: number): Promise<GetChannelResponse> {
  const res = await api.get(`/api/channel/${id}`)
  return res.data
}

/**
 * Get channel operations summary for administrators
 */
export async function getChannelOps(
  autoBan?: boolean
): Promise<ChannelOpsResponse> {
  const res = await api.get<ChannelOpsResponse>('/api/channel/ops', {
    ...channelActionConfig(),
    params: autoBan === undefined ? undefined : { auto_ban: autoBan },
  })
  return res.data
}

export async function getChannelDefaultBaseURLs(): Promise<
  Partial<Record<number, string>>
> {
  const response = await api.get<{
    success: boolean
    data: Partial<Record<number, string>>
  }>('/api/channel/default_base_urls')
  return requireServerSuccess(response.data).data
}

/**
 * Create new channel(s)
 * Supports single, batch, and multi-key modes
 */
export async function createChannel(
  data: AddChannelRequest
): Promise<{
  success: boolean
  message?: string
  data?: { ids?: number[] }
}> {
  const res = await api.post('/api/channel', data, channelActionConfig())
  return res.data
}

/**
 * Update existing channel
 */
export async function updateChannel(
  id: number,
  data: Partial<Channel>
): Promise<{ success: boolean; message?: string; data?: Channel }> {
  const res = await api.put(
    '/api/channel/',
    { id, ...data },
    channelActionConfig()
  )
  return res.data
}

/**
 * Update channel enabled/disabled status.
 */
export async function updateChannelStatus(
  id: number,
  status: number
): Promise<{ success: boolean; message?: string; data?: boolean }> {
  const res = await api.post(
    `/api/channel/${id}/status`,
    { status },
    channelActionConfig()
  )
  return res.data
}

/**
 * Batch update channel enabled/disabled status.
 */
export async function batchUpdateChannelStatus(
  ids: number[],
  status: number
): Promise<{ success: boolean; message?: string; data?: number }> {
  const res = await api.post(
    '/api/channel/status/batch',
    { ids, status },
    channelActionConfig()
  )
  return res.data
}

/**
 * Delete single channel
 */
export async function deleteChannel(
  id: number
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(`/api/channel/${id}`, channelActionConfig())
  return res.data
}

/**
 * Batch delete channels
 */
export async function batchDeleteChannels(
  data: BatchDeleteParams
): Promise<{ success: boolean; message?: string; data?: number }> {
  const res = await api.post('/api/channel/batch', data, channelActionConfig())
  return res.data
}

/**
 * Batch set tag for channels
 */
export async function batchSetChannelTag(
  data: BatchSetTagParams
): Promise<{ success: boolean; message?: string; data?: number }> {
  const res = await api.post(
    '/api/channel/batch/tag',
    data,
    channelActionConfig()
  )
  return res.data
}

// ============================================================================
// Channel Operations
// ============================================================================

/**
 * Test channel connectivity
 */
export async function testChannel(
  id: number,
  params?: { model?: string; endpoint_type?: string; stream?: boolean }
): Promise<ChannelTestResponse> {
  const res = await api.get(
    `/api/channel/test/${id}`,
    channelActionConfig({ params })
  )
  return res.data
}

/**
 * Update channel balance
 */
export async function updateChannelBalance(
  id: number
): Promise<ChannelBalanceResponse> {
  const res = await api.get(
    `/api/channel/update_balance/${id}`,
    channelActionConfig()
  )
  return res.data
}

/**
 * Fetch available models from upstream provider
 */
export async function fetchUpstreamModels(
  id: number
): Promise<FetchModelsResponse> {
  const res = await api.get(
    `/api/channel/fetch_models/${id}`,
    channelActionConfig()
  )
  return res.data
}

/**
 * Copy/clone a channel
 */
export async function copyChannel(
  id: number,
  params: CopyChannelParams = {}
): Promise<CopyChannelResponse> {
  const res = await api.post(
    `/api/channel/copy/${id}`,
    null,
    channelActionConfig({ params })
  )
  return res.data
}

/**
 * Fix channel abilities
 */
export async function fixChannelAbilities(): Promise<{
  success: boolean
  message?: string
  data?: { success: number; fails: number }
}> {
  const res = await api.post(
    '/api/channel/fix',
    undefined,
    channelActionConfig()
  )
  return res.data
}

/**
 * Delete all disabled channels
 */
export async function deleteDisabledChannels(): Promise<{
  success: boolean
  message?: string
  data?: number
}> {
  const res = await api.delete('/api/channel/disabled', channelActionConfig())
  return res.data
}

/**
 * Get channel key (requires 2FA verification)
 */
export async function getChannelKey(
  id: number,
  proofToken: string,
  signal?: AbortSignal
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const res = await api.post(
    `/api/channel/${id}/key`,
    undefined,
    channelActionConfig({
      headers: { 'X-Security-Proof': proofToken },
      signal,
    })
  )
  return res.data
}

// ============================================================================
// Codex Channel Operations
// ============================================================================

export async function refreshCodexCredential(
  channelId: number
): Promise<CodexCredentialRefreshResponse> {
  const res = await api.post(
    `/api/channel/${channelId}/codex/refresh`,
    {},
    channelActionConfig()
  )
  return res.data
}

export async function getCodexUsage(
  channelId: number
): Promise<CodexUsageResponse> {
  const res = await api.get(
    `/api/channel/${channelId}/codex/usage`,
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function getCodexResetCredits(
  channelId: number
): Promise<CodexResetCreditsResponse> {
  const res = await api.get(
    `/api/channel/${channelId}/codex/usage/reset-credits`,
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function resetCodexUsage(
  channelId: number
): Promise<CodexUsageResetResponse> {
  const res = await api.post(
    `/api/channel/${channelId}/codex/usage/reset`,
    {},
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

// ============================================================================
// Multi-Key Management
// ============================================================================

/**
 * Manage multi-key channel operations
 */
export async function manageMultiKeys(
  params: MultiKeyManageParams
): Promise<MultiKeyStatusResponse | { success: boolean; message?: string }> {
  const res = await api.post(
    '/api/channel/multi_key/manage',
    params,
    channelActionConfig()
  )
  return res.data
}

/**
 * Get key status for multi-key channel
 */
export async function getMultiKeyStatus(
  channelId: number,
  page = 1,
  pageSize = 50,
  status?: number
): Promise<MultiKeyStatusResponse> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'get_key_status',
    page,
    page_size: pageSize,
    status,
  }) as Promise<MultiKeyStatusResponse>
}

/**
 * Enable a specific key in multi-key channel
 */
export async function enableMultiKey(
  channelId: number,
  keyIndex: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'enable_key',
    key_index: keyIndex,
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Disable a specific key in multi-key channel
 */
export async function disableMultiKey(
  channelId: number,
  keyIndex: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'disable_key',
    key_index: keyIndex,
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Delete a specific key in multi-key channel
 */
export async function deleteMultiKey(
  channelId: number,
  keyIndex: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'delete_key',
    key_index: keyIndex,
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Enable all keys in multi-key channel
 */
export async function enableAllMultiKeys(
  channelId: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'enable_all_keys',
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Disable all keys in multi-key channel
 */
export async function disableAllMultiKeys(
  channelId: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'disable_all_keys',
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Delete all disabled keys in multi-key channel
 */
export async function deleteDisabledMultiKeys(
  channelId: number
): Promise<{ success: boolean; message?: string; data?: number }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'delete_disabled_keys',
  }) as Promise<{ success: boolean; message?: string; data?: number }>
}

// ============================================================================
// Tag Operations
// ============================================================================

/**
 * Enable all channels with a specific tag
 */
export async function enableTagChannels(
  tag: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post(
    '/api/channel/tag/enabled',
    { tag },
    channelActionConfig()
  )
  return res.data
}

/**
 * Disable all channels with a specific tag
 */
export async function disableTagChannels(
  tag: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post(
    '/api/channel/tag/disabled',
    { tag },
    channelActionConfig()
  )
  return res.data
}

/**
 * Edit all channels with a specific tag
 */
export async function editTagChannels(
  params: TagOperationParams
): Promise<{ success: boolean; message?: string }> {
  const res = await api.put('/api/channel/tag', params, channelActionConfig())
  return res.data
}

/**
 * Get models for a specific tag
 */
export async function getTagModels(
  tag: string
): Promise<{ success: boolean; message?: string; data?: string }> {
  const res = await api.get('/api/channel/tag/models', { params: { tag } })
  return res.data
}

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Fetch models from the current unsaved channel form configuration.
 */
export async function fetchModels(data: {
  base_url: string
  type: number
  key?: string
  channel_id?: number
  advanced_custom?: string
  header_override?: string
  proxy?: string
}): Promise<FetchModelsResponse> {
  const res = await api.post(
    '/api/channel/fetch_models',
    data,
    channelActionConfig()
  )
  return res.data
}

/**
 * Delete an Ollama model from a channel
 */
export async function deleteOllamaModel(params: {
  channel_id: number
  model_name: string
}): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(
    '/api/channel/ollama/delete',
    channelActionConfig({ data: params })
  )
  return res.data
}

/**
 * Test all enabled channels
 */
export async function testAllChannels(): Promise<{
  success: boolean
  message?: string
}> {
  const res = await api.get('/api/channel/test', channelActionConfig())
  return res.data
}

/**
 * Update balance for all enabled channels
 */
export async function updateAllChannelsBalance(): Promise<{
  success: boolean
  message?: string
  data?: { started?: boolean; task_id?: string }
}> {
  const res = await api.get(
    '/api/channel/update_balance',
    channelActionConfig()
  )
  return res.data
}

export type ChannelBalanceTask = {
  task_id: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  state?: { total?: number; processed?: number; progress?: number } | null
  error?: string
}

export async function getChannelBalanceTask(taskId: string): Promise<{
  success: boolean
  message?: string
  data?: ChannelBalanceTask | null
}> {
  const response = await api.get(
    `/api/channel/update_balance/task/${encodeURIComponent(taskId)}`,
    channelActionConfig()
  )
  return response.data
}

/**
 * Get all available models
 */
export async function getAllModels(): Promise<{
  success: boolean
  message?: string
  data?: Array<{ id: string; [key: string]: unknown }>
}> {
  const res = await api.get('/api/channel/models')
  return res.data
}

/**
 * Get all enabled models
 */
export async function getEnabledModels(): Promise<{
  success: boolean
  message?: string
  data?: string[]
}> {
  const res = await api.get('/api/channel/models_enabled')
  return res.data
}

// ============================================================================
// Ollama Utilities
// ============================================================================

/**
 * Check Ollama version for a given channel
 */
export async function getOllamaVersion(
  channelId: number
): Promise<{ success: boolean; message?: string; data?: { version: string } }> {
  const res = await api.get(`/api/channel/ollama/version/${channelId}`)
  return res.data
}

// ============================================================================
// Group Management
// ============================================================================

/**
 * Get all available groups (re-exported from users API for convenience)
 */
export const getGroups = getUserGroups

// ============================================================================
// Prefill Groups (Model Groups)
// ============================================================================

/**
 * Get prefill groups for quick model selection
 */
export async function getPrefillGroups(
  type: 'model' | 'group' = 'model'
): Promise<{
  success: boolean
  message?: string
  data?: Array<{ id: number; name: string; items: string | string[] }>
}> {
  const res = await api.get('/api/prefill_group', { params: { type } })
  return res.data
}

// ============================================================================
// Group Channel Management, Smart Scheduling, Price Table & CRON Sync
// ============================================================================

export type SchedulingMode =
  | 'standard'
  | 'bbr'
  | 'lowercost'
  | 'speed'
  | 'availability'
  | 'costcontrol'

export type CostControlWindow = 'today' | '24h' | '7d' | '30d'

export interface CostControlUpstreamAlloc {
  enabled: boolean
  weight: number
}

export interface CostControlChannelRule {
  upstream_id?: string
  priority?: number
  weight?: number
}

export interface GroupCostControlPolicy {
  window: CostControlWindow
  max_cost_ratio_pct: number
  min_net_profit_usd: number
  overflow_sensitivity: number
  upstream_allocations: Record<string, CostControlUpstreamAlloc>
  channel_rules: Record<string, CostControlChannelRule>
}

export interface RealtimeProfitCounter {
  calls: number
  revenue_usd: number
  cost_usd: number
  net_profit_usd: number
}

export interface GroupRealtimeProfitSnapshot {
  group: string
  window: CostControlWindow
  window_start: number
  updated_at: number
  total_calls: number
  revenue_usd: number
  cost_usd: number
  net_profit_usd: number
  net_margin_pct: number
  cost_ratio_pct: number
  max_cost_ratio_pct: number
  min_net_profit_usd: number
  allowed_cost_usd: number
  profit_cushion_usd: number
  is_over_cost: boolean
  divert_prob_pct: number
  by_upstream: Record<string, RealtimeProfitCounter>
  by_channel: Record<string, RealtimeProfitCounter>
}

export interface UpstreamSourceBrief {
  id: string
  name: string
  base_url?: string
  sync_mode?: string
  enabled: boolean
  is_direct: boolean
  currency: string
  exchange_rate: number
  balance_usd: number
  used_quota_usd: number
  channel_count: number
}

export interface GroupSchedulingSummary {
  group: string
  description: string
  ratio: number
  mode: SchedulingMode
  enabled_channel_ids: number[]
  cost_control?: GroupCostControlPolicy
  realtime_profit?: GroupRealtimeProfitSnapshot
}

export interface ChannelBrief {
  id: number
  name: string
  type: number
  status: number
  group: string
  models: string
  priority: number
  weight: number
  tag: string
  base_url: string
  balance?: number
  balance_updated_time?: number
  upstream_source_id?: string
  upstream_source_name?: string
  upstream_group?: string
  base_multiplier?: number
  currency?: string
  from_upstream?: boolean
}

export interface GroupSchedulingResponseData {
  groups: GroupSchedulingSummary[]
  channels: ChannelBrief[]
  upstream_sources?: UpstreamSourceBrief[]
}

export async function getGroupScheduling(): Promise<{
  success: boolean
  message?: string
  data?: GroupSchedulingResponseData
}> {
  const res = await api.get(
    '/api/channel/group_scheduling',
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function updateGroupScheduling(payload: {
  group?: string
  mode?: SchedulingMode
  enabled_channel_ids?: number[]
  modes?: Record<string, SchedulingMode>
  cost_control?: GroupCostControlPolicy
  cost_controls?: Record<string, GroupCostControlPolicy>
}): Promise<{
  success: boolean
  message?: string
  data?: GroupSchedulingResponseData
}> {
  const res = await api.put(
    '/api/channel/group_scheduling',
    payload,
    channelActionConfig()
  )
  return res.data
}

export interface ModelPriceTableRow {
  channel_id: number
  channel_name: string
  channel_type: number
  channel_status: number
  model: string
  input_cost: number
  output_cost: number
  raw_input_cost?: number
  raw_output_cost?: number
  model_ratio: number
  completion_ratio: number
  cache_ratio: number
  create_cache_ratio: number
  model_price: number
  raw_model_price?: number
  quota_type: number
  base_multiplier: number
  currency: string
  exchange_rate: number
  from_upstream: boolean
  availability: number
  tps_min: number
  tps_avg: number
  tps_max: number
  avg_ttft: number
  is_selected_price: boolean
}

export interface ModelPriceSyncConfig {
  highest_price_priority: boolean
  preferred_channels: Record<string, number>
  default_currency: string
  custom_exchange_rate: number
  usd_exchange_rate?: number
  cny_exchange_rate?: number
  twd_exchange_rate?: number
  channel_currencies: Record<number, string>
  channel_exchange_rates: Record<number, number>
  channel_topup_multipliers?: Record<number, number>
  channel_upstream_groups: Record<number, string>
  cron_enabled: boolean
  cron_interval_minutes: number
  auto_sync_models: boolean
  auto_sync_prices: boolean
  replace_models?: boolean
  classifier_enabled?: boolean
  classifier_rules?: ChannelClassifierRule[]
  last_sync_time: number
}

export interface ChannelClassifierRule {
  id: string
  name: string
  target_group: string
  description?: string
  group_ratio: number
  keywords: string[]
  include_native_api: boolean
  enabled: boolean
}

export interface ChannelClassificationItemResult {
  channel_id: number
  channel_name: string
  channel_type: number
  upstream_group?: string
  is_native_api: boolean
  matched_groups: string[]
  final_groups: string
  changed: boolean
}

export interface ChannelClassificationSummary {
  total_channels: number
  updated_channels: number
  synced_groups: string[]
  items: ChannelClassificationItemResult[]
}

export async function getModelPriceTable(modelKeyword?: string): Promise<{
  success: boolean
  message?: string
  data?: {
    rows: ModelPriceTableRow[]
    config: ModelPriceSyncConfig
  }
}> {
  const res = await api.get('/api/channel/model_price_table', {
    params: modelKeyword ? { model: modelKeyword } : undefined,
    ...channelActionConfig({ disableDuplicate: true }),
  })
  return res.data
}

export async function applyModelPriceTable(payload: {
  highest_price_priority?: boolean
  preferred_channels?: Record<string, number>
  model?: string
  channel_id?: number
}): Promise<{
  success: boolean
  message?: string
  data?: {
    updated_models: number
    config: ModelPriceSyncConfig
    rows: ModelPriceTableRow[]
  }
}> {
  const res = await api.post(
    '/api/channel/model_price_table/apply',
    payload,
    channelActionConfig()
  )
  return res.data
}

export interface ChannelUpstreamSyncItemResult {
  channel_id: number
  channel_name: string
  channel_type: number
  success: boolean
  error?: string
  synced_model_count: number
  added_models: string[]
  removed_models?: string[]
  synced_price_count: number
  detected_currency: string
  exchange_rate: number
  base_multiplier: number
  upstream_group?: string
  available_groups?: Record<string, number>
  balance?: number
  balance_updated_time?: number
}

export async function getCronSyncConfig(): Promise<{
  success: boolean
  message?: string
  data?: {
    config: ModelPriceSyncConfig
    channels: ChannelBrief[]
  }
}> {
  const res = await api.get(
    '/api/channel/cron_sync_config',
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function updateCronSyncConfig(
  payload: ModelPriceSyncConfig
): Promise<{
  success: boolean
  message?: string
  data?: {
    config: ModelPriceSyncConfig
  }
}> {
  const res = await api.put(
    '/api/channel/cron_sync_config',
    payload,
    channelActionConfig()
  )
  return res.data
}

export async function runCronSyncNow(payload?: {
  channel_ids?: number[]
  sync_models?: boolean
  sync_prices?: boolean
  replace_models?: boolean
  currency?: string
  exchange_rate?: number
  highest_price_priority?: boolean
}): Promise<{
  success: boolean
  message?: string
  data?: {
    results: ChannelUpstreamSyncItemResult[]
    config: ModelPriceSyncConfig
  }
}> {
  const res = await api.post(
    '/api/channel/cron_sync_run',
    payload ?? {},
    channelActionConfig()
  )
  return res.data
}

export type CronJobType =
  | 'upstream_group_sync'
  | 'upstream_sync'
  | 'channel_test'
  | 'channel_balance'
  | 'shell_script'
  | 'http_request'

export interface CronJob {
  id: string
  name: string
  description?: string
  enabled: boolean
  cron_expr: string
  job_type: CronJobType
  script?: string
  http_method?: string
  http_headers?: string
  http_body?: string
  working_dir?: string
  timeout_sec: number
  built_in?: boolean
  last_run_at: number
  next_run_at: number
  last_status: 'idle' | 'running' | 'success' | 'failed' | string
  last_duration_ms: number
  last_output?: string
  run_count: number
}

export async function getCronJobs(): Promise<{
  success: boolean
  message?: string
  data?: {
    jobs: CronJob[]
  }
}> {
  const res = await api.get(
    '/api/channel/cron_jobs',
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function saveCronJob(payload: Partial<CronJob>): Promise<{
  success: boolean
  message?: string
  data?: {
    jobs: CronJob[]
  }
}> {
  const res = await api.post(
    '/api/channel/cron_jobs',
    payload,
    channelActionConfig()
  )
  return res.data
}

export async function deleteCronJob(id: string): Promise<{
  success: boolean
  message?: string
  data?: {
    jobs: CronJob[]
  }
}> {
  const res = await api.delete(
    `/api/channel/cron_jobs/${encodeURIComponent(id)}`,
    channelActionConfig()
  )
  return res.data
}

export async function runCronJobNow(id: string): Promise<{
  success: boolean
  message?: string
  data?: {
    job: CronJob
    jobs: CronJob[]
  }
}> {
  const res = await api.post(
    `/api/channel/cron_jobs/${encodeURIComponent(id)}/run`,
    {},
    channelActionConfig()
  )
  return res.data
}

export interface UpstreamAccountSource {
  id: string
  name: string
  base_url: string
  sync_mode?: 'standard' | 'lumiya'
  user_id: string
  access_token?: string
  has_access_token?: boolean
  token_name_rule: string
  channel_name_rule: string
  channel_tag: string
  local_group: string
  currency: string
  exchange_rate: number
  topup_multiplier?: number
  clean_removed_groups: boolean
  adopt_existing_channel: boolean
  enabled: boolean
  balance_usd?: number
  used_quota_usd?: number
  last_sync_at: number
  last_status: string
  last_message?: string
  active_groups?: string[]
  created_channels?: string[]
  updated_channels?: string[]
  removed_channels?: string[]
}

export interface AutoProvisionedChannelMeta {
  channel_id: number
  channel_name?: string
  source_id: string
  source_name: string
  upstream_group: string
  auto_created: boolean
  balance?: number
  balance_updated_time?: number
  created_at: number
  updated_at: number
}

export async function getUpstreamAccounts(): Promise<{
  success: boolean
  message?: string
  data?: {
    accounts: UpstreamAccountSource[]
    auto_provisioned_channels: Record<string, AutoProvisionedChannelMeta>
  }
}> {
  const res = await api.get(
    '/api/channel/upstream_accounts',
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function saveUpstreamAccount(
  payload: Partial<UpstreamAccountSource>
): Promise<{
  success: boolean
  message?: string
  data?: {
    accounts: UpstreamAccountSource[]
    auto_provisioned_channels: Record<string, AutoProvisionedChannelMeta>
  }
}> {
  const res = await api.post(
    '/api/channel/upstream_accounts',
    payload,
    channelActionConfig()
  )
  return res.data
}

export async function deleteUpstreamAccount(id: string): Promise<{
  success: boolean
  message?: string
  data?: {
    accounts: UpstreamAccountSource[]
    auto_provisioned_channels: Record<string, AutoProvisionedChannelMeta>
  }
}> {
  const res = await api.delete(
    `/api/channel/upstream_accounts/${encodeURIComponent(id)}`,
    channelActionConfig()
  )
  return res.data
}

export async function syncUpstreamAccountsNow(accountIds?: string[]): Promise<{
  success: boolean
  message?: string
  data?: {
    summary: string
    error?: string
    accounts: UpstreamAccountSource[]
    auto_provisioned_channels: Record<string, AutoProvisionedChannelMeta>
  }
}> {
  const res = await api.post(
    '/api/channel/upstream_accounts/sync',
    { account_ids: accountIds ?? [] },
    channelActionConfig()
  )
  return res.data
}

export async function getChannelClassifier(): Promise<{
  success: boolean
  message?: string
  data?: {
    enabled: boolean
    rules: ChannelClassifierRule[]
    preview: ChannelClassificationItemResult[]
  }
}> {
  const res = await api.get(
    '/api/channel/classifier',
    channelActionConfig({ disableDuplicate: true })
  )
  return res.data
}

export async function saveChannelClassifier(payload: {
  enabled?: boolean
  rules?: ChannelClassifierRule[]
  rule?: Partial<ChannelClassifierRule>
  apply_now?: boolean
}): Promise<{
  success: boolean
  message?: string
  data?: {
    enabled: boolean
    rules: ChannelClassifierRule[]
    summary?: ChannelClassificationSummary
    preview: ChannelClassificationItemResult[]
  }
}> {
  const res = await api.post(
    '/api/channel/classifier',
    payload,
    channelActionConfig()
  )
  return res.data
}

export async function deleteChannelClassifierRule(id: string): Promise<{
  success: boolean
  message?: string
  data?: {
    enabled: boolean
    rules: ChannelClassifierRule[]
    summary?: ChannelClassificationSummary
    preview: ChannelClassificationItemResult[]
  }
}> {
  const res = await api.delete(
    `/api/channel/classifier/${encodeURIComponent(id)}`,
    channelActionConfig()
  )
  return res.data
}

export async function applyChannelClassifierNow(
  channelIds?: number[]
): Promise<{
  success: boolean
  message?: string
  data?: {
    enabled: boolean
    rules: ChannelClassifierRule[]
    summary: ChannelClassificationSummary
    preview: ChannelClassificationItemResult[]
  }
}> {
  const res = await api.post(
    '/api/channel/classifier/apply',
    { channel_ids: channelIds ?? [] },
    channelActionConfig()
  )
  return res.data
}

export interface ModelGroupPriceDetail {
  group: string
  description: string
  group_ratio: number
  quota_type: number
  input_price: number
  output_price: number
  call_count: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  revenue_usd: number
  cost_usd: number
  profit_usd: number
  margin_pct: number
}

export interface ModelChannelUsageDetail {
  channel_id: number
  channel_name: string
  channel_type: number
  upstream_group?: string
  base_multiplier: number
  topup_multiplier?: number
  currency: string
  exchange_rate: number
  from_upstream?: boolean
  quota_type: number
  input_cost: number
  output_cost: number
  raw_input_cost?: number
  raw_output_cost?: number
  call_count: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  revenue_usd: number
  cost_usd: number
  profit_usd: number
  margin_pct: number
}

export interface ModelCostAnalysisRow {
  model: string
  quota_type: number
  billing_mode: string
  base_input_price: number
  base_output_price: number
  model_ratio: number
  completion_ratio: number
  min_input_cost: number
  max_input_cost: number
  min_output_cost: number
  max_output_cost: number
  call_count: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  revenue_usd: number
  cost_usd: number
  profit_usd: number
  margin_pct: number
  group_prices: ModelGroupPriceDetail[]
  channel_usages: ModelChannelUsageDetail[]
}

export interface UserModelChannelCostDetail {
  model: string
  channel_id: number
  channel_name: string
  call_count: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  revenue_usd: number
  cost_usd: number
  profit_usd: number
  margin_pct: number
}

export interface UserCostAnalysisRow {
  user_id: number
  username: string
  display_name: string
  group: string
  status: number
  remain_quota_usd: number
  lifetime_used_usd: number
  topup_total_usd: number
  online_topup_usd: number
  online_paid_money: number
  system_topup_usd: number
  consumed_usd: number
  call_count: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  revenue_usd: number
  cost_usd: number
  gross_profit_usd: number
  gross_margin_pct: number
  model_breakdowns: UserModelChannelCostDetail[]
}

export async function getModelCostAnalysis(params?: {
  start_timestamp?: number
  end_timestamp?: number
  keyword?: string
}): Promise<{
  success: boolean
  message?: string
  data?: {
    models: ModelCostAnalysisRow[]
    summary: {
      model_count: number
      total_calls: number
      total_tokens: number
      total_revenue: number
      total_cost: number
      total_profit: number
      margin_pct: number
    }
  }
}> {
  const res = await api.get('/api/channel/cost_management/models', {
    params,
    ...channelActionConfig({ disableDuplicate: true }),
  })
  return res.data
}

export async function getUserCostAnalysis(params?: {
  start_timestamp?: number
  end_timestamp?: number
  keyword?: string
}): Promise<{
  success: boolean
  message?: string
  data?: {
    users: UserCostAnalysisRow[]
    summary: {
      user_count: number
      total_topup_usd: number
      total_consumed_usd: number
      total_revenue_usd: number
      total_cost_usd: number
      total_profit_usd: number
      overall_margin_pct: number
      total_calls: number
      total_tokens: number
    }
  }
}> {
  const res = await api.get('/api/channel/cost_management/users', {
    params,
    ...channelActionConfig({ disableDuplicate: true }),
  })
  return res.data
}

export interface ModelChannelHealthDetail {
  channel_id: number
  channel_name: string
  channel_type: number
  channel_status: number
  model: string
  request_count: number
  success_count: number
  error_count: number
  availability: number
  avg_prompt_tokens: number
  avg_output_tokens: number
  avg_ttft_sec: number
  median_ttft_sec: number
  p95_ttft_sec: number
  prefill_tps_avg: number
  prefill_tps_median: number
  decode_tps_p5_low: number
  decode_tps_median: number
  decode_tps_trimmed_90: number
  decode_tps_p95_high: number
  decode_tps_avg: number
  total_tps_p5_low: number
  total_tps_median: number
  total_tps_trimmed_90: number
  total_tps_p95_high: number
  total_tps_avg: number
}

export interface ModelHealthAnalysisRow {
  model: string
  channel_count: number
  request_count: number
  success_count: number
  error_count: number
  availability: number
  avg_prompt_tokens: number
  avg_output_tokens: number
  avg_ttft_sec: number
  median_ttft_sec: number
  p95_ttft_sec: number
  prefill_tps_avg: number
  prefill_tps_median: number
  decode_tps_p5_low: number
  decode_tps_median: number
  decode_tps_trimmed_90: number
  decode_tps_p95_high: number
  decode_tps_avg: number
  total_tps_p5_low: number
  total_tps_median: number
  total_tps_trimmed_90: number
  total_tps_p95_high: number
  total_tps_avg: number
  channels: ModelChannelHealthDetail[]
}

export interface ChannelHealthSummaryRow {
  channel_id: number
  channel_name: string
  channel_type: number
  channel_status: number
  model_count: number
  request_count: number
  success_count: number
  error_count: number
  availability: number
  avg_prompt_tokens: number
  avg_output_tokens: number
  avg_ttft_sec: number
  median_ttft_sec: number
  p95_ttft_sec: number
  prefill_tps_avg: number
  prefill_tps_median: number
  decode_tps_p5_low: number
  decode_tps_median: number
  decode_tps_trimmed_90: number
  decode_tps_p95_high: number
  decode_tps_avg: number
  total_tps_p5_low: number
  total_tps_median: number
  total_tps_trimmed_90: number
  total_tps_p95_high: number
  total_tps_avg: number
  models: ModelChannelHealthDetail[]
}

export interface ModelHealthSummary {
  model_count: number
  channel_count: number
  request_count: number
  success_count: number
  error_count: number
  availability: number
  avg_ttft_sec: number
  median_ttft_sec: number
  prefill_tps_avg: number
  prefill_tps_median: number
  decode_tps_p5_low: number
  decode_tps_median: number
  decode_tps_trimmed_90: number
  decode_tps_p95_high: number
  decode_tps_avg: number
  total_tps_p5_low: number
  total_tps_median: number
  total_tps_trimmed_90: number
  total_tps_p95_high: number
  total_tps_avg: number
}

export async function getModelHealthAnalysis(params?: {
  start_timestamp?: number
  end_timestamp?: number
  keyword?: string
}): Promise<{
  success: boolean
  message?: string
  data?: {
    models: ModelHealthAnalysisRow[]
    channels: ChannelHealthSummaryRow[]
    summary: ModelHealthSummary
  }
}> {
  const res = await api.get('/api/channel/model_health', {
    params,
    ...channelActionConfig({ disableDuplicate: true }),
  })
  return res.data
}
