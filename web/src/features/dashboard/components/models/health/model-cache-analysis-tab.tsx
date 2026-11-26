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
  ChevronDown,
  ChevronRight,
  Cpu,
  DatabaseZap,
  HardDriveDownload,
  Info,
  Layers,
  Percent,
  Server,
} from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type {
  ChannelHealthSummaryRow,
  ChannelHealthTrendSeries,
  ModelChannelHealthDetail,
  ModelHealthAnalysisRow,
  ModelHealthSummary,
  ModelHealthTrendPoint,
} from '@/features/channels/api'
import { ChannelTypeLogo } from '@/features/channels/components/channel-type-badge'
import { formatCompactNumber, formatNumber } from '@/lib/format'
import type { TimeGranularity, TimeRangeKey } from '@/lib/time'
import { cn } from '@/lib/utils'

import { ModelHealthCharts } from './health-charts'
import {
  CacheRateDistributionBar,
  HealthToolbarControls,
} from './health-shared'
import {
  formatPercentValue,
  getAvailabilityBadgeClass,
  getCacheRateBadgeClass,
  type CachePercentileScope,
  type HealthEntityViewMode,
} from './health-utils'

type CacheStatsCarrier = {
  total_input_tokens: number
  cache_hit_call_count: number
  cache_rate_p5_low: number
  cache_rate_median: number
  cache_rate_p95_high: number
  cache_rate_avg: number
  hit_cache_rate_p5_low: number
  hit_cache_rate_median: number
  hit_cache_rate_p95_high: number
  hit_cache_rate_avg: number
}

function resolveCachePercentiles(
  item: CacheStatsCarrier | undefined,
  scope: CachePercentileScope
) {
  if (!item) {
    return { p5: 0, p50: 0, p95: 0, avg: 0, hasSamples: false }
  }
  if (scope === 'hit') {
    return {
      p5: item.hit_cache_rate_p5_low,
      p50: item.hit_cache_rate_median,
      p95: item.hit_cache_rate_p95_high,
      avg: item.hit_cache_rate_avg,
      hasSamples: item.cache_hit_call_count > 0,
    }
  }
  return {
    p5: item.cache_rate_p5_low,
    p50: item.cache_rate_median,
    p95: item.cache_rate_p95_high,
    avg: item.cache_rate_avg,
    hasSamples: item.total_input_tokens > 0,
  }
}

export type ModelCacheAnalysisTabProps = {
  summary: ModelHealthSummary | undefined
  models: ModelHealthAnalysisRow[]
  channels: ChannelHealthSummaryRow[]
  trend: ModelHealthTrendPoint[]
  channelTrends: ChannelHealthTrendSeries[]
  granularity: TimeGranularity
  viewMode: HealthEntityViewMode
  onViewModeChange: (next: HealthEntityViewMode) => void
  keyword: string
  onKeywordChange: (next: string) => void
  timeRange: TimeRangeKey
  onTimeRangeChange: (next: TimeRangeKey) => void
  onlyActiveCalls: boolean
  onToggleOnlyActiveCalls: () => void
  isLoading: boolean
  isFetching: boolean
  onRefresh: () => void
  isMobile: boolean
  intlLocale?: string
}

export function ModelCacheAnalysisTab(props: ModelCacheAnalysisTabProps) {
  const { t } = useTranslation()
  const [cacheScope, setCacheScope] = useState<CachePercentileScope>('all')
  const [expandedModels, setExpandedModels] = useState<Record<string, boolean>>(
    {}
  )
  const [expandedChannels, setExpandedChannels] = useState<
    Record<number, boolean>
  >({})

  const summaryDist = resolveCachePercentiles(props.summary, cacheScope)

  const toggleModel = (modelName: string) => {
    setExpandedModels((prev) => ({
      ...prev,
      [modelName]: !prev[modelName],
    }))
  }

  const toggleChannel = (channelId: number) => {
    setExpandedChannels((prev) => ({
      ...prev,
      [channelId]: !prev[channelId],
    }))
  }

  const scopeSwitch = (
    <Tabs
      value={cacheScope}
      onValueChange={(val) => setCacheScope(val as CachePercentileScope)}
    >
      <TabsList className='h-8'>
        <TabsTrigger value='all' className='px-2 text-xs'>
          {t('All Input Calls')}
        </TabsTrigger>
        <TabsTrigger value='hit' className='px-2 text-xs'>
          {t('Cache-Hit Calls Only')}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  )

  return (
    <div className='space-y-4'>
      {/* Cache KPI Summary Cards */}
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span className='flex items-center gap-1'>
                {t('Weighted Avg Cache Rate')}
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Info className='text-muted-foreground size-3.5 cursor-help' />
                    }
                  />
                  <TooltipContent className='max-w-xs text-xs'>
                    {t(
                      'Total Cache-Read Tokens ÷ Normalized Total Input Tokens across Anthropic and OpenAI/Gemini formats.'
                    )}
                  </TooltipContent>
                </Tooltip>
              </span>
              <DatabaseZap aria-hidden='true' className='size-4 text-sky-500' />
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 text-xl font-bold'>
              <span className='text-sky-600 dark:text-sky-400'>
                {formatPercentValue(props.summary?.weighted_cache_rate ?? 0, 2)}
              </span>
              <span className='text-muted-foreground font-mono text-xs font-normal'>
                {t('Input')}:{' '}
                {formatCompactNumber(
                  props.summary?.total_input_tokens ?? 0,
                  props.intlLocale
                )}
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span className='flex items-center gap-1'>
                {t('Cache Rate (P5 / P50 / P95)')}
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Info className='text-muted-foreground size-3.5 cursor-help' />
                    }
                  />
                  <TooltipContent className='max-w-xs text-xs'>
                    {t(
                      'Per-request prompt cache hit percentage at 5% low (P5), median (P50), and 95% high (P95).'
                    )}
                  </TooltipContent>
                </Tooltip>
              </span>
              <Percent aria-hidden='true' className='text-primary size-4' />
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-1.5 font-mono text-lg font-bold'>
              <span className='text-amber-600 dark:text-amber-400'>
                {formatPercentValue(summaryDist.p5, 1)}
              </span>
              <span className='text-muted-foreground text-xs'>/</span>
              <span className='text-xl'>
                {formatPercentValue(summaryDist.p50, 1)}
              </span>
              <span className='text-muted-foreground text-xs'>/</span>
              <span className='text-emerald-600 dark:text-emerald-400'>
                {formatPercentValue(summaryDist.p95, 1)}
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Cache Trigger Call Ratio')}</span>
              <Layers aria-hidden='true' className='size-4 text-emerald-500' />
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 text-xl font-bold'>
              <span>
                {formatPercentValue(
                  props.summary?.cache_hit_call_ratio ?? 0,
                  1
                )}
              </span>
              <span className='text-muted-foreground font-mono text-xs font-normal'>
                {formatNumber(
                  props.summary?.cache_hit_call_count ?? 0,
                  props.intlLocale
                )}{' '}
                /{' '}
                {formatNumber(
                  props.summary?.success_count ?? 0,
                  props.intlLocale
                )}{' '}
                {t('calls')}
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Cache Read vs Creation Tokens')}</span>
              <HardDriveDownload
                aria-hidden='true'
                className='text-muted-foreground size-4'
              />
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 font-mono text-xl font-bold'>
              <span className='text-emerald-600 dark:text-emerald-400'>
                {formatCompactNumber(
                  props.summary?.cache_read_tokens ?? 0,
                  props.intlLocale
                )}
              </span>
              <span className='text-muted-foreground text-xs font-normal'>
                {t('Write')}:{' '}
                {formatCompactNumber(
                  props.summary?.cache_write_tokens ?? 0,
                  props.intlLocale
                )}
              </span>
            </CardTitle>
          </CardHeader>
        </Card>
      </div>

      {/* Cache Charts */}
      <ModelHealthCharts
        mode='cache'
        entityViewMode={props.viewMode}
        cacheScope={cacheScope}
        trend={props.trend}
        channelTrends={props.channelTrends}
        granularity={props.granularity}
        models={props.models}
        channels={props.channels}
        isLoading={props.isLoading}
      />

      {/* Main Cache Matrix (Inheriting ModelHealthPanel dual-mode layout) */}
      <Card>
        <CardHeader className='pb-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div className='space-y-1'>
              <div className='flex flex-wrap items-center gap-2.5'>
                <CardTitle className='text-base'>
                  {t('Model & Channel Prompt Cache Matrix')}
                </CardTitle>
                <Tabs
                  value={props.viewMode}
                  onValueChange={(val) =>
                    props.onViewModeChange(val as HealthEntityViewMode)
                  }
                >
                  <TabsList className='h-8'>
                    <TabsTrigger value='models' className='px-2.5 text-xs'>
                      <Cpu aria-hidden='true' className='me-1.5 size-3.5' />
                      {t('By Model Average')} ({props.models.length})
                    </TabsTrigger>
                    <TabsTrigger value='channels' className='px-2.5 text-xs'>
                      <Server aria-hidden='true' className='me-1.5 size-3.5' />
                      {t('By Channel Average')} ({props.channels.length})
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>
              <CardDescription className='text-xs'>
                {props.viewMode === 'models'
                  ? t(
                      'Click any model row to compare prompt cache hit rate, P5 / P50 / P95 distribution, and cache read/write volumes across upstream channels'
                    )
                  : t(
                      'Click any channel row to inspect per-model prompt cache effectiveness and detect channels dropping upstream cache'
                    )}
              </CardDescription>
            </div>

            <HealthToolbarControls
              keyword={props.keyword}
              onKeywordChange={props.onKeywordChange}
              timeRange={props.timeRange}
              onTimeRangeChange={props.onTimeRangeChange}
              onlyActiveCalls={props.onlyActiveCalls}
              onToggleOnlyActiveCalls={props.onToggleOnlyActiveCalls}
              isFetching={props.isFetching}
              onRefresh={props.onRefresh}
              extraSlot={scopeSwitch}
            />
          </div>
        </CardHeader>

        <CardContent className='p-0'>
          {props.isLoading && (
            <div className='space-y-2 p-4'>
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
            </div>
          )}

          {!props.isLoading && props.viewMode === 'models' && (
            <ModelsCacheTable
              rows={props.models}
              cacheScope={cacheScope}
              expanded={expandedModels}
              onToggle={toggleModel}
              isMobile={props.isMobile}
              intlLocale={props.intlLocale}
            />
          )}

          {!props.isLoading && props.viewMode === 'channels' && (
            <ChannelsCacheTable
              rows={props.channels}
              cacheScope={cacheScope}
              expanded={expandedChannels}
              onToggle={toggleChannel}
              isMobile={props.isMobile}
              intlLocale={props.intlLocale}
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function ModelsCacheTable(props: {
  rows: ModelHealthAnalysisRow[]
  cacheScope: CachePercentileScope
  expanded: Record<string, boolean>
  onToggle: (modelName: string) => void
  isMobile: boolean
  intlLocale?: string
}) {
  const { t } = useTranslation()

  if (props.isMobile) {
    return (
      <div className='flex flex-col gap-3 p-3'>
        {props.rows.length === 0 ? (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            {t('No matching models found')}
          </div>
        ) : (
          props.rows.map((row) => {
            const isExpanded = Boolean(props.expanded[row.model])
            const dist = resolveCachePercentiles(row, props.cacheScope)
            return (
              <div
                key={row.model}
                className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
              >
                <button
                  type='button'
                  aria-expanded={isExpanded}
                  onClick={() => props.onToggle(row.model)}
                  className='flex w-full items-start justify-between gap-2 text-start'
                >
                  <div className='flex min-w-0 items-center gap-1.5'>
                    {isExpanded ? (
                      <ChevronDown
                        aria-hidden='true'
                        className='text-muted-foreground size-4 shrink-0'
                      />
                    ) : (
                      <ChevronRight
                        aria-hidden='true'
                        className='text-muted-foreground size-4 shrink-0'
                      />
                    )}
                    <span className='truncate font-mono text-xs font-semibold'>
                      {row.model}
                    </span>
                  </div>
                  <Badge
                    variant='outline'
                    className={cn(
                      'font-mono text-[10px]',
                      getCacheRateBadgeClass(row.weighted_cache_rate)
                    )}
                  >
                    {formatPercentValue(row.weighted_cache_rate, 1)}
                  </Badge>
                </button>

                <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 font-mono text-[11px]'>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Cache Rate (P5 / P50 / P95)')}
                    </span>
                    <span>
                      {formatPercentValue(dist.p5, 0)} /{' '}
                      {formatPercentValue(dist.p50, 0)} /{' '}
                      {formatPercentValue(dist.p95, 0)}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Cache Trigger Call Ratio')}
                    </span>
                    <span>
                      {formatPercentValue(row.cache_hit_call_ratio, 1)} (
                      {row.cache_hit_call_count}/{row.success_count})
                    </span>
                  </div>
                </div>

                {isExpanded && (
                  <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                    {row.channels.map((ch) => (
                      <MobileCacheDetailCard
                        key={ch.channel_id}
                        detail={ch}
                        cacheScope={props.cacheScope}
                        showChannel
                        intlLocale={props.intlLocale}
                      />
                    ))}
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>
    )
  }

  return (
    <div className='overflow-x-auto'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-8' />
            <TableHead>{t('Model')}</TableHead>
            <TableHead className='text-right'>{t('Availability')}</TableHead>
            <TableHead className='text-right'>
              {t('Calls & Hit Ratio')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Input / Cache Read / Write')}
            </TableHead>
            <TableHead>
              {t('Cache Rate (5% Low · 50% Median · 95% High)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Per-Call Cache Avg')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Weighted Avg Cache Rate')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={8}
                className='text-muted-foreground h-24 text-center text-sm'
              >
                {t('No matching models found')}
              </TableCell>
            </TableRow>
          ) : (
            props.rows.map((row) => {
              const isExpanded = Boolean(props.expanded[row.model])
              const dist = resolveCachePercentiles(row, props.cacheScope)
              return (
                <Fragment key={row.model}>
                  <TableRow
                    tabIndex={0}
                    aria-expanded={isExpanded}
                    className='hover:bg-muted/50 cursor-pointer'
                    onClick={() => props.onToggle(row.model)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        props.onToggle(row.model)
                      }
                    }}
                  >
                    <TableCell>
                      {isExpanded ? (
                        <ChevronDown
                          aria-hidden='true'
                          className='text-muted-foreground size-4'
                        />
                      ) : (
                        <ChevronRight
                          aria-hidden='true'
                          className='text-muted-foreground size-4'
                        />
                      )}
                    </TableCell>
                    <TableCell>
                      <div className='flex items-center gap-2'>
                        <span className='font-mono text-xs font-semibold'>
                          {row.model}
                        </span>
                        <Badge
                          variant='outline'
                          className='px-1.5 py-0 font-mono text-[10px]'
                        >
                          {row.channel_count} {t('Channels')}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className='text-right'>
                      <Badge
                        variant='outline'
                        className={cn(
                          'font-mono text-xs',
                          getAvailabilityBadgeClass(row.availability)
                        )}
                      >
                        {row.availability.toFixed(2)}%
                      </Badge>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-medium'>
                          {formatNumber(row.request_count, props.intlLocale)}{' '}
                          {t('calls')}
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          {t('Hit')}:{' '}
                          {formatPercentValue(row.cache_hit_call_ratio, 1)} (
                          {row.cache_hit_call_count})
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span>
                          {formatCompactNumber(
                            row.total_input_tokens,
                            props.intlLocale
                          )}
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          R:{' '}
                          {formatCompactNumber(
                            row.cache_read_tokens,
                            props.intlLocale
                          )}{' '}
                          / W:{' '}
                          {formatCompactNumber(
                            row.cache_write_tokens,
                            props.intlLocale
                          )}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <CacheRateDistributionBar
                        p5Low={dist.p5}
                        median={dist.p50}
                        p95High={dist.p95}
                        hasSamples={dist.hasSamples}
                      />
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs font-semibold'>
                      {dist.hasSamples ? formatPercentValue(dist.avg, 1) : '-'}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Badge
                        variant='outline'
                        className={cn(
                          'font-mono text-xs',
                          getCacheRateBadgeClass(row.weighted_cache_rate)
                        )}
                      >
                        {formatPercentValue(row.weighted_cache_rate, 2)}
                      </Badge>
                    </TableCell>
                  </TableRow>

                  {isExpanded && (
                    <TableRow className='bg-muted/20'>
                      <TableCell colSpan={8} className='p-4'>
                        <div className='bg-background rounded-lg border p-3'>
                          <div className='mb-2.5 flex items-center gap-1.5 text-xs font-semibold'>
                            <Server
                              aria-hidden='true'
                              className='text-primary size-3.5'
                            />
                            <span>
                              {t('Same-Model Channel Cache Comparison')} (
                              {row.channels.length})
                            </span>
                          </div>
                          <NestedCacheTable
                            items={row.channels}
                            cacheScope={props.cacheScope}
                            showChannelCol
                            intlLocale={props.intlLocale}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </Fragment>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}

function ChannelsCacheTable(props: {
  rows: ChannelHealthSummaryRow[]
  cacheScope: CachePercentileScope
  expanded: Record<number, boolean>
  onToggle: (channelId: number) => void
  isMobile: boolean
  intlLocale?: string
}) {
  const { t } = useTranslation()

  if (props.isMobile) {
    return (
      <div className='flex flex-col gap-3 p-3'>
        {props.rows.length === 0 ? (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            {t('No matching channels found')}
          </div>
        ) : (
          props.rows.map((row) => {
            const isExpanded = Boolean(props.expanded[row.channel_id])
            const dist = resolveCachePercentiles(row, props.cacheScope)
            return (
              <div
                key={row.channel_id}
                className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
              >
                <button
                  type='button'
                  aria-expanded={isExpanded}
                  onClick={() => props.onToggle(row.channel_id)}
                  className='flex w-full items-start justify-between gap-2 text-start'
                >
                  <div className='flex min-w-0 items-center gap-1.5'>
                    {isExpanded ? (
                      <ChevronDown
                        aria-hidden='true'
                        className='text-muted-foreground size-4 shrink-0'
                      />
                    ) : (
                      <ChevronRight
                        aria-hidden='true'
                        className='text-muted-foreground size-4 shrink-0'
                      />
                    )}
                    <ChannelTypeLogo type={row.channel_type} size={14} />
                    <span className='truncate text-xs font-semibold'>
                      #{row.channel_id} {row.channel_name}
                    </span>
                  </div>
                  <Badge
                    variant='outline'
                    className={cn(
                      'font-mono text-[10px]',
                      getCacheRateBadgeClass(row.weighted_cache_rate)
                    )}
                  >
                    {formatPercentValue(row.weighted_cache_rate, 1)}
                  </Badge>
                </button>

                <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 font-mono text-[11px]'>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Cache Rate (P5 / P50 / P95)')}
                    </span>
                    <span>
                      {formatPercentValue(dist.p5, 0)} /{' '}
                      {formatPercentValue(dist.p50, 0)} /{' '}
                      {formatPercentValue(dist.p95, 0)}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Cache Trigger Call Ratio')}
                    </span>
                    <span>
                      {formatPercentValue(row.cache_hit_call_ratio, 1)} (
                      {row.cache_hit_call_count}/{row.success_count})
                    </span>
                  </div>
                </div>

                {isExpanded && (
                  <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                    {row.models.map((m) => (
                      <MobileCacheDetailCard
                        key={`${row.channel_id}-${m.model}`}
                        detail={m}
                        cacheScope={props.cacheScope}
                        showChannel={false}
                        intlLocale={props.intlLocale}
                      />
                    ))}
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>
    )
  }

  return (
    <div className='overflow-x-auto'>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className='w-8' />
            <TableHead>{t('Channel')}</TableHead>
            <TableHead className='text-right'>{t('Availability')}</TableHead>
            <TableHead className='text-right'>
              {t('Calls & Hit Ratio')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Input / Cache Read / Write')}
            </TableHead>
            <TableHead>
              {t('Cache Rate (5% Low · 50% Median · 95% High)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Per-Call Cache Avg')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Weighted Avg Cache Rate')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={8}
                className='text-muted-foreground h-24 text-center text-sm'
              >
                {t('No matching channels found')}
              </TableCell>
            </TableRow>
          ) : (
            props.rows.map((row) => {
              const isExpanded = Boolean(props.expanded[row.channel_id])
              const dist = resolveCachePercentiles(row, props.cacheScope)
              return (
                <Fragment key={row.channel_id}>
                  <TableRow
                    tabIndex={0}
                    aria-expanded={isExpanded}
                    className='hover:bg-muted/50 cursor-pointer'
                    onClick={() => props.onToggle(row.channel_id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        props.onToggle(row.channel_id)
                      }
                    }}
                  >
                    <TableCell>
                      {isExpanded ? (
                        <ChevronDown
                          aria-hidden='true'
                          className='text-muted-foreground size-4'
                        />
                      ) : (
                        <ChevronRight
                          aria-hidden='true'
                          className='text-muted-foreground size-4'
                        />
                      )}
                    </TableCell>
                    <TableCell>
                      <div className='flex items-center gap-2'>
                        <span className='text-muted-foreground font-mono text-xs'>
                          #{row.channel_id}
                        </span>
                        <ChannelTypeLogo type={row.channel_type} size={15} />
                        <span className='text-xs font-semibold'>
                          {row.channel_name}
                        </span>
                        <Badge
                          variant='outline'
                          className='px-1.5 py-0 font-mono text-[10px]'
                        >
                          {row.model_count} {t('Models')}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className='text-right'>
                      <Badge
                        variant='outline'
                        className={cn(
                          'font-mono text-xs',
                          getAvailabilityBadgeClass(row.availability)
                        )}
                      >
                        {row.availability.toFixed(2)}%
                      </Badge>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-medium'>
                          {formatNumber(row.request_count, props.intlLocale)}{' '}
                          {t('calls')}
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          {t('Hit')}:{' '}
                          {formatPercentValue(row.cache_hit_call_ratio, 1)} (
                          {row.cache_hit_call_count})
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span>
                          {formatCompactNumber(
                            row.total_input_tokens,
                            props.intlLocale
                          )}
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          R:{' '}
                          {formatCompactNumber(
                            row.cache_read_tokens,
                            props.intlLocale
                          )}{' '}
                          / W:{' '}
                          {formatCompactNumber(
                            row.cache_write_tokens,
                            props.intlLocale
                          )}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <CacheRateDistributionBar
                        p5Low={dist.p5}
                        median={dist.p50}
                        p95High={dist.p95}
                        hasSamples={dist.hasSamples}
                      />
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs font-semibold'>
                      {dist.hasSamples ? formatPercentValue(dist.avg, 1) : '-'}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Badge
                        variant='outline'
                        className={cn(
                          'font-mono text-xs',
                          getCacheRateBadgeClass(row.weighted_cache_rate)
                        )}
                      >
                        {formatPercentValue(row.weighted_cache_rate, 2)}
                      </Badge>
                    </TableCell>
                  </TableRow>

                  {isExpanded && (
                    <TableRow className='bg-muted/20'>
                      <TableCell colSpan={8} className='p-4'>
                        <div className='bg-background rounded-lg border p-3'>
                          <div className='mb-2.5 flex items-center gap-1.5 text-xs font-semibold'>
                            <Cpu
                              aria-hidden='true'
                              className='text-primary size-3.5'
                            />
                            <span>
                              {t('Channel Model Cache Breakdown')} (
                              {row.models.length})
                            </span>
                          </div>
                          <NestedCacheTable
                            items={row.models}
                            cacheScope={props.cacheScope}
                            showChannelCol={false}
                            intlLocale={props.intlLocale}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </Fragment>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}

function NestedCacheTable(props: {
  items: ModelChannelHealthDetail[]
  cacheScope: CachePercentileScope
  showChannelCol: boolean
  intlLocale?: string
}) {
  const { t } = useTranslation()

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className='h-8 text-[11px]'>
            {props.showChannelCol ? t('Channel') : t('Model')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Availability')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Calls & Hit Ratio')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Input / Cache Read / Write')}
          </TableHead>
          <TableHead className='h-8 text-[11px]'>
            {t('Cache Rate (5% Low · 50% Median · 95% High)')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Per-Call Cache Avg')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Weighted Avg Cache Rate')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.items.map((item) => {
          const dist = resolveCachePercentiles(item, props.cacheScope)
          return (
            <TableRow key={`${item.channel_id}-${item.model}`}>
              <TableCell className='py-1.5 text-xs'>
                {props.showChannelCol ? (
                  <div className='flex items-center gap-1.5'>
                    <span className='text-muted-foreground font-mono'>
                      #{item.channel_id}
                    </span>
                    <ChannelTypeLogo type={item.channel_type} size={14} />
                    <span className='font-medium'>{item.channel_name}</span>
                  </div>
                ) : (
                  <span className='font-mono font-semibold'>{item.model}</span>
                )}
              </TableCell>
              <TableCell className='py-1.5 text-right'>
                <Badge
                  variant='outline'
                  className={cn(
                    'font-mono text-[10px]',
                    getAvailabilityBadgeClass(item.availability)
                  )}
                >
                  {item.availability.toFixed(2)}%
                </Badge>
              </TableCell>
              <TableCell className='py-1.5 text-right font-mono text-xs'>
                <span>
                  {formatNumber(item.request_count, props.intlLocale)}
                </span>
                <span className='text-muted-foreground ms-1 text-[10px]'>
                  ({formatPercentValue(item.cache_hit_call_ratio, 0)})
                </span>
              </TableCell>
              <TableCell className='py-1.5 text-right font-mono text-xs'>
                {formatCompactNumber(item.total_input_tokens, props.intlLocale)}{' '}
                /{' '}
                {formatCompactNumber(item.cache_read_tokens, props.intlLocale)}{' '}
                /{' '}
                {formatCompactNumber(item.cache_write_tokens, props.intlLocale)}
              </TableCell>
              <TableCell className='py-1.5'>
                <CacheRateDistributionBar
                  p5Low={dist.p5}
                  median={dist.p50}
                  p95High={dist.p95}
                  hasSamples={dist.hasSamples}
                />
              </TableCell>
              <TableCell className='py-1.5 text-right font-mono text-xs font-semibold'>
                {dist.hasSamples ? formatPercentValue(dist.avg, 1) : '-'}
              </TableCell>
              <TableCell className='py-1.5 text-right'>
                <Badge
                  variant='outline'
                  className={cn(
                    'font-mono text-[10px]',
                    getCacheRateBadgeClass(item.weighted_cache_rate)
                  )}
                >
                  {formatPercentValue(item.weighted_cache_rate, 2)}
                </Badge>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function MobileCacheDetailCard(props: {
  detail: ModelChannelHealthDetail
  cacheScope: CachePercentileScope
  showChannel: boolean
  intlLocale?: string
}) {
  const { t } = useTranslation()
  const d = props.detail
  const dist = resolveCachePercentiles(d, props.cacheScope)

  return (
    <div className='bg-muted/30 flex flex-col gap-1.5 rounded p-2 text-xs'>
      <div className='flex items-center justify-between gap-2'>
        {props.showChannel ? (
          <div className='flex min-w-0 items-center gap-1.5'>
            <ChannelTypeLogo type={d.channel_type} size={13} />
            <span className='truncate font-medium'>
              #{d.channel_id} {d.channel_name}
            </span>
          </div>
        ) : (
          <span className='truncate font-mono font-semibold'>{d.model}</span>
        )}
        <Badge
          variant='outline'
          className={cn(
            'font-mono text-[10px]',
            getCacheRateBadgeClass(d.weighted_cache_rate)
          )}
        >
          {formatPercentValue(d.weighted_cache_rate, 1)}
        </Badge>
      </div>
      <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 font-mono text-[11px]'>
        <span>
          P5/P50/P95: {formatPercentValue(dist.p5, 0)} /{' '}
          <strong className='text-foreground'>
            {formatPercentValue(dist.p50, 0)}
          </strong>{' '}
          / {formatPercentValue(dist.p95, 0)}
        </span>
        <span>
          {t('Hit')}: {formatPercentValue(d.cache_hit_call_ratio, 0)} (
          {formatCompactNumber(d.cache_read_tokens, props.intlLocale)})
        </span>
      </div>
    </div>
  )
}
