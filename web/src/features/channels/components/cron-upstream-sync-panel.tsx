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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  Coins,
  Play,
  RefreshCw,
  Save,
  Wallet,
  XCircle,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useMediaQuery } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  type ChannelUpstreamSyncItemResult,
  type ModelPriceSyncConfig,
  getCronSyncConfig,
  runCronSyncNow,
  updateCronSyncConfig,
} from '../api'
import { CHANNEL_TYPE_NEW_API, CHANNEL_TYPE_OPTIONS } from '../constants'
import { ChannelTypeLogo } from './channel-type-badge'

const DEFAULT_SYNC_CFG: ModelPriceSyncConfig = {
  highest_price_priority: true,
  preferred_channels: {},
  default_currency: 'AUTO',
  custom_exchange_rate: 0,
  usd_exchange_rate: 1,
  cny_exchange_rate: 7.3,
  twd_exchange_rate: 32,
  channel_currencies: {},
  channel_exchange_rates: {},
  channel_upstream_groups: {},
  cron_enabled: false,
  cron_interval_minutes: 60,
  auto_sync_models: true,
  auto_sync_prices: true,
  replace_models: true,
  last_sync_time: 0,
}

export function CronUpstreamSyncPanel() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  useSystemConfigStore((state) => state.config.currency)

  const cronQuery = useQuery({
    queryKey: ['channel-cron-sync-config'],
    queryFn: async () => requireServerSuccess(await getCronSyncConfig()),
  })

  const [formCfg, setFormCfg] = useState<ModelPriceSyncConfig>(DEFAULT_SYNC_CFG)
  const [selectedChannelIds, setSelectedChannelIds] = useState<number[]>([])
  const [lastSyncResults, setLastSyncResults] = useState<
    ChannelUpstreamSyncItemResult[]
  >([])

  const channels = useMemo(
    () => cronQuery.data?.data?.channels ?? [],
    [cronQuery.data]
  )

  useEffect(() => {
    const serverCfg = cronQuery.data?.data?.config
    if (serverCfg) {
      setFormCfg({
        ...DEFAULT_SYNC_CFG,
        ...serverCfg,
        usd_exchange_rate: 1,
        cny_exchange_rate:
          serverCfg.cny_exchange_rate &&
          serverCfg.cny_exchange_rate > 5.5 &&
          serverCfg.cny_exchange_rate < 15
            ? serverCfg.cny_exchange_rate
            : 7.3,
        twd_exchange_rate:
          serverCfg.twd_exchange_rate && serverCfg.twd_exchange_rate >= 15
            ? serverCfg.twd_exchange_rate
            : 32,
        channel_currencies: serverCfg.channel_currencies ?? {},
        channel_exchange_rates: serverCfg.channel_exchange_rates ?? {},
        channel_upstream_groups: serverCfg.channel_upstream_groups ?? {},
        preferred_channels: serverCfg.preferred_channels ?? {},
        replace_models: serverCfg.replace_models ?? true,
      })
    }
  }, [cronQuery.data])

  const saveConfigMutation = useMutation({
    mutationFn: async () =>
      requireServerSuccess(await updateCronSyncConfig(formCfg)),
    onSuccess: () => {
      toast.success(t('CRON upstream sync settings saved'))
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save CRON sync settings'))
    },
  })

  const runSyncMutation = useMutation({
    mutationFn: async () => {
      await updateCronSyncConfig(formCfg)
      return requireServerSuccess(
        await runCronSyncNow({
          channel_ids:
            selectedChannelIds.length > 0 ? selectedChannelIds : undefined,
          sync_models: formCfg.auto_sync_models,
          sync_prices: formCfg.auto_sync_prices,
          replace_models: formCfg.replace_models ?? true,
          currency: formCfg.default_currency,
          exchange_rate: formCfg.custom_exchange_rate,
          highest_price_priority: formCfg.highest_price_priority,
        })
      )
    },
    onSuccess: (res) => {
      const results = res.data?.results ?? []
      setLastSyncResults(results)
      const okCount = results.filter((r) => r.success).length
      toast.success(
        `${t('Upstream sync completed')}: ${okCount}/${results.length} ${t('channels succeeded')}`
      )
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['channel-model-price-table'],
      })
      void queryClient.invalidateQueries({ queryKey: ['upstream-accounts'] })
      void queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to run upstream model sync'))
    },
  })

  const toggleChannelSelection = (id: number, checked: boolean) => {
    setSelectedChannelIds((prev) => {
      if (checked) {
        return prev.includes(id) ? prev : [...prev, id]
      }
      return prev.filter((x) => x !== id)
    })
  }

  const updateChannelCurrency = (channelId: number, currency: string) => {
    setFormCfg((prev) => ({
      ...prev,
      channel_currencies: {
        ...prev.channel_currencies,
        [channelId]: currency,
      },
    }))
  }

  const updateChannelUpstreamGroup = (channelId: number, group: string) => {
    setFormCfg((prev) => ({
      ...prev,
      channel_upstream_groups: {
        ...prev.channel_upstream_groups,
        [channelId]: group,
      },
    }))
  }

  const getChannelTypeLabel = (type: number) =>
    CHANNEL_TYPE_OPTIONS.find((item) => item.value === type)?.label ??
    `#${type}`

  const formatBalanceUSD = (val: number | undefined): string => {
    if (typeof val !== 'number' || !Number.isFinite(val)) {
      return '-'
    }
    return formatBillingCurrencyFromUSD(val, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
      locale: intlLocale,
    })
  }

  if (cronQuery.isLoading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-36 w-full rounded-xl' />
        <Skeleton className='h-80 w-full rounded-xl' />
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      {/* Upstream Sync & Currency Configuration Card */}
      <div className='border-border/60 bg-card flex flex-col gap-4 rounded-xl border p-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='flex items-center gap-2.5'>
            <RefreshCw aria-hidden='true' className='text-primary size-5' />
            <div>
              <h3 className='text-sm font-semibold'>
                {t('Channel Upstream Model Sync')}
              </h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Query upstream channels for supported models and sync New API upstream prices at 1x base ratio'
                )}
              </p>
            </div>
          </div>

          <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => saveConfigMutation.mutate()}
              disabled={saveConfigMutation.isPending}
              className='flex-1 gap-1.5 sm:flex-initial'
            >
              <Save aria-hidden='true' className='size-3.5' />
              <span>{t('Save Sync Settings')}</span>
            </Button>
            <Button
              size='sm'
              onClick={() => runSyncMutation.mutate()}
              disabled={runSyncMutation.isPending}
              className='flex-1 gap-1.5 sm:flex-initial'
            >
              {runSyncMutation.isPending ? (
                <RefreshCw
                  aria-hidden='true'
                  className='size-3.5 animate-spin'
                />
              ) : (
                <Play aria-hidden='true' className='size-3.5' />
              )}
              <span>
                {selectedChannelIds.length > 0
                  ? `${t('Sync Selected Channels')} (${selectedChannelIds.length})`
                  : t('Sync All Upstream Channels Now')}
              </span>
            </Button>
          </div>
        </div>

        <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4'>
          {/* Sync Upstream Models */}
          <div className='border-border/50 bg-muted/20 flex items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <Label className='text-xs font-semibold'>
                {t('Sync Upstream Models')}
              </Label>
              <p className='text-muted-foreground text-[11px]'>
                {t('Fetch and update supported models per channel')}
              </p>
            </div>
            <Switch
              checked={formCfg.auto_sync_models}
              onCheckedChange={(val) =>
                setFormCfg((p) => ({ ...p, auto_sync_models: Boolean(val) }))
              }
            />
          </div>

          {/* Authoritative Replace / Clean Models */}
          <div className='border-border/50 bg-muted/20 flex items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <Label className='text-xs font-semibold'>
                {t('Clean Removed / Non-Group Models')}
              </Label>
              <p className='text-muted-foreground text-[11px]'>
                {t('Remove models not in the channel upstream group')}
              </p>
            </div>
            <Switch
              checked={formCfg.replace_models ?? true}
              onCheckedChange={(val) =>
                setFormCfg((p) => ({ ...p, replace_models: Boolean(val) }))
              }
            />
          </div>

          {/* Sync 1x Base Prices */}
          <div className='border-border/50 bg-muted/20 flex items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <Label className='text-xs font-semibold'>
                {t('Sync New API Prices (1x Base)')}
              </Label>
              <p className='text-muted-foreground text-[11px]'>
                {t('Apply authoritative 1x base pricing to system')}
              </p>
            </div>
            <Switch
              checked={formCfg.auto_sync_prices}
              onCheckedChange={(val) =>
                setFormCfg((p) => ({ ...p, auto_sync_prices: Boolean(val) }))
              }
            />
          </div>

          {/* Default Currency & Exchange Rates (CNY / USD / TWD -> Site NTD) */}
          <div className='border-border/50 bg-muted/20 flex flex-col justify-between gap-2 rounded-lg border p-3'>
            <div className='flex items-center justify-between gap-1.5'>
              <div className='flex items-center gap-1.5'>
                <Coins aria-hidden='true' className='text-primary size-3.5' />
                <Label className='text-xs font-semibold'>
                  {t('Channel Currency')}
                </Label>
              </div>
              <NativeSelect
                size='sm'
                value={formCfg.default_currency || 'AUTO'}
                onChange={(e) =>
                  setFormCfg((p) => ({
                    ...p,
                    default_currency: e.target.value,
                  }))
                }
              >
                <NativeSelectOption value='AUTO'>
                  {t('Auto Detect')}
                </NativeSelectOption>
                <NativeSelectOption value='CNY'>CNY (¥)</NativeSelectOption>
                <NativeSelectOption value='USD'>USD ($)</NativeSelectOption>
                <NativeSelectOption value='TWD'>TWD (NT$)</NativeSelectOption>
              </NativeSelect>
            </div>
            <p className='text-muted-foreground text-[11px]'>
              {t(
                'Upstream settlement currency code; leave empty to use system default'
              )}
            </p>
          </div>
        </div>
      </div>

      {/* Channels List with Per-Channel Currency Override */}
      <div className='border-border/60 bg-card flex flex-col gap-3 rounded-xl border p-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h4 className='text-sm font-semibold'>
            {t('Upstream Channels')} ({channels.length})
          </h4>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setSelectedChannelIds(channels.map((c) => c.id))}
            >
              {t('Select All')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setSelectedChannelIds([])}
            >
              {t('Clear Selection')}
            </Button>
          </div>
        </div>

        {isMobile ? (
          <div className='flex flex-col gap-2.5'>
            {channels.length === 0 ? (
              <div className='text-muted-foreground py-8 text-center text-sm'>
                {t('No matching channels found')}
              </div>
            ) : (
              channels.map((ch) => {
              const isSelected = selectedChannelIds.includes(ch.id)
              const chCurrency = formCfg.channel_currencies?.[ch.id] || 'AUTO'
              const syncRes = lastSyncResults.find(
                (r) => r.channel_id === ch.id
              )
              return (
                <div
                  key={ch.id}
                  className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
                >
                  <div className='flex items-start justify-between gap-2'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <Checkbox
                        checked={isSelected}
                        onCheckedChange={(val) =>
                          toggleChannelSelection(ch.id, Boolean(val))
                        }
                        aria-label={`${t('Select channel')} ${ch.name}`}
                      />
                      <span className='text-muted-foreground shrink-0 font-mono text-xs'>
                        #{ch.id}
                      </span>
                      <span className='truncate text-xs font-semibold'>
                        {ch.name}
                      </span>
                    </div>
                    <div className='flex shrink-0 items-center gap-1.5 text-xs'>
                      {typeof (syncRes?.balance ?? ch.balance) === 'number' ? (
                        <Badge
                          variant='outline'
                          className='gap-1 px-1.5 py-0 font-mono text-[10px] text-emerald-600 dark:text-emerald-400'
                        >
                          <Wallet aria-hidden='true' className='size-3' />
                          {formatBalanceUSD(syncRes?.balance ?? ch.balance)}
                        </Badge>
                      ) : null}
                      <ChannelTypeLogo type={ch.type} size={14} />
                      <span className='text-[11px]'>
                        {t(getChannelTypeLabel(ch.type))}
                      </span>
                    </div>
                  </div>

                  <div className='flex flex-wrap items-center justify-between gap-1 text-xs'>
                    <span className='text-muted-foreground truncate font-mono text-[11px]'>
                      {ch.base_url || t('Official Default (1x)')}
                    </span>
                    {ch.type === CHANNEL_TYPE_NEW_API ? (
                      <Badge
                        variant='secondary'
                        className='px-1.5 py-0 text-[10px]'
                      >
                        {t('Supports 1x Price Sync')}
                      </Badge>
                    ) : null}
                  </div>

                  <div className='bg-muted/20 flex flex-col gap-2 rounded-md p-2 text-xs'>
                    <div className='flex items-center justify-between gap-2'>
                      <span className='text-muted-foreground shrink-0 text-[11px]'>
                        {t('Currency Override')}
                      </span>
                      <div className='flex items-center gap-1.5'>
                        <NativeSelect
                          size='sm'
                          value={chCurrency}
                          onChange={(e) =>
                            updateChannelCurrency(ch.id, e.target.value)
                          }
                        >
                          <NativeSelectOption value='AUTO'>
                            {t('Auto Detect')}
                          </NativeSelectOption>
                          <NativeSelectOption value='USD'>
                            USD
                          </NativeSelectOption>
                          <NativeSelectOption value='CNY'>
                            CNY
                          </NativeSelectOption>
                          <NativeSelectOption value='TWD'>
                            TWD
                          </NativeSelectOption>
                        </NativeSelect>
                      </div>
                    </div>

                    {ch.type === CHANNEL_TYPE_NEW_API ? (
                      <div className='flex items-center justify-between gap-2'>
                        <span className='text-muted-foreground shrink-0 text-[11px]'>
                          {t('Upstream Group')}
                        </span>
                        {syncRes?.available_groups &&
                        Object.keys(syncRes.available_groups).length > 0 ? (
                          <NativeSelect
                            size='sm'
                            value={
                              formCfg.channel_upstream_groups?.[ch.id] || ''
                            }
                            onChange={(e) =>
                              updateChannelUpstreamGroup(ch.id, e.target.value)
                            }
                          >
                            <NativeSelectOption value=''>
                              {t('Auto Detect')}
                            </NativeSelectOption>
                            {Object.entries(syncRes.available_groups).map(
                              ([gName, gRatio]) => (
                                <NativeSelectOption key={gName} value={gName}>
                                  {gName} ({gRatio}x)
                                </NativeSelectOption>
                              )
                            )}
                          </NativeSelect>
                        ) : (
                          <Input
                            className='h-7 w-36 text-xs'
                            placeholder={t('Auto Detect')}
                            value={
                              formCfg.channel_upstream_groups?.[ch.id] || ''
                            }
                            onChange={(e) =>
                              updateChannelUpstreamGroup(ch.id, e.target.value)
                            }
                          />
                        )}
                      </div>
                    ) : null}
                  </div>

                  {syncRes ? (
                    <div className='text-xs'>
                      {syncRes.success ? (
                        <div className='flex flex-wrap items-center gap-1.5 text-emerald-600 dark:text-emerald-400'>
                          <CheckCircle2
                            aria-hidden='true'
                            className='size-3.5 shrink-0'
                          />
                          <span>
                            {syncRes.synced_model_count} {t('models')} (+
                            {syncRes.added_models?.length ?? 0}
                            {(syncRes.removed_models?.length ?? 0) > 0
                              ? ` / -${syncRes.removed_models?.length ?? 0}`
                              : ''}
                            ), {syncRes.synced_price_count} {t('prices')}
                          </span>
                          {syncRes.upstream_group ? (
                            <Badge
                              variant='outline'
                              className='px-1.5 py-0 font-mono text-[10px]'
                            >
                              {syncRes.upstream_group} (
                              {syncRes.base_multiplier}x)
                            </Badge>
                          ) : null}
                        </div>
                      ) : (
                        <div className='flex items-center gap-1.5 text-rose-600 dark:text-rose-400'>
                          <XCircle
                            aria-hidden='true'
                            className='size-3.5 shrink-0'
                          />
                          <span className='truncate'>{syncRes.error}</span>
                        </div>
                      )}
                    </div>
                  ) : null}
                </div>
              )
            })
            )}
          </div>
        ) : (
          <div className='border-border/50 max-h-[60dvh] overflow-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-12 text-center'>
                    {t('Sync')}
                  </TableHead>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Balance')}</TableHead>
                  <TableHead>{t('Upstream Type')}</TableHead>
                  <TableHead>{t('Base URL')}</TableHead>
                  <TableHead>{t('Currency Override')}</TableHead>
                  <TableHead>{t('Upstream Group')}</TableHead>
                  <TableHead>{t('Last Sync Result')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {channels.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={8}
                      className='text-muted-foreground h-24 text-center text-sm'
                    >
                      {t('No matching channels found')}
                    </TableCell>
                  </TableRow>
                ) : (
                  channels.map((ch) => {
                  const isSelected = selectedChannelIds.includes(ch.id)
                  const chCurrency =
                    formCfg.channel_currencies?.[ch.id] || 'AUTO'
                  const syncRes = lastSyncResults.find(
                    (r) => r.channel_id === ch.id
                  )
                  return (
                    <TableRow key={ch.id}>
                      <TableCell className='text-center'>
                        <Checkbox
                          checked={isSelected}
                          onCheckedChange={(val) =>
                            toggleChannelSelection(ch.id, Boolean(val))
                          }
                          aria-label={`${t('Select channel')} ${ch.name}`}
                        />
                      </TableCell>
                      <TableCell>
                        <div className='flex items-center gap-2'>
                          <span className='text-muted-foreground font-mono text-xs'>
                            #{ch.id}
                          </span>
                          <span className='font-medium'>{ch.name}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant='outline'
                          className='gap-1 px-2 py-0.5 font-mono text-xs text-emerald-600 dark:text-emerald-400'
                        >
                          <Wallet aria-hidden='true' className='size-3' />
                          {formatBalanceUSD(
                            syncRes?.balance ?? ch.balance ?? 0
                          )}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className='flex items-center gap-1.5 text-xs'>
                          <ChannelTypeLogo type={ch.type} size={15} />
                          <span>{t(getChannelTypeLabel(ch.type))}</span>
                          {ch.type === CHANNEL_TYPE_NEW_API && (
                            <Badge
                              variant='secondary'
                              className='px-1.5 py-0 text-[10px]'
                            >
                              {t('Supports 1x Price Sync')}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className='text-muted-foreground max-w-48 truncate font-mono text-xs'>
                        {ch.base_url || t('Official Default (1x)')}
                      </TableCell>
                      <TableCell>
                        <div className='flex items-center gap-1.5'>
                          <NativeSelect
                            size='sm'
                            value={chCurrency}
                            onChange={(e) =>
                              updateChannelCurrency(ch.id, e.target.value)
                            }
                          >
                            <NativeSelectOption value='AUTO'>
                              {t('Auto Detect')}
                            </NativeSelectOption>
                            <NativeSelectOption value='USD'>
                              USD
                            </NativeSelectOption>
                            <NativeSelectOption value='CNY'>
                              CNY
                            </NativeSelectOption>
                            <NativeSelectOption value='TWD'>
                              TWD
                            </NativeSelectOption>
                          </NativeSelect>
                        </div>
                      </TableCell>
                      <TableCell>
                        {ch.type === CHANNEL_TYPE_NEW_API ? (
                          <div className='flex items-center gap-1.5'>
                            {syncRes?.available_groups &&
                            Object.keys(syncRes.available_groups).length > 0 ? (
                              <NativeSelect
                                size='sm'
                                value={
                                  formCfg.channel_upstream_groups?.[ch.id] || ''
                                }
                                onChange={(e) =>
                                  updateChannelUpstreamGroup(
                                    ch.id,
                                    e.target.value
                                  )
                                }
                              >
                                <NativeSelectOption value=''>
                                  {t('Auto Detect')}
                                </NativeSelectOption>
                                {Object.entries(syncRes.available_groups).map(
                                  ([gName, gRatio]) => (
                                    <NativeSelectOption
                                      key={gName}
                                      value={gName}
                                    >
                                      {gName} ({gRatio}x)
                                    </NativeSelectOption>
                                  )
                                )}
                              </NativeSelect>
                            ) : (
                              <Input
                                className='h-7 w-40 text-xs'
                                placeholder={t('Auto Detect')}
                                value={
                                  formCfg.channel_upstream_groups?.[ch.id] || ''
                                }
                                onChange={(e) =>
                                  updateChannelUpstreamGroup(
                                    ch.id,
                                    e.target.value
                                  )
                                }
                              />
                            )}
                          </div>
                        ) : (
                          <span className='text-muted-foreground'>-</span>
                        )}
                      </TableCell>
                      <TableCell className='text-xs'>
                        {(() => {
                          if (!syncRes) {
                            return (
                              <span className='text-muted-foreground'>-</span>
                            )
                          }
                          if (syncRes.success) {
                            const addedCount = syncRes.added_models?.length ?? 0
                            const removedCount =
                              syncRes.removed_models?.length ?? 0
                            return (
                              <div className='flex flex-wrap items-center gap-1.5 text-emerald-600 dark:text-emerald-400'>
                                <CheckCircle2
                                  aria-hidden='true'
                                  className='size-3.5 shrink-0'
                                />
                                <span>
                                  {syncRes.synced_model_count} {t('models')} (+
                                  {addedCount}
                                  {removedCount > 0
                                    ? ` / -${removedCount}`
                                    : ''}
                                  ), {syncRes.synced_price_count} {t('prices')}
                                </span>
                                {syncRes.upstream_group ? (
                                  <Badge
                                    variant='outline'
                                    className='px-1.5 py-0 font-mono text-[10px]'
                                  >
                                    {syncRes.upstream_group} (
                                    {syncRes.base_multiplier}x)
                                  </Badge>
                                ) : null}
                              </div>
                            )
                          }
                          return (
                            <div
                              className={cn(
                                'flex items-center gap-1.5 text-rose-600 dark:text-rose-400'
                              )}
                            >
                              <XCircle
                                aria-hidden='true'
                                className='size-3.5'
                              />
                              <span className='max-w-xs truncate'>
                                {syncRes.error}
                              </span>
                            </div>
                          )
                        })()}
                      </TableCell>
                    </TableRow>
                  )
                })
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </div>
  )
}
