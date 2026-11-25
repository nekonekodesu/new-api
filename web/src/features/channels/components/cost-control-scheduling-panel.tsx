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
import {
  AlertTriangle,
  CheckCircle2,
  DollarSign,
  Filter,
  Network,
  RefreshCw,
  Scale,
  SlidersHorizontal,
  TrendingDown,
  TrendingUp,
  Wallet,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import {
  formatBillingAmountUSD,
  getCurrencyDisplay,
} from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type {
  ChannelBrief,
  CostControlUpstreamAlloc,
  CostControlWindow,
  GroupCostControlPolicy,
  GroupRealtimeProfitSnapshot,
  UpstreamSourceBrief,
} from '../api'
import {
  DIRECT_UPSTREAM_ID,
  evaluateRealtimeProfitWithPolicy,
  resolveEffectiveChannelUpstream,
  resolveUpstreamAlloc,
} from '../lib/cost-control'
import { NumericSpinnerInput } from './numeric-spinner-input'

const WINDOW_OPTIONS: Array<{ value: CostControlWindow; labelKey: string }> = [
  { value: '24h', labelKey: 'Rolling 24h' },
  { value: 'today', labelKey: 'Today' },
  { value: '7d', labelKey: 'Rolling 7d' },
  { value: '30d', labelKey: 'Rolling 30d' },
]

const COST_RATIO_PRESETS = [70, 80, 90, 100]
const SENSITIVITY_PRESETS = [
  { value: 0.5, labelKey: 'Gentle (0.5x)' },
  { value: 1, labelKey: 'Balanced (1x)' },
  { value: 2, labelKey: 'Aggressive (2x)' },
]
const UPSTREAM_WEIGHT_PRESETS = [25, 50, 100, 200]

function getBillingCurrencyMeta(): { symbol: string; exchangeRate: number } {
  const { meta } = getCurrencyDisplay()
  if (meta.kind === 'tokens') {
    return { symbol: '$', exchangeRate: 1 }
  }
  const rate =
    Number.isFinite(meta.exchangeRate) && meta.exchangeRate > 0
      ? meta.exchangeRate
      : 1
  return { symbol: meta.symbol || '$', exchangeRate: rate }
}

export function CostControlProfitPolicyCard(props: {
  group: string
  groupRatio: number
  policy: GroupCostControlPolicy
  snapshot?: GroupRealtimeProfitSnapshot
  isRefreshing: boolean
  onRefresh: () => void
  onChangePolicy: (
    updater: (prev: GroupCostControlPolicy) => GroupCostControlPolicy
  ) => void
}) {
  const { t, i18n } = useTranslation()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage ?? i18n.language)
  const currencyMeta = getBillingCurrencyMeta()

  const live = useMemo(
    () => evaluateRealtimeProfitWithPolicy(props.snapshot, props.policy),
    [props.snapshot, props.policy]
  )

  const displayFloorVal = useMemo(() => {
    const converted = props.policy.min_net_profit_usd * currencyMeta.exchangeRate
    return Math.round(converted * 10000) / 10000
  }, [props.policy.min_net_profit_usd, currencyMeta.exchangeRate])

  const [draftFloorText, setDraftFloorText] = useState('')
  const [editingFloor, setEditingFloor] = useState(false)
  const floorInputValue = editingFloor
    ? draftFloorText
    : String(displayFloorVal)

  const targetMarginPct = Math.round((100 - props.policy.max_cost_ratio_pct) * 10) / 10
  const costBarWidthPct = Math.min(
    100,
    Math.max(
      0,
      props.policy.max_cost_ratio_pct > 0
        ? (live.costRatioPct / props.policy.max_cost_ratio_pct) * 100
        : 0
    )
  )

  let profitColorClass = 'text-emerald-600 dark:text-emerald-400'
  if (live.netProfitUSD < -1e-9) {
    profitColorClass = 'text-destructive'
  }

  let cushionColorClass = 'text-emerald-600 dark:text-emerald-400'
  if (live.isOverCost) {
    cushionColorClass = 'text-amber-600 dark:text-amber-400'
  }

  return (
    <div className='border-border/60 bg-card flex flex-col gap-4 rounded-xl border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='space-y-1'>
          <div className='flex flex-wrap items-center gap-2'>
            <Scale aria-hidden='true' className='text-primary size-4' />
            <h3 className='text-sm font-semibold'>
              {t('Real-Time Net Profit & Cost Control Policy')}
            </h3>
            {live.isOverCost ? (
              <Badge variant='destructive' className='gap-1 text-[11px]'>
                <AlertTriangle aria-hidden='true' className='size-3' />
                <span>
                  {t('Over Cost · Auto Low-Cost Shift')} (
                  {live.divertProbPct.toFixed(0)}%)
                </span>
              </Badge>
            ) : (
              <Badge
                variant='secondary'
                className='gap-1 border-emerald-500/30 bg-emerald-500/10 text-[11px] text-emerald-700 dark:text-emerald-300'
              >
                <CheckCircle2 aria-hidden='true' className='size-3' />
                <span>{t('Within Cost · Priority Ratio Active')}</span>
              </Badge>
            )}
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Tracks real-time group revenue vs upstream cost. Within budget, requests follow Upstream Allocation and Channel Priority Ratios; when over cost, traffic shifts proportionally to profitable channels.'
            )}
          </p>
        </div>

        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={props.onRefresh}
          disabled={props.isRefreshing}
          className='w-full gap-1.5 sm:w-auto'
        >
          <RefreshCw
            aria-hidden='true'
            className={cn('size-3.5', props.isRefreshing && 'animate-spin')}
          />
          <span>{t('Refresh Profit')}</span>
        </Button>
      </div>

      {/* Real-time Profit KPI Grid */}
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        <div className='border-border/60 bg-background/70 flex flex-col justify-between gap-1.5 rounded-xl border p-3'>
          <div className='text-muted-foreground flex items-center justify-between text-xs'>
            <span>{t('Group Revenue (Window)')}</span>
            <DollarSign
              aria-hidden='true'
              className='size-3.5 text-emerald-500'
            />
          </div>
          <div className='font-mono text-lg font-bold'>
            {formatBillingAmountUSD(live.revenueUSD, intlLocale)}
          </div>
          <div className='text-muted-foreground flex items-center justify-between text-[11px]'>
            <span>
              {formatNumber(live.totalCalls, intlLocale)} {t('Calls')}
            </span>
            <span>
              {t('Group Ratio')}: {props.groupRatio}x
            </span>
          </div>
        </div>

        <div className='border-border/60 bg-background/70 flex flex-col justify-between gap-1.5 rounded-xl border p-3'>
          <div className='text-muted-foreground flex items-center justify-between text-xs'>
            <span>{t('Upstream Cost & Cost Ratio')}</span>
            <Wallet aria-hidden='true' className='text-primary size-3.5' />
          </div>
          <div className='flex items-baseline justify-between gap-2'>
            <span className='font-mono text-lg font-bold'>
              {formatBillingAmountUSD(live.costUSD, intlLocale)}
            </span>
            <span className='text-muted-foreground font-mono text-xs'>
              {live.costRatioPct.toFixed(1)}% / {props.policy.max_cost_ratio_pct}%
            </span>
          </div>
          <div className='bg-muted h-1.5 w-full overflow-hidden rounded-full'>
            <div
              className={cn(
                'h-full rounded-full transition-all',
                live.isOverCost ? 'bg-destructive' : 'bg-primary'
              )}
              style={{ width: `${costBarWidthPct}%` }}
            />
          </div>
        </div>

        <div className='border-border/60 bg-background/70 flex flex-col justify-between gap-1.5 rounded-xl border p-3'>
          <div className='text-muted-foreground flex items-center justify-between text-xs'>
            <span>{t('Real-Time Net Profit')}</span>
            {live.netProfitUSD >= 0 ? (
              <TrendingUp
                aria-hidden='true'
                className='size-3.5 text-emerald-500'
              />
            ) : (
              <TrendingDown
                aria-hidden='true'
                className='text-destructive size-3.5'
              />
            )}
          </div>
          <div className={cn('font-mono text-lg font-bold', profitColorClass)}>
            {live.netProfitUSD > 0 ? '+' : ''}
            {formatBillingAmountUSD(live.netProfitUSD, intlLocale)}
          </div>
          <div className='flex items-center justify-between text-[11px]'>
            <span className='text-muted-foreground'>{t('Net Margin')}</span>
            <Badge
              variant={live.netMarginPct >= 0 ? 'secondary' : 'destructive'}
              className='px-1.5 py-0 font-mono text-[10px]'
            >
              {live.netMarginPct.toFixed(1)}%
            </Badge>
          </div>
        </div>

        <div className='border-border/60 bg-background/70 flex flex-col justify-between gap-1.5 rounded-xl border p-3'>
          <div className='text-muted-foreground flex items-center justify-between text-xs'>
            <span>{t('Cost Budget Headroom')}</span>
            <SlidersHorizontal
              aria-hidden='true'
              className='text-muted-foreground size-3.5'
            />
          </div>
          <div className={cn('font-mono text-lg font-bold', cushionColorClass)}>
            {live.profitCushionUSD > 0 ? '+' : ''}
            {formatBillingAmountUSD(live.profitCushionUSD, intlLocale)}
          </div>
          <div className='text-muted-foreground truncate text-[11px]'>
            {live.isOverCost
              ? t('Shifting {{pct}}% requests to low-cost channels', {
                  pct: live.divertProbPct.toFixed(0),
                })
              : t('Allowed Cost: {{amount}}', {
                  amount: formatBillingAmountUSD(live.allowedCostUSD, intlLocale),
                })}
          </div>
        </div>
      </div>

      {/* Policy Threshold Controls */}
      <div className='border-border/60 bg-muted/20 grid grid-cols-1 gap-3.5 rounded-xl border p-3.5 sm:grid-cols-2 xl:grid-cols-4'>
        {/* Window Selector */}
        <div className='flex flex-col gap-1.5'>
          <span className='text-xs font-medium'>
            {t('Profit Tracking Window')}
          </span>
          <div className='grid grid-cols-2 gap-1.5 sm:grid-cols-4 xl:grid-cols-2 2xl:grid-cols-4'>
            {WINDOW_OPTIONS.map((opt) => {
              const active = props.policy.window === opt.value
              return (
                <button
                  key={opt.value}
                  type='button'
                  onClick={() =>
                    props.onChangePolicy((prev) => ({
                      ...prev,
                      window: opt.value,
                    }))
                  }
                  className={cn(
                    'rounded-lg border px-2 py-1.5 text-center text-[11px] font-medium transition-colors',
                    active
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-border/60 bg-background hover:bg-muted/60'
                  )}
                >
                  {t(opt.labelKey)}
                </button>
              )
            })}
          </div>
          <span className='text-muted-foreground text-[11px]'>
            {t('Rolling window for net profit evaluation')}
          </span>
        </div>

        {/* Max Cost Ratio % */}
        <div className='flex flex-col gap-1.5'>
          <div className='flex items-center justify-between gap-1'>
            <span className='text-xs font-medium'>
              {t('Max Cost Ratio (%)')}
            </span>
            <span className='text-muted-foreground font-mono text-[11px]'>
              {t('Margin >= {{pct}}%', { pct: targetMarginPct })}
            </span>
          </div>
          <div className='flex flex-wrap items-center gap-1.5'>
            <NumericSpinnerInput
              value={props.policy.max_cost_ratio_pct}
              min={10}
              max={300}
              step={5}
              ariaLabel={t('Max Cost Ratio (%)')}
              onChange={(val) =>
                props.onChangePolicy((prev) => ({
                  ...prev,
                  max_cost_ratio_pct: val,
                }))
              }
            />
            <div className='flex items-center gap-1'>
              {COST_RATIO_PRESETS.map((preset) => (
                <button
                  key={preset}
                  type='button'
                  onClick={() =>
                    props.onChangePolicy((prev) => ({
                      ...prev,
                      max_cost_ratio_pct: preset,
                    }))
                  }
                  className={cn(
                    'rounded-md border px-1.5 py-1 font-mono text-[10px] transition-colors',
                    props.policy.max_cost_ratio_pct === preset
                      ? 'border-primary bg-primary/10 text-primary font-semibold'
                      : 'border-border/60 bg-background hover:bg-muted'
                  )}
                >
                  {preset}%
                </button>
              ))}
            </div>
          </div>
          <span className='text-muted-foreground text-[11px]'>
            {t('100% = break-even; 80% = lock >= 20% profit')}
          </span>
        </div>

        {/* Min Net Profit Floor */}
        <div className='flex flex-col gap-1.5'>
          <span className='text-xs font-medium'>
            {t('Min Net Profit Floor')} ({currencyMeta.symbol})
          </span>
          <div className='relative'>
            <span className='text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 -translate-y-1/2 font-mono text-xs'>
              {currencyMeta.symbol}
            </span>
            <Input
              type='number'
              step='any'
              aria-label={t('Min Net Profit Floor')}
              value={floorInputValue}
              onFocus={() => {
                setDraftFloorText(String(displayFloorVal))
                setEditingFloor(true)
              }}
              onChange={(e) => {
                const raw = e.target.value
                setDraftFloorText(raw)
                const parsed = Number(raw)
                if (raw !== '' && raw !== '-' && Number.isFinite(parsed)) {
                  const usdVal =
                    Math.round((parsed / currencyMeta.exchangeRate) * 1e6) / 1e6
                  props.onChangePolicy((prev) => ({
                    ...prev,
                    min_net_profit_usd: usdVal,
                  }))
                }
              }}
              onBlur={() => {
                setEditingFloor(false)
              }}
              className='h-8 ps-8 font-mono text-xs'
            />
          </div>
          <span className='text-muted-foreground text-[11px]'>
            {t('0 = no extra floor; negative allows subsidy buffer')}
          </span>
        </div>

        {/* Overflow Sensitivity */}
        <div className='flex flex-col gap-1.5'>
          <span className='text-xs font-medium'>
            {t('Overflow Shift Sensitivity')}
          </span>
          <div className='flex flex-wrap items-center gap-1.5'>
            {SENSITIVITY_PRESETS.map((preset) => {
              const active =
                Math.abs(props.policy.overflow_sensitivity - preset.value) <
                0.05
              return (
                <button
                  key={preset.value}
                  type='button'
                  onClick={() =>
                    props.onChangePolicy((prev) => ({
                      ...prev,
                      overflow_sensitivity: preset.value,
                    }))
                  }
                  className={cn(
                    'rounded-lg border px-2 py-1.5 text-[11px] font-medium transition-colors',
                    active
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-border/60 bg-background hover:bg-muted/60'
                  )}
                >
                  {t(preset.labelKey)}
                </button>
              )
            })}
          </div>
          <span className='text-muted-foreground text-[11px]'>
            {t('Controls how quickly over-cost traffic shifts to cheap channels')}
          </span>
        </div>
      </div>
    </div>
  )
}

export function CostControlUpstreamAllocationCard(props: {
  group: string
  policy: GroupCostControlPolicy
  upstreamSources: UpstreamSourceBrief[]
  channels: ChannelBrief[]
  enabledChannelIds: number[]
  snapshot?: GroupRealtimeProfitSnapshot
  selectedUpstreamFilter: string
  onSelectUpstreamFilter: (upstreamId: string) => void
  onChangePolicy: (
    updater: (prev: GroupCostControlPolicy) => GroupCostControlPolicy
  ) => void
}) {
  const { t, i18n } = useTranslation()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage ?? i18n.language)
  const enabledSet = useMemo(
    () => new Set(props.enabledChannelIds),
    [props.enabledChannelIds]
  )

  const upstreamRows = useMemo(() => {
    const countsByUpstream = new Map<
      string,
      { total: number; enabledInGroup: number }
    >()
    for (const ch of props.channels) {
      const resolved = resolveEffectiveChannelUpstream(
        ch,
        props.policy,
        props.upstreamSources
      )
      const prev = countsByUpstream.get(resolved.id) ?? {
        total: 0,
        enabledInGroup: 0,
      }
      prev.total += 1
      if (enabledSet.has(ch.id)) {
        prev.enabledInGroup += 1
      }
      countsByUpstream.set(resolved.id, prev)
    }

    const sources: UpstreamSourceBrief[] =
      props.upstreamSources.length > 0
        ? props.upstreamSources
        : [
            {
              id: DIRECT_UPSTREAM_ID,
              name: 'Direct',
              enabled: true,
              is_direct: true,
              currency: 'USD',
              exchange_rate: 1,
              balance_usd: 0,
              used_quota_usd: 0,
              channel_count: props.channels.length,
            },
          ]

    let totalActiveWeight = 0
    const draftItems = sources.map((src) => {
      const alloc = resolveUpstreamAlloc(src.id, props.policy)
      const counts = countsByUpstream.get(src.id) ?? {
        total: 0,
        enabledInGroup: 0,
      }
      const effectiveActiveWeight =
        alloc.enabled && counts.enabledInGroup > 0 ? alloc.weight : 0
      totalActiveWeight += effectiveActiveWeight
      return {
        src,
        alloc,
        counts,
        effectiveActiveWeight,
      }
    })

    const totalCalls = props.snapshot?.total_calls ?? 0

    return draftItems.map((item) => {
      const targetSharePct =
        totalActiveWeight > 0 && item.effectiveActiveWeight > 0
          ? (item.effectiveActiveWeight / totalActiveWeight) * 100
          : 0
      const profitStat = props.snapshot?.by_upstream?.[item.src.id]
      const calls = profitStat?.calls ?? 0
      const actualSharePct = totalCalls > 0 ? (calls / totalCalls) * 100 : 0
      const netProfitUSD = profitStat?.net_profit_usd ?? 0

      return {
        ...item,
        targetSharePct,
        calls,
        actualSharePct,
        netProfitUSD,
      }
    })
  }, [
    props.channels,
    props.policy,
    props.upstreamSources,
    props.snapshot,
    enabledSet,
  ])

  const updateUpstreamAlloc = (
    upstreamId: string,
    patch: Partial<CostControlUpstreamAlloc>
  ) => {
    props.onChangePolicy((prev) => {
      const cur = resolveUpstreamAlloc(upstreamId, prev)
      return {
        ...prev,
        upstream_allocations: {
          ...prev.upstream_allocations,
          [upstreamId]: {
            ...cur,
            ...patch,
          },
        },
      }
    })
  }

  const handleEqualizeWeights = () => {
    props.onChangePolicy((prev) => {
      const nextAllocs = { ...prev.upstream_allocations }
      for (const row of upstreamRows) {
        if (row.counts.enabledInGroup > 0) {
          nextAllocs[row.src.id] = { enabled: true, weight: 100 }
        }
      }
      return {
        ...prev,
        upstream_allocations: nextAllocs,
      }
    })
  }

  return (
    <div className='border-border/60 bg-card flex flex-col gap-3.5 rounded-xl border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-2'>
        <div className='space-y-0.5'>
          <div className='flex flex-wrap items-center gap-2'>
            <Network aria-hidden='true' className='text-primary size-4' />
            <h3 className='text-sm font-semibold'>
              {t('Upstream Account Allocation')}
            </h3>
            <Badge variant='outline' className='text-[11px]'>
              {upstreamRows.length} {t('Upstreams')}
            </Badge>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Discovered from CRON Upstream Accounts and direct channels. Within cost budget, traffic distributes across enabled upstreams by weight before selecting channels.'
            )}
          </p>
        </div>

        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={handleEqualizeWeights}
          className='w-full sm:w-auto'
        >
          {t('Equalize Active Upstreams')}
        </Button>
      </div>

      <div className='grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-3'>
        {upstreamRows.map((row) => {
          const displayName = row.src.is_direct
            ? t('Direct / Independent Channels')
            : row.src.name
          const isFiltered = props.selectedUpstreamFilter === row.src.id
          let profitClass = 'text-foreground'
          if (row.netProfitUSD > 1e-9) {
            profitClass = 'text-emerald-600 dark:text-emerald-400'
          } else if (row.netProfitUSD < -1e-9) {
            profitClass = 'text-destructive'
          }

          return (
            <div
              key={row.src.id}
              className={cn(
                'border-border/60 flex flex-col justify-between gap-3 rounded-xl border p-3.5 transition-all',
                row.alloc.enabled && row.counts.enabledInGroup > 0
                  ? 'bg-background'
                  : 'bg-muted/20 opacity-80',
                isFiltered && 'border-primary ring-primary/20 ring-2'
              )}
            >
              <div className='flex items-start justify-between gap-2'>
                <div className='min-w-0 space-y-1'>
                  <div className='flex flex-wrap items-center gap-1.5'>
                    <span className='truncate text-xs font-semibold'>
                      {displayName}
                    </span>
                    <Badge
                      variant={row.src.is_direct ? 'outline' : 'secondary'}
                      className='px-1.5 py-0 text-[10px]'
                    >
                      {row.src.is_direct ? t('Direct') : 'CRON'}
                    </Badge>
                    {!row.src.is_direct && row.src.balance_usd > 0 && (
                      <Badge
                        variant='outline'
                        className='px-1.5 py-0 font-mono text-[10px]'
                      >
                        {t('Bal')}: {formatBillingAmountUSD(row.src.balance_usd, intlLocale)}
                      </Badge>
                    )}
                  </div>
                  {row.src.base_url && (
                    <p className='text-muted-foreground truncate font-mono text-[11px]'>
                      {row.src.base_url}
                    </p>
                  )}
                </div>

                <div className='flex shrink-0 items-center gap-2'>
                  <button
                    type='button'
                    onClick={() =>
                      props.onSelectUpstreamFilter(
                        isFiltered ? 'all' : row.src.id
                      )
                    }
                    aria-label={`${t('Filter channels by upstream')} ${displayName}`}
                    className={cn(
                      'inline-flex items-center gap-1 rounded-md border px-2 py-1 text-[10px] font-medium transition-colors',
                      isFiltered
                        ? 'border-primary bg-primary text-primary-foreground'
                        : 'border-border/60 bg-background hover:bg-muted'
                    )}
                  >
                    <Filter aria-hidden='true' className='size-3' />
                    <span>
                      {row.counts.enabledInGroup}/{row.counts.total}
                    </span>
                  </button>
                  <Switch
                    checked={row.alloc.enabled}
                    onCheckedChange={(checked) =>
                      updateUpstreamAlloc(row.src.id, {
                        enabled: Boolean(checked),
                      })
                    }
                    aria-label={`${t('Enable upstream allocation')} ${displayName}`}
                  />
                </div>
              </div>

              <div className='bg-muted/30 grid grid-cols-3 gap-2 rounded-lg p-2 text-[11px]'>
                <div className='flex flex-col'>
                  <span className='text-muted-foreground text-[10px]'>
                    {t('Target Ratio')}
                  </span>
                  <span className='text-primary font-mono font-semibold'>
                    {row.targetSharePct.toFixed(1)}%
                  </span>
                </div>
                <div className='flex flex-col'>
                  <span className='text-muted-foreground text-[10px]'>
                    {t('Actual Calls')}
                  </span>
                  <span className='font-mono font-medium'>
                    {formatNumber(row.calls, intlLocale)} (
                    {row.actualSharePct.toFixed(0)}%)
                  </span>
                </div>
                <div className='flex flex-col'>
                  <span className='text-muted-foreground text-[10px]'>
                    {t('Net Profit')}
                  </span>
                  <span className={cn('font-mono font-semibold', profitClass)}>
                    {row.netProfitUSD > 0 ? '+' : ''}
                    {formatBillingAmountUSD(row.netProfitUSD, intlLocale)}
                  </span>
                </div>
              </div>

              <div className='space-y-2'>
                <div className='bg-muted h-1.5 w-full overflow-hidden rounded-full'>
                  <div
                    className='bg-primary h-full rounded-full transition-all'
                    style={{ width: `${Math.min(100, row.targetSharePct)}%` }}
                  />
                </div>

                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <span className='text-muted-foreground text-[11px]'>
                    {t('Allocation Weight')}
                  </span>
                  <div className='flex flex-wrap items-center gap-1'>
                    <NumericSpinnerInput
                      value={row.alloc.weight}
                      min={0}
                      max={1000}
                      step={10}
                      disabled={!row.alloc.enabled}
                      ariaLabel={`${displayName} ${t('Allocation Weight')}`}
                      onChange={(val) =>
                        updateUpstreamAlloc(row.src.id, { weight: val })
                      }
                    />
                    {UPSTREAM_WEIGHT_PRESETS.map((preset) => (
                      <button
                        key={preset}
                        type='button'
                        disabled={!row.alloc.enabled}
                        onClick={() =>
                          updateUpstreamAlloc(row.src.id, { weight: preset })
                        }
                        className={cn(
                          'rounded border px-1.5 py-0.5 font-mono text-[10px] transition-colors disabled:opacity-40',
                          row.alloc.weight === preset
                            ? 'border-primary bg-primary/10 text-primary font-semibold'
                            : 'border-border/60 bg-background hover:bg-muted'
                        )}
                      >
                        {preset}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
