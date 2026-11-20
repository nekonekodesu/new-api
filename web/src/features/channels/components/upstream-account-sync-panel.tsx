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
  ChevronDown,
  ChevronRight,
  Edit3,
  Layers,
  Play,
  Plus,
  RefreshCw,
  Search,
  Server,
  Tag,
  Trash2,
  Wallet,
  XCircle,
} from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  deleteUpstreamAccount,
  getUpstreamAccounts,
  saveUpstreamAccount,
  syncUpstreamAccountsNow,
  type AutoProvisionedChannelMeta,
  type UpstreamAccountSource,
} from '../api'
import { ChannelClassifierPanel } from './channel-classifier-panel'

function resolveAccountSyncMode(
  acc: Partial<UpstreamAccountSource>
): 'standard' | 'lumiya' {
  if (acc.sync_mode === 'lumiya' || acc.sync_mode === 'standard') {
    return acc.sync_mode
  }
  const combined = `${acc.name ?? ''} ${acc.base_url ?? ''}`.toLowerCase()
  if (
    combined.includes('lumiya') ||
    combined.includes('露米娅') ||
    combined.includes('xn--1ys141f4ks')
  ) {
    return 'lumiya'
  }
  return 'standard'
}

function createDefaultAccountForm(): Partial<UpstreamAccountSource> {
  return {
    id: '',
    name: '',
    base_url: '',
    sync_mode: 'standard',
    user_id: '',
    access_token: '',
    token_name_rule: 'new_api-{group}-{ratio}',
    channel_name_rule: '{name}-{group}-{ratio}',
    channel_tag: '',
    local_group: 'default',
    currency: 'CNY',
    exchange_rate: 5,
    clean_removed_groups: true,
    adopt_existing_channel: true,
    enabled: true,
  }
}

function formatBalanceUSD(val: number | undefined, locale?: string): string {
  if (typeof val !== 'number' || !Number.isFinite(val)) {
    return '-'
  }
  return formatBillingCurrencyFromUSD(val, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
    locale,
  })
}

export function UpstreamAccountSyncPanel() {
  const { t, i18n } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  useSystemConfigStore((state) => state.config.currency)

  const [editingAccount, setEditingAccount] =
    useState<Partial<UpstreamAccountSource> | null>(null)
  const [deletingAccount, setDeletingAccount] =
    useState<UpstreamAccountSource | null>(null)
  const [syncingAccountId, setSyncingAccountId] = useState<string | null>(null)
  const [expandedAccountIds, setExpandedAccountIds] = useState<Set<string>>(
    () => new Set()
  )
  const [channelFilters, setChannelFilters] = useState<Record<string, string>>(
    {}
  )

  const toggleAccountExpanded = (id: string) => {
    setExpandedAccountIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const accountsQuery = useQuery({
    queryKey: ['upstream-accounts'],
    queryFn: async () => requireServerSuccess(await getUpstreamAccounts()),
  })

  const saveMutation = useMutation({
    mutationFn: async (payload: Partial<UpstreamAccountSource>) =>
      requireServerSuccess(await saveUpstreamAccount(payload)),
    onSuccess: () => {
      toast.success(t('Upstream account saved'))
      setEditingAccount(null)
      void queryClient.invalidateQueries({ queryKey: ['upstream-accounts'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to save upstream account'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: string) =>
      requireServerSuccess(await deleteUpstreamAccount(id)),
    onSuccess: () => {
      toast.success(t('Upstream account deleted'))
      setDeletingAccount(null)
      void queryClient.invalidateQueries({ queryKey: ['upstream-accounts'] })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to delete upstream account'))
    },
  })

  const syncMutation = useMutation({
    mutationFn: async (accountIds?: string[]) => {
      setSyncingAccountId(
        accountIds && accountIds.length === 1 ? accountIds[0] : 'ALL'
      )
      return requireServerSuccess(await syncUpstreamAccountsNow(accountIds))
    },
    onSuccess: (res) => {
      setSyncingAccountId(null)
      if (res.data?.error) {
        toast.error(res.data.summary || res.data.error)
      } else {
        toast.success(
          res.data?.summary || t('Upstream groups and channels synced')
        )
      }
      void queryClient.invalidateQueries({ queryKey: ['upstream-accounts'] })
      void queryClient.invalidateQueries({
        queryKey: ['channel-cron-sync-config'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['channel-model-price-table'],
      })
      void queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (err) => {
      setSyncingAccountId(null)
      handleServerError(err, t('Failed to sync upstream accounts'))
    },
  })

  const formatUnixTime = (ts: number) => {
    if (!ts || ts <= 0) {
      return t('Never synced')
    }
    return new Date(ts * 1000).toLocaleString(intlLocale)
  }

  if (accountsQuery.isLoading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-28 w-full' />
        <Skeleton className='h-80 w-full' />
      </div>
    )
  }

  const accounts: UpstreamAccountSource[] =
    accountsQuery.data?.data?.accounts ?? []
  const autoChannelsMap: Record<string, AutoProvisionedChannelMeta> =
    accountsQuery.data?.data?.auto_provisioned_channels ?? {}
  const autoChannelsList = Object.values(autoChannelsMap).sort(
    (a, b) => a.channel_id - b.channel_id
  )

  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader className='pb-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div>
              <CardTitle className='flex items-center gap-2 text-base'>
                <Layers aria-hidden='true' className='text-primary size-4' />
                <span>{t('Upstream Account & Group Channel Sync')}</span>
              </CardTitle>
              <CardDescription className='mt-1 text-xs'>
                {t(
                  'Sync all non-auto groups from upstream New API accounts into local channels (without creating upstream groups locally), auto-manage tokens by naming rule, and clean up auto-created channels when upstream groups are removed'
                )}
              </CardDescription>
            </div>
            <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => syncMutation.mutate(undefined)}
                disabled={syncMutation.isPending || accounts.length === 0}
                className='flex-1 sm:flex-initial'
              >
                <RefreshCw
                  aria-hidden='true'
                  className={`me-1.5 size-3.5 ${syncingAccountId === 'ALL' ? 'animate-spin' : ''}`}
                />
                {t('Sync All Enabled Upstreams')}
              </Button>
              <Button
                size='sm'
                onClick={() => setEditingAccount(createDefaultAccountForm())}
                className='flex-1 sm:flex-initial'
              >
                <Plus aria-hidden='true' className='me-1.5 size-3.5' />
                {t('Add Upstream Account')}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className='p-0'>
          {isMobile ? (
            <div className='flex flex-col gap-3 p-3'>
              {accounts.length === 0 ? (
                <div className='text-muted-foreground py-8 text-center text-sm'>
                  {t(
                    'No upstream accounts configured yet. Click "Add Upstream Account" to configure New API upstream sources.'
                  )}
                </div>
              ) : (
                accounts.map((acc) => {
                  const isSyncing = syncingAccountId === acc.id
                  return (
                    <div
                      key={acc.id}
                      className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
                    >
                      <div className='flex items-start justify-between gap-2'>
                        <div className='flex min-w-0 items-start gap-2.5'>
                          <Switch
                            checked={acc.enabled}
                            onCheckedChange={(checked) =>
                              saveMutation.mutate({
                                ...acc,
                                enabled: Boolean(checked),
                              })
                            }
                            disabled={saveMutation.isPending}
                            className='mt-0.5 shrink-0'
                          />
                          <div className='flex min-w-0 flex-col gap-0.5'>
                            <div className='flex flex-wrap items-center gap-1.5 font-medium'>
                              <Server
                                aria-hidden='true'
                                className='text-primary size-3.5 shrink-0'
                              />
                              <span className='truncate text-xs font-semibold'>
                                {acc.name}
                              </span>
                              <Badge
                                variant={
                                  resolveAccountSyncMode(acc) === 'lumiya'
                                    ? 'default'
                                    : 'secondary'
                                }
                                className='text-[10px]'
                              >
                                {resolveAccountSyncMode(acc) === 'lumiya'
                                  ? t('Lumiya mode')
                                  : t('Standard Mode')}
                              </Badge>
                              <Badge variant='outline' className='text-[10px]'>
                                UID: {acc.user_id || '-'}
                              </Badge>
                              <Badge
                                variant='outline'
                                className='font-mono text-[10px]'
                              >
                                {acc.currency || 'CNY'} (×{acc.exchange_rate || 5})
                              </Badge>
                              {typeof acc.balance_usd === 'number' &&
                              (acc.balance_usd > 0 || acc.last_sync_at > 0) ? (
                                <Badge
                                  variant='secondary'
                                  className='gap-1 font-mono text-[10px] text-emerald-600 dark:text-emerald-400'
                                >
                                  <Wallet
                                    aria-hidden='true'
                                    className='size-3'
                                  />
                                  {t('Balance')}:{' '}
                                  {formatBalanceUSD(acc.balance_usd)}
                                </Badge>
                              ) : null}
                            </div>
                            <span className='text-muted-foreground truncate font-mono text-[11px]'>
                              {acc.base_url}
                            </span>
                          </div>
                        </div>
                      </div>

                      <div className='bg-muted/20 grid grid-cols-1 gap-1.5 rounded-md p-2 text-xs'>
                        <div className='flex flex-wrap items-center justify-between gap-1'>
                          <span className='text-muted-foreground text-[11px]'>
                            Token:
                          </span>
                          <code className='bg-muted truncate rounded px-1.5 py-0.5 font-mono text-[11px]'>
                            {acc.token_name_rule}
                          </code>
                        </div>
                        <div className='flex flex-wrap items-center justify-between gap-1'>
                          <span className='text-muted-foreground text-[11px]'>
                            {t('Channel')}:
                          </span>
                          <code className='bg-muted truncate rounded px-1.5 py-0.5 font-mono text-[11px]'>
                            {acc.channel_name_rule}
                          </code>
                        </div>
                        <div className='flex flex-wrap items-center justify-between gap-1 pt-0.5'>
                          <Badge
                            variant='secondary'
                            className='w-fit gap-1 font-mono text-[10px]'
                          >
                            <Tag aria-hidden='true' className='size-3' />
                            {acc.channel_tag || `auto:${acc.name}`}
                          </Badge>
                          <span className='text-muted-foreground text-[11px]'>
                            {t('Local Group')}:{' '}
                            <span className='text-foreground font-mono font-medium'>
                              {acc.local_group || 'default'}
                            </span>
                          </span>
                        </div>
                      </div>

                      <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
                        <Badge
                          variant={
                            acc.clean_removed_groups ? 'default' : 'outline'
                          }
                          className='w-fit text-[10px]'
                        >
                          {acc.clean_removed_groups
                            ? t('Auto-Remove Disappeared Groups')
                            : t('Keep Removed Groups')}
                        </Badge>
                        {acc.last_sync_at > 0 ? (
                          <div className='flex items-center gap-1'>
                            {acc.last_status === 'success' ? (
                              <CheckCircle2
                                aria-hidden='true'
                                className='size-3.5 shrink-0 text-emerald-500'
                              />
                            ) : (
                              <XCircle
                                aria-hidden='true'
                                className='size-3.5 shrink-0 text-rose-500'
                              />
                            )}
                            <span className='font-mono text-[11px]'>
                              {formatUnixTime(acc.last_sync_at)}
                            </span>
                          </div>
                        ) : (
                          <span className='text-muted-foreground text-[11px]'>
                            {t('Never synced')}
                          </span>
                        )}
                      </div>

                      {acc.last_message ? (
                        <span className='text-muted-foreground truncate font-mono text-[11px]'>
                          {acc.last_message}
                        </span>
                      ) : null}

                      {(() => {
                        const accChannels = autoChannelsList.filter(
                          (m) =>
                            m.source_id === acc.id ||
                            m.source_name.toLowerCase() ===
                              acc.name.toLowerCase()
                        )
                        if (accChannels.length === 0) return null
                        const isExpanded = expandedAccountIds.has(acc.id)
                        const keyword = (channelFilters[acc.id] ?? '')
                          .trim()
                          .toLowerCase()
                        const visibleChannels = keyword
                          ? accChannels.filter(
                              (ch) =>
                                String(ch.channel_id).includes(keyword) ||
                                (ch.channel_name || '')
                                  .toLowerCase()
                                  .includes(keyword) ||
                                (ch.upstream_group || '')
                                  .toLowerCase()
                                  .includes(keyword)
                            )
                          : accChannels
                        return (
                          <Collapsible
                            open={isExpanded}
                            onOpenChange={() => toggleAccountExpanded(acc.id)}
                            className='border-border/50 bg-muted/15 rounded-md border p-2'
                          >
                            <div className='flex items-center justify-between gap-2 text-[11px]'>
                              <CollapsibleTrigger
                                render={
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    className='h-6 gap-1 px-1.5 text-[11px]'
                                  />
                                }
                              >
                                {isExpanded ? (
                                  <ChevronDown
                                    aria-hidden='true'
                                    className='size-3.5'
                                  />
                                ) : (
                                  <ChevronRight
                                    aria-hidden='true'
                                    className='size-3.5'
                                  />
                                )}
                                <Wallet
                                  aria-hidden='true'
                                  className='size-3'
                                />
                                <span>
                                  {t('Synced Channels')} ({accChannels.length})
                                </span>
                              </CollapsibleTrigger>
                              {typeof acc.used_quota_usd === 'number' &&
                              acc.used_quota_usd > 0 ? (
                                <span className='text-muted-foreground font-mono text-[10px]'>
                                  {t('Used:')}{' '}
                                  {formatBalanceUSD(acc.used_quota_usd)}
                                </span>
                              ) : null}
                            </div>
                            <CollapsibleContent className='mt-2 flex flex-col gap-2'>
                              {accChannels.length > 6 ? (
                                <div className='relative'>
                                  <Search
                                    aria-hidden='true'
                                    className='text-muted-foreground pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2'
                                  />
                                  <Input
                                    value={channelFilters[acc.id] ?? ''}
                                    onChange={(e) =>
                                      setChannelFilters((prev) => ({
                                        ...prev,
                                        [acc.id]: e.target.value,
                                      }))
                                    }
                                    placeholder={t(
                                      'Filter synced channels by #ID, name, or group...'
                                    )}
                                    className='h-7 pl-7 text-xs'
                                  />
                                </div>
                              ) : null}
                              <div className='grid max-h-60 grid-cols-1 gap-1.5 overflow-y-auto pr-0.5'>
                                {visibleChannels.map((chMeta) => (
                                  <div
                                    key={chMeta.channel_id}
                                    className='border-border/50 bg-background/80 flex items-center justify-between gap-2 rounded-md border px-2 py-1.5 font-mono text-[11px]'
                                  >
                                    <div className='flex min-w-0 items-center gap-1.5'>
                                      <span className='text-primary shrink-0 font-semibold'>
                                        #{chMeta.channel_id}
                                      </span>
                                      <span className='truncate'>
                                        {chMeta.channel_name ||
                                          chMeta.upstream_group}
                                      </span>
                                    </div>
                                    <span className='shrink-0 font-semibold text-emerald-600 dark:text-emerald-400'>
                                      {formatBalanceUSD(chMeta.balance ?? 0)}
                                    </span>
                                  </div>
                                ))}
                              </div>
                            </CollapsibleContent>
                          </Collapsible>
                        )
                      })()}

                      <div className='border-border/40 flex items-center justify-end gap-1.5 border-t pt-2'>
                        <Button
                          variant='outline'
                          size='sm'
                          className='h-7 px-2.5 text-xs'
                          onClick={() => syncMutation.mutate([acc.id])}
                          disabled={syncMutation.isPending}
                        >
                          {isSyncing ? (
                            <RefreshCw
                              aria-hidden='true'
                              className='me-1 size-3 animate-spin'
                            />
                          ) : (
                            <Play aria-hidden='true' className='me-1 size-3' />
                          )}
                          {t('Sync Now')}
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-7 px-2 text-xs'
                          aria-label={`${t('Edit Upstream Account')} ${acc.name}`}
                          onClick={() =>
                            setEditingAccount({
                              ...acc,
                              access_token: '',
                            })
                          }
                        >
                          <Edit3 aria-hidden='true' className='size-3.5' />
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          className='text-destructive hover:text-destructive h-7 px-2 text-xs'
                          aria-label={`${t('Delete Upstream Account')} ${acc.name}`}
                          onClick={() => setDeletingAccount(acc)}
                        >
                          <Trash2 aria-hidden='true' className='size-3.5' />
                        </Button>
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          ) : (
            <div className='overflow-x-auto'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-10' />
                    <TableHead className='w-16'>{t('Enabled')}</TableHead>
                    <TableHead>{t('Upstream Name & URL')}</TableHead>
                    <TableHead>{t('Token & Channel Naming Rules')}</TableHead>
                    <TableHead>{t('Auto Tag & Local Group')}</TableHead>
                    <TableHead>{t('Auto Cleanup')}</TableHead>
                    <TableHead>{t('Last Sync & Result')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accounts.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={8}
                        className='text-muted-foreground h-24 text-center text-sm'
                      >
                        {t(
                          'No upstream accounts configured yet. Click "Add Upstream Account" to configure New API upstream sources.'
                        )}
                      </TableCell>
                    </TableRow>
                  ) : (
                    accounts.map((acc) => {
                      const isSyncing = syncingAccountId === acc.id
                      const accChannels = autoChannelsList.filter(
                        (m) =>
                          m.source_id === acc.id ||
                          m.source_name.toLowerCase() === acc.name.toLowerCase()
                      )
                      const isExpanded = expandedAccountIds.has(acc.id)
                      const keyword = (channelFilters[acc.id] ?? '')
                        .trim()
                        .toLowerCase()
                      const visibleChannels = keyword
                        ? accChannels.filter(
                            (ch) =>
                              String(ch.channel_id).includes(keyword) ||
                              (ch.channel_name || '')
                                .toLowerCase()
                                .includes(keyword) ||
                              (ch.upstream_group || '')
                                .toLowerCase()
                                .includes(keyword)
                          )
                        : accChannels

                      return (
                        <Fragment key={acc.id}>
                          <TableRow>
                            <TableCell className='pr-0'>
                              {accChannels.length > 0 ? (
                                <Button
                                  type='button'
                                  variant='ghost'
                                  size='sm'
                                  aria-expanded={isExpanded}
                                  aria-label={`${isExpanded ? t('Collapse details') : t('Expand details')} ${acc.name}`}
                                  onClick={() => toggleAccountExpanded(acc.id)}
                                  className='size-7 p-0'
                                >
                                  {isExpanded ? (
                                    <ChevronDown
                                      aria-hidden='true'
                                      className='size-4'
                                    />
                                  ) : (
                                    <ChevronRight
                                      aria-hidden='true'
                                      className='size-4'
                                    />
                                  )}
                                </Button>
                              ) : null}
                            </TableCell>
                            <TableCell>
                            <Switch
                              checked={acc.enabled}
                              onCheckedChange={(checked) =>
                                saveMutation.mutate({
                                  ...acc,
                                  enabled: Boolean(checked),
                                })
                              }
                              disabled={saveMutation.isPending}
                            />
                          </TableCell>
                          <TableCell>
                            <div className='flex flex-col gap-0.5'>
                              <div className='flex items-center gap-1.5 font-medium'>
                                <Server
                                  aria-hidden='true'
                                  className='text-primary size-3.5'
                                />
                                <span>{acc.name}</span>
                                <Badge
                                  variant={
                                    resolveAccountSyncMode(acc) === 'lumiya'
                                      ? 'default'
                                      : 'secondary'
                                  }
                                  className='text-[10px]'
                                >
                                  {resolveAccountSyncMode(acc) === 'lumiya'
                                    ? t('Lumiya mode')
                                    : t('Standard Mode')}
                                </Badge>
                                <Badge
                                  variant='outline'
                                  className='text-[10px]'
                                >
                                  UID: {acc.user_id || '-'}
                                </Badge>
                                <Badge
                                  variant='outline'
                                  className='font-mono text-[10px]'
                                >
                                  {acc.currency || 'CNY'} (×{acc.exchange_rate || 5})
                                </Badge>
                                {typeof acc.balance_usd === 'number' &&
                                (acc.balance_usd > 0 || acc.last_sync_at > 0) ? (
                                  <Badge
                                    variant='secondary'
                                    className='gap-1 font-mono text-[10px] text-emerald-600 dark:text-emerald-400'
                                  >
                                    <Wallet
                                      aria-hidden='true'
                                      className='size-3'
                                    />
                                    {t('Balance')}:{' '}
                                    {formatBalanceUSD(acc.balance_usd)}
                                  </Badge>
                                ) : null}
                                {typeof acc.used_quota_usd === 'number' &&
                                acc.used_quota_usd > 0 ? (
                                  <Badge
                                    variant='outline'
                                    className='font-mono text-[10px]'
                                  >
                                    {t('Used:')}{' '}
                                    {formatBalanceUSD(acc.used_quota_usd)}
                                  </Badge>
                                ) : null}
                              </div>
                              <span className='text-muted-foreground font-mono text-xs'>
                                {acc.base_url}
                              </span>
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className='flex flex-col gap-1 text-xs'>
                              <div className='flex items-center gap-1.5'>
                                <span className='text-muted-foreground'>
                                  Token:
                                </span>
                                <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-[11px]'>
                                  {acc.token_name_rule}
                                </code>
                              </div>
                              <div className='flex items-center gap-1.5'>
                                <span className='text-muted-foreground'>
                                  {t('Channel')}:
                                </span>
                                <code className='bg-muted rounded px-1.5 py-0.5 font-mono text-[11px]'>
                                  {acc.channel_name_rule}
                                </code>
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className='flex flex-col gap-1'>
                              <Badge
                                variant='secondary'
                                className='w-fit gap-1 font-mono text-[11px]'
                              >
                                <Tag aria-hidden='true' className='size-3' />
                                {acc.channel_tag || `auto:${acc.name}`}
                              </Badge>
                              <span className='text-muted-foreground text-xs'>
                                {t('Local Group')}:{' '}
                                <span className='text-foreground font-mono font-medium'>
                                  {acc.local_group || 'default'}
                                </span>
                              </span>
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className='flex flex-col gap-1 text-xs'>
                              <Badge
                                variant={
                                  acc.clean_removed_groups
                                    ? 'default'
                                    : 'outline'
                                }
                                className='w-fit text-[10px]'
                              >
                                {acc.clean_removed_groups
                                  ? t('Auto-Remove Disappeared Groups')
                                  : t('Keep Removed Groups')}
                              </Badge>
                              {acc.adopt_existing_channel ? (
                                <span className='text-muted-foreground text-[11px]'>
                                  {t('Adopts matched channels')}
                                </span>
                              ) : null}
                            </div>
                          </TableCell>
                          <TableCell className='text-xs'>
                            {acc.last_sync_at > 0 ? (
                              <div className='flex flex-col gap-1'>
                                <div className='flex items-center gap-1.5'>
                                  {acc.last_status === 'success' ? (
                                    <CheckCircle2
                                      aria-hidden='true'
                                      className='size-3.5 text-emerald-500'
                                    />
                                  ) : (
                                    <XCircle
                                      aria-hidden='true'
                                      className='size-3.5 text-rose-500'
                                    />
                                  )}
                                  <span className='font-mono'>
                                    {formatUnixTime(acc.last_sync_at)}
                                  </span>
                                </div>
                                {acc.last_message ? (
                                  <span className='text-muted-foreground max-w-xs truncate font-mono text-[11px]'>
                                    {acc.last_message}
                                  </span>
                                ) : null}
                                {accChannels.length > 0 ? (
                                  <Button
                                    type='button'
                                    variant='secondary'
                                    size='sm'
                                    onClick={() =>
                                      toggleAccountExpanded(acc.id)
                                    }
                                    className='mt-0.5 h-6 w-fit gap-1.5 px-2 text-[11px]'
                                  >
                                    {isExpanded ? (
                                      <ChevronDown
                                        aria-hidden='true'
                                        className='size-3'
                                      />
                                    ) : (
                                      <ChevronRight
                                        aria-hidden='true'
                                        className='size-3'
                                      />
                                    )}
                                    <span>
                                      {t('Synced Channels')} (
                                      {accChannels.length})
                                    </span>
                                  </Button>
                                ) : null}
                              </div>
                            ) : (
                              <span className='text-muted-foreground'>
                                {t('Never synced')}
                              </span>
                            )}
                          </TableCell>
                          <TableCell className='text-right'>
                            <div className='flex items-center justify-end gap-1'>
                              <Button
                                variant='outline'
                                size='sm'
                                className='h-7 px-2 text-xs'
                                onClick={() => syncMutation.mutate([acc.id])}
                                disabled={syncMutation.isPending}
                              >
                                {isSyncing ? (
                                  <RefreshCw
                                    aria-hidden='true'
                                    className='me-1 size-3 animate-spin'
                                  />
                                ) : (
                                  <Play
                                    aria-hidden='true'
                                    className='me-1 size-3'
                                  />
                                )}
                                {t('Sync Now')}
                              </Button>
                              <Button
                                variant='ghost'
                                size='sm'
                                className='h-7 px-2 text-xs'
                                aria-label={`${t('Edit Upstream Account')} ${acc.name}`}
                                onClick={() =>
                                  setEditingAccount({
                                    ...acc,
                                    access_token: '',
                                  })
                                }
                              >
                                <Edit3
                                  aria-hidden='true'
                                  className='size-3.5'
                                />
                              </Button>
                              <Button
                                variant='ghost'
                                size='sm'
                                className='text-destructive hover:text-destructive h-7 px-2 text-xs'
                                aria-label={`${t('Delete Upstream Account')} ${acc.name}`}
                                onClick={() => setDeletingAccount(acc)}
                              >
                                <Trash2
                                  aria-hidden='true'
                                  className='size-3.5'
                                />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                        {isExpanded && accChannels.length > 0 ? (
                          <TableRow className='bg-muted/20 hover:bg-muted/20'>
                            <TableCell colSpan={8} className='px-6 py-4'>
                              <div className='flex flex-col gap-3'>
                                <div className='flex flex-wrap items-center justify-between gap-2'>
                                  <div className='flex items-center gap-2 text-xs font-medium'>
                                    <Wallet
                                      aria-hidden='true'
                                      className='text-primary size-3.5'
                                    />
                                    <span>
                                      {t('Synced Channels')} (
                                      {visibleChannels.length}/
                                      {accChannels.length})
                                    </span>
                                  </div>
                                  {accChannels.length > 6 ? (
                                    <div className='relative w-full sm:w-72'>
                                      <Search
                                        aria-hidden='true'
                                        className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2'
                                      />
                                      <Input
                                        value={channelFilters[acc.id] ?? ''}
                                        onChange={(e) =>
                                          setChannelFilters((prev) => ({
                                            ...prev,
                                            [acc.id]: e.target.value,
                                          }))
                                        }
                                        placeholder={t(
                                          'Filter synced channels by #ID, name, or group...'
                                        )}
                                        className='h-8 pl-8 text-xs'
                                      />
                                    </div>
                                  ) : null}
                                </div>
                                <div className='grid max-h-80 grid-cols-1 gap-2 overflow-y-auto pr-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'>
                                  {visibleChannels.map((chMeta) => (
                                    <div
                                      key={chMeta.channel_id}
                                      className='border-border/60 bg-background/80 flex items-center justify-between gap-2 rounded-lg border px-2.5 py-2 font-mono text-xs'
                                    >
                                      <div className='flex min-w-0 items-center gap-1.5'>
                                        <span className='text-primary shrink-0 font-semibold'>
                                          #{chMeta.channel_id}
                                        </span>
                                        <span
                                          className='truncate'
                                          title={
                                            chMeta.channel_name ||
                                            chMeta.upstream_group
                                          }
                                        >
                                          {chMeta.channel_name ||
                                            chMeta.upstream_group}
                                        </span>
                                      </div>
                                      <span className='shrink-0 font-semibold text-emerald-600 dark:text-emerald-400'>
                                        {formatBalanceUSD(chMeta.balance ?? 0)}
                                      </span>
                                    </div>
                                  ))}
                                </div>
                              </div>
                            </TableCell>
                          </TableRow>
                        ) : null}
                        </Fragment>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {autoChannelsList.length > 0 ? (
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='flex items-center gap-2 text-sm'>
              <Tag aria-hidden='true' className='text-primary size-4' />
              <span>
                {t('Auto-Tagged Upstream Channels')} ({autoChannelsList.length})
              </span>
            </CardTitle>
            <CardDescription className='text-xs'>
              {t(
                'Channels marked as auto-provisioned will be automatically updated or removed when their corresponding upstream group changes or disappears'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className='flex flex-wrap gap-2'>
              {autoChannelsList.map((meta) => (
                <Badge
                  key={meta.channel_id}
                  variant='outline'
                  className='gap-1.5 px-2.5 py-1 font-mono text-xs'
                >
                  <span className='text-primary font-semibold'>
                    #{meta.channel_id}
                  </span>
                  <span>{meta.channel_name || meta.source_name}</span>
                  <span className='text-muted-foreground'>/</span>
                  <span>{meta.upstream_group}</span>
                  <span className='text-muted-foreground'>|</span>
                  <span className='flex items-center gap-1 font-semibold text-emerald-600 dark:text-emerald-400'>
                    <Wallet aria-hidden='true' className='size-3' />
                    {formatBalanceUSD(meta.balance ?? 0)}
                  </span>
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : null}

      <ChannelClassifierPanel />

      <Dialog
        open={Boolean(editingAccount)}
        onOpenChange={(open) => {
          if (!open) setEditingAccount(null)
        }}
        title={
          editingAccount?.id
            ? t('Edit Upstream Account')
            : t('Add Upstream Account')
        }
        description={t(
          'Configure an upstream New API panel account to automatically sync all non-auto groups into local channels'
        )}
        footer={
          <div className='flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row sm:items-center sm:justify-end'>
            <Button
              variant='outline'
              onClick={() => setEditingAccount(null)}
              disabled={saveMutation.isPending}
              className='w-full sm:w-auto'
            >
              {t('Cancel')}
            </Button>
            <Button
              onClick={() => {
                if (editingAccount) {
                  saveMutation.mutate(editingAccount)
                }
              }}
              disabled={saveMutation.isPending}
              className='w-full sm:w-auto'
            >
              {t('Save Upstream Account')}
            </Button>
          </div>
        }
      >
        {editingAccount ? (
          <div className='grid max-h-[70dvh] gap-4 overflow-y-auto px-0.5 py-1'>
              <div className='grid gap-4 sm:grid-cols-3'>
                <div className='space-y-1.5'>
                  <Label>{t('Upstream Label Name')}</Label>
                  <Input
                    value={editingAccount.name ?? ''}
                    onChange={(e) => {
                      const nextName = e.target.value
                      const lower =
                        `${nextName} ${editingAccount.base_url ?? ''}`.toLowerCase()
                      const nextMode =
                        lower.includes('lumiya') || lower.includes('露米娅')
                          ? 'lumiya'
                          : (editingAccount.sync_mode ?? 'standard')
                      setEditingAccount({
                        ...editingAccount,
                        name: nextName,
                        sync_mode: nextMode,
                      })
                    }}
                    placeholder='e.g. neartoken / moyuu / lumiya'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Upstream Base URL')}</Label>
                  <Input
                    value={editingAccount.base_url ?? ''}
                    onChange={(e) => {
                      const nextUrl = e.target.value
                      const lower =
                        `${editingAccount.name ?? ''} ${nextUrl}`.toLowerCase()
                      const nextMode =
                        lower.includes('lumiya') || lower.includes('露米娅')
                          ? 'lumiya'
                          : (editingAccount.sync_mode ?? 'standard')
                      setEditingAccount({
                        ...editingAccount,
                        base_url: nextUrl,
                        sync_mode: nextMode,
                      })
                    }}
                    placeholder='https://llm.pko.svcfusion.com'
                    className='font-mono'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Sync Mode')}</Label>
                  <Select
                    value={resolveAccountSyncMode(editingAccount)}
                    onValueChange={(val) =>
                      setEditingAccount({
                        ...editingAccount,
                        sync_mode: val === 'lumiya' ? 'lumiya' : 'standard',
                      })
                    }
                  >
                    <SelectTrigger className='w-full text-xs'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='standard'>
                        {t('Standard Mode')}
                      </SelectItem>
                      <SelectItem value='lumiya'>{t('Lumiya mode')}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className='grid gap-4 sm:grid-cols-2'>
                <div className='space-y-1.5'>
                  <Label>{t('Upstream User ID (New-Api-User)')}</Label>
                  <Input
                    value={editingAccount.user_id ?? ''}
                    onChange={(e) =>
                      setEditingAccount({
                        ...editingAccount,
                        user_id: e.target.value,
                      })
                    }
                    placeholder='e.g. 3'
                    className='font-mono'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Upstream Account REST API Key')}</Label>
                  <Input
                    type='password'
                    value={editingAccount.access_token ?? ''}
                    onChange={(e) =>
                      setEditingAccount({
                        ...editingAccount,
                        access_token: e.target.value,
                      })
                    }
                    placeholder={
                      editingAccount.has_access_token
                        ? t('Leave blank to keep existing key')
                        : 'nap_... or system access token'
                    }
                    className='font-mono'
                  />
                </div>
              </div>

              <div className='grid gap-4 sm:grid-cols-2'>
                <div className='space-y-1.5'>
                  <Label>
                    {t('Upstream Token Naming Rule ({name}, {group}, {ratio})')}
                  </Label>
                  <Input
                    value={editingAccount.token_name_rule ?? ''}
                    onChange={(e) =>
                      setEditingAccount({
                        ...editingAccount,
                        token_name_rule: e.target.value,
                      })
                    }
                    placeholder='new_api-{group}-{ratio}'
                    className='font-mono text-xs'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>
                    {t('Local Channel Naming Rule ({name}, {group}, {ratio})')}
                  </Label>
                  <Input
                    value={editingAccount.channel_name_rule ?? ''}
                    onChange={(e) =>
                      setEditingAccount({
                        ...editingAccount,
                        channel_name_rule: e.target.value,
                      })
                    }
                    placeholder='{name}-{group}-{ratio}'
                    className='font-mono text-xs'
                  />
                </div>
              </div>

              <div className='grid gap-4 sm:grid-cols-3'>
                <div className='space-y-1.5'>
                  <Label>{t('Auto-Created Channel Tag')}</Label>
                  <Input
                    value={editingAccount.channel_tag ?? ''}
                    onChange={(e) =>
                      setEditingAccount({
                        ...editingAccount,
                        channel_tag: e.target.value,
                      })
                    }
                    placeholder={`auto:${editingAccount.name || 'upstream'}`}
                    className='font-mono text-xs'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Local Target Group')}</Label>
                  <Input
                    value={editingAccount.local_group ?? 'default'}
                    onChange={(e) =>
                      setEditingAccount({
                        ...editingAccount,
                        local_group: e.target.value,
                      })
                    }
                    placeholder='default'
                    className='font-mono text-xs'
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label>{t('Upstream Currency & Exchange Rate')}</Label>
                  <div className='flex items-center gap-2'>
                    <Select
                      value={editingAccount.currency ?? 'CNY'}
                      onValueChange={(val) => {
                        const c = String(val)
                        let rate = editingAccount.exchange_rate ?? 5
                        if (c === 'USD') rate = 35
                        if (c === 'CNY') rate = 5
                        if (c === 'TWD') rate = 1
                        setEditingAccount({
                          ...editingAccount,
                          currency: c,
                          exchange_rate: rate,
                        })
                      }}
                    >
                      <SelectTrigger className='w-28 text-xs'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='CNY'>CNY (¥ 人民幣)</SelectItem>
                        <SelectItem value='USD'>USD ($ 美金)</SelectItem>
                        <SelectItem value='TWD'>TWD (NT$ 台幣)</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input
                      type='number'
                      step='0.1'
                      value={editingAccount.exchange_rate ?? 5}
                      onChange={(e) =>
                        setEditingAccount({
                          ...editingAccount,
                          exchange_rate: Number(e.target.value) || 1.0,
                        })
                      }
                      className='w-24 font-mono text-xs'
                    />
                  </div>
                </div>
              </div>

              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5 pe-2'>
                    <Label className='text-xs font-medium'>
                      {t('Auto-Remove Disappeared Groups')}
                    </Label>
                    <p className='text-muted-foreground text-[11px]'>
                      {t(
                        'Automatically delete auto-created channels when their upstream group disappears'
                      )}
                    </p>
                  </div>
                  <Switch
                    checked={editingAccount.clean_removed_groups ?? true}
                    onCheckedChange={(checked) =>
                      setEditingAccount({
                        ...editingAccount,
                        clean_removed_groups: Boolean(checked),
                      })
                    }
                  />
                </div>
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5 pe-2'>
                    <Label className='text-xs font-medium'>
                      {t('Tag & Adopt Matched Existing Channels')}
                    </Label>
                    <p className='text-muted-foreground text-[11px]'>
                      {t(
                        'Mark existing channels matching this upstream as auto-managed'
                      )}
                    </p>
                  </div>
                  <Switch
                    checked={editingAccount.adopt_existing_channel ?? true}
                    onCheckedChange={(checked) =>
                      setEditingAccount({
                        ...editingAccount,
                        adopt_existing_channel: Boolean(checked),
                      })
                    }
                  />
                </div>
              </div>
            </div>
        ) : null}
      </Dialog>

      <ConfirmDialog
        open={Boolean(deletingAccount)}
        onOpenChange={(open) => {
          if (!open) setDeletingAccount(null)
        }}
        title={t('Delete Upstream Account')}
        desc={t(
          'Are you sure you want to remove this upstream account configuration? Existing channels will not be deleted.'
        )}
        confirmText={t('Delete')}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deletingAccount) {
            deleteMutation.mutate(deletingAccount.id)
          }
        }}
      />
    </div>
  )
}
