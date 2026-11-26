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
  ArrowUpRight,
  ChevronDown,
  ChevronRight,
  Cpu,
  DatabaseZap,
  Gauge,
  HeartPulse,
  Server,
  Timer,
} from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { HealthToolbarControls } from './health-shared'
import {
  formatPercentValue,
  formatTpsValue,
  formatTtftSec,
  getAvailabilityBadgeClass,
  getCacheRateBadgeClass,
  type HealthEntityViewMode,
  type HealthSubTab,
} from './health-utils'

export type ComprehensiveHealthTabProps = {
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
  onSelectSubTab: (tab: HealthSubTab) => void
  isMobile: boolean
  intlLocale?: string
}

export function ComprehensiveHealthTab(props: ComprehensiveHealthTabProps) {
  const { t } = useTranslation()
  const [expandedModels, setExpandedModels] = useState<Record<string, boolean>>(
    {}
  )
  const [expandedChannels, setExpandedChannels] = useState<
    Record<number, boolean>
  >({})

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

  return (
    <div className='space-y-4'>
      {/* Comprehensive 4-Pillar KPI Cards */}
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Overall Availability & Requests')}</span>
              <HeartPulse
                aria-hidden='true'
                className='size-4 text-emerald-500'
              />
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 text-xl font-bold'>
              <span className='text-emerald-600 dark:text-emerald-400'>
                {(props.summary?.availability ?? 100).toFixed(2)}%
              </span>
              <span className='text-muted-foreground font-mono text-xs font-normal'>
                {formatNumber(
                  props.summary?.request_count ?? 0,
                  props.intlLocale
                )}{' '}
                {t('calls')} ({props.summary?.error_count ?? 0} {t('Errors')})
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('First Token Time (TTFT)')}</span>
              <button
                type='button'
                onClick={() => props.onSelectSubTab('speed')}
                className='text-muted-foreground hover:text-foreground inline-flex items-center gap-0.5 text-[11px]'
              >
                <span>{t('Response Speed')}</span>
                <ArrowUpRight aria-hidden='true' className='size-3' />
              </button>
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 text-xl font-bold'>
              <span>{formatTtftSec(props.summary?.median_ttft_sec ?? 0)}</span>
              <span className='text-muted-foreground font-mono text-xs font-normal'>
                {t('Avg')}: {formatTtftSec(props.summary?.avg_ttft_sec ?? 0)} ·
                P95: {formatTtftSec(props.summary?.p95_ttft_sec ?? 0)}
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Generation Speed (Decode TPS)')}</span>
              <Gauge aria-hidden='true' className='text-primary size-4' />
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 font-mono text-xl font-bold'>
              <span>
                {formatTpsValue(props.summary?.decode_tps_trimmed_90 ?? 0)}{' '}
                <span className='text-muted-foreground text-xs font-normal'>
                  tok/s
                </span>
              </span>
              <span className='text-muted-foreground text-xs font-normal'>
                ({formatTpsValue(props.summary?.decode_tps_p5_low ?? 0)} ~{' '}
                {formatTpsValue(props.summary?.decode_tps_p95_high ?? 0)})
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Overall Cache Hit Rate')}</span>
              <button
                type='button'
                onClick={() => props.onSelectSubTab('cache')}
                className='text-muted-foreground hover:text-foreground inline-flex items-center gap-0.5 text-[11px]'
              >
                <span>{t('Model Cache Analysis')}</span>
                <ArrowUpRight aria-hidden='true' className='size-3' />
              </button>
            </CardDescription>
            <CardTitle className='flex flex-wrap items-baseline gap-2 text-xl font-bold'>
              <span className='text-sky-600 dark:text-sky-400'>
                {formatPercentValue(props.summary?.weighted_cache_rate ?? 0, 1)}
              </span>
              <span className='text-muted-foreground font-mono text-xs font-normal'>
                P50:{' '}
                {formatPercentValue(props.summary?.cache_rate_median ?? 0, 1)} ·{' '}
                {formatCompactNumber(
                  props.summary?.cache_read_tokens ?? 0,
                  props.intlLocale
                )}{' '}
                {t('Cached')}
              </span>
            </CardTitle>
          </CardHeader>
        </Card>
      </div>

      {/* Interactive Trend Charts */}
      <ModelHealthCharts
        mode='overview'
        entityViewMode={props.viewMode}
        trend={props.trend}
        channelTrends={props.channelTrends}
        granularity={props.granularity}
        models={props.models}
        channels={props.channels}
        isLoading={props.isLoading}
      />

      {/* Unified Summary Matrix */}
      <Card>
        <CardHeader className='pb-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div className='space-y-1'>
              <div className='flex flex-wrap items-center gap-2.5'>
                <CardTitle className='text-base'>
                  {t('Comprehensive Model & Channel Health Summary')}
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
                {t(
                  'At-a-glance overview combining availability, first-token latency (TTFT), decode TPS, and prompt cache hit rate'
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
            <OverviewModelsTable
              rows={props.models}
              expanded={expandedModels}
              onToggle={toggleModel}
              onSelectSubTab={props.onSelectSubTab}
              isMobile={props.isMobile}
              intlLocale={props.intlLocale}
            />
          )}

          {!props.isLoading && props.viewMode === 'channels' && (
            <OverviewChannelsTable
              rows={props.channels}
              expanded={expandedChannels}
              onToggle={toggleChannel}
              onSelectSubTab={props.onSelectSubTab}
              isMobile={props.isMobile}
              intlLocale={props.intlLocale}
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function OverviewModelsTable(props: {
  rows: ModelHealthAnalysisRow[]
  expanded: Record<string, boolean>
  onToggle: (modelName: string) => void
  onSelectSubTab: (tab: HealthSubTab) => void
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
                      getAvailabilityBadgeClass(row.availability)
                    )}
                  >
                    {row.availability.toFixed(1)}%
                  </Badge>
                </button>

                <div className='bg-muted/20 grid grid-cols-3 gap-2 rounded-md p-2 font-mono text-[11px]'>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      TTFT (P50)
                    </span>
                    <span>{formatTtftSec(row.median_ttft_sec)}</span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      TPS (90%)
                    </span>
                    <span>{formatTpsValue(row.decode_tps_trimmed_90)} t/s</span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Cache Rate')}
                    </span>
                    <span>
                      {formatPercentValue(row.weighted_cache_rate, 1)}
                    </span>
                  </div>
                </div>

                {isExpanded && (
                  <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                    {row.channels.map((ch) => (
                      <OverviewMobileDetailCard
                        key={ch.channel_id}
                        detail={ch}
                        showChannel
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
              {t('Calls (Success / Error)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('First Token Time (P50 / P95)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Decode TPS (90% / Range)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Cache Hit Rate (Weighted / P50)')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={7}
                className='text-muted-foreground h-24 text-center text-sm'
              >
                {t('No matching models found')}
              </TableCell>
            </TableRow>
          ) : (
            props.rows.map((row) => {
              const isExpanded = Boolean(props.expanded[row.model])
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
                      <span className='font-medium'>
                        {formatNumber(row.request_count, props.intlLocale)}
                      </span>
                      <span className='text-muted-foreground ms-1 text-[10px]'>
                        ({row.success_count} / {row.error_count})
                      </span>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-semibold'>
                          {formatTtftSec(row.median_ttft_sec)}
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          P95: {formatTtftSec(row.p95_ttft_sec)} ·{' '}
                          {formatTpsValue(row.prefill_tps_median)} tok/s
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-semibold'>
                          {formatTpsValue(row.decode_tps_trimmed_90)} t/s
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          {formatTpsValue(row.decode_tps_p5_low)} ~{' '}
                          {formatTpsValue(row.decode_tps_p95_high)} t/s
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end gap-0.5'>
                        <Badge
                          variant='outline'
                          className={cn(
                            'font-mono text-xs',
                            getCacheRateBadgeClass(row.weighted_cache_rate)
                          )}
                        >
                          {formatPercentValue(row.weighted_cache_rate, 1)}
                        </Badge>
                        <span className='text-muted-foreground text-[10px]'>
                          P50: {formatPercentValue(row.cache_rate_median, 1)}
                        </span>
                      </div>
                    </TableCell>
                  </TableRow>

                  {isExpanded && (
                    <TableRow className='bg-muted/20'>
                      <TableCell colSpan={7} className='p-4'>
                        <div className='bg-background rounded-lg border p-3'>
                          <div className='mb-2.5 flex flex-wrap items-center justify-between gap-2'>
                            <span className='text-xs font-semibold'>
                              {t('Same-Model Channel Health Comparison')} (
                              {row.channels.length})
                            </span>
                            <div className='flex items-center gap-2'>
                              <Button
                                variant='outline'
                                size='sm'
                                className='h-7 gap-1 text-xs'
                                onClick={() => props.onSelectSubTab('speed')}
                              >
                                <Timer aria-hidden='true' className='size-3' />
                                {t('Response Speed')}
                              </Button>
                              <Button
                                variant='outline'
                                size='sm'
                                className='h-7 gap-1 text-xs'
                                onClick={() => props.onSelectSubTab('cache')}
                              >
                                <DatabaseZap
                                  aria-hidden='true'
                                  className='size-3'
                                />
                                {t('Model Cache Analysis')}
                              </Button>
                            </div>
                          </div>
                          <OverviewNestedTable
                            items={row.channels}
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

function OverviewChannelsTable(props: {
  rows: ChannelHealthSummaryRow[]
  expanded: Record<number, boolean>
  onToggle: (channelId: number) => void
  onSelectSubTab: (tab: HealthSubTab) => void
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
                      getAvailabilityBadgeClass(row.availability)
                    )}
                  >
                    {row.availability.toFixed(1)}%
                  </Badge>
                </button>

                <div className='bg-muted/20 grid grid-cols-3 gap-2 rounded-md p-2 font-mono text-[11px]'>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      TTFT (P50)
                    </span>
                    <span>{formatTtftSec(row.median_ttft_sec)}</span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      TPS (90%)
                    </span>
                    <span>{formatTpsValue(row.decode_tps_trimmed_90)} t/s</span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Cache Rate')}
                    </span>
                    <span>
                      {formatPercentValue(row.weighted_cache_rate, 1)}
                    </span>
                  </div>
                </div>

                {isExpanded && (
                  <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                    {row.models.map((m) => (
                      <OverviewMobileDetailCard
                        key={`${row.channel_id}-${m.model}`}
                        detail={m}
                        showChannel={false}
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
              {t('Calls (Success / Error)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('First Token Time (P50 / P95)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Decode TPS (90% / Range)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Cache Hit Rate (Weighted / P50)')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.rows.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={7}
                className='text-muted-foreground h-24 text-center text-sm'
              >
                {t('No matching channels found')}
              </TableCell>
            </TableRow>
          ) : (
            props.rows.map((row) => {
              const isExpanded = Boolean(props.expanded[row.channel_id])
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
                      <span className='font-medium'>
                        {formatNumber(row.request_count, props.intlLocale)}
                      </span>
                      <span className='text-muted-foreground ms-1 text-[10px]'>
                        ({row.success_count} / {row.error_count})
                      </span>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-semibold'>
                          {formatTtftSec(row.median_ttft_sec)}
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          P95: {formatTtftSec(row.p95_ttft_sec)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-semibold'>
                          {formatTpsValue(row.decode_tps_trimmed_90)} t/s
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          {formatTpsValue(row.decode_tps_p5_low)} ~{' '}
                          {formatTpsValue(row.decode_tps_p95_high)} t/s
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end gap-0.5'>
                        <Badge
                          variant='outline'
                          className={cn(
                            'font-mono text-xs',
                            getCacheRateBadgeClass(row.weighted_cache_rate)
                          )}
                        >
                          {formatPercentValue(row.weighted_cache_rate, 1)}
                        </Badge>
                        <span className='text-muted-foreground text-[10px]'>
                          P50: {formatPercentValue(row.cache_rate_median, 1)}
                        </span>
                      </div>
                    </TableCell>
                  </TableRow>

                  {isExpanded && (
                    <TableRow className='bg-muted/20'>
                      <TableCell colSpan={7} className='p-4'>
                        <div className='bg-background rounded-lg border p-3'>
                          <div className='mb-2.5 flex flex-wrap items-center justify-between gap-2'>
                            <span className='text-xs font-semibold'>
                              {t('Channel Model Performance Breakdown')} (
                              {row.models.length})
                            </span>
                            <div className='flex items-center gap-2'>
                              <Button
                                variant='outline'
                                size='sm'
                                className='h-7 gap-1 text-xs'
                                onClick={() => props.onSelectSubTab('speed')}
                              >
                                <Timer aria-hidden='true' className='size-3' />
                                {t('Response Speed')}
                              </Button>
                              <Button
                                variant='outline'
                                size='sm'
                                className='h-7 gap-1 text-xs'
                                onClick={() => props.onSelectSubTab('cache')}
                              >
                                <DatabaseZap
                                  aria-hidden='true'
                                  className='size-3'
                                />
                                {t('Model Cache Analysis')}
                              </Button>
                            </div>
                          </div>
                          <OverviewNestedTable
                            items={row.models}
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

function OverviewNestedTable(props: {
  items: ModelChannelHealthDetail[]
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
            {t('Calls (Success / Error)')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('First Token Time (P50 / P95)')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Decode TPS (90% / Range)')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Cache Hit Rate (Weighted / P50)')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.items.map((item) => (
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
              <span>{formatNumber(item.request_count, props.intlLocale)}</span>
              <span className='text-muted-foreground ms-1 text-[10px]'>
                ({item.success_count} / {item.error_count})
              </span>
            </TableCell>
            <TableCell className='py-1.5 text-right font-mono text-xs'>
              {formatTtftSec(item.median_ttft_sec)}{' '}
              <span className='text-muted-foreground text-[10px]'>
                (P95: {formatTtftSec(item.p95_ttft_sec)})
              </span>
            </TableCell>
            <TableCell className='py-1.5 text-right font-mono text-xs'>
              <span className='font-semibold'>
                {formatTpsValue(item.decode_tps_trimmed_90)} t/s
              </span>{' '}
              <span className='text-muted-foreground text-[10px]'>
                ({formatTpsValue(item.decode_tps_p5_low)}~
                {formatTpsValue(item.decode_tps_p95_high)})
              </span>
            </TableCell>
            <TableCell className='py-1.5 text-right font-mono text-xs'>
              <span className='font-semibold text-sky-600 dark:text-sky-400'>
                {formatPercentValue(item.weighted_cache_rate, 1)}
              </span>{' '}
              <span className='text-muted-foreground text-[10px]'>
                (P50: {formatPercentValue(item.cache_rate_median, 1)})
              </span>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function OverviewMobileDetailCard(props: {
  detail: ModelChannelHealthDetail
  showChannel: boolean
}) {
  const { t } = useTranslation()
  const d = props.detail

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
            getAvailabilityBadgeClass(d.availability)
          )}
        >
          {d.availability.toFixed(1)}% ({d.request_count})
        </Badge>
      </div>
      <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 font-mono text-[11px]'>
        <span>TTFT: {formatTtftSec(d.median_ttft_sec)}</span>
        <span>TPS: {formatTpsValue(d.decode_tps_trimmed_90)} t/s</span>
        <span>
          {t('Cache Rate')}: {formatPercentValue(d.weighted_cache_rate, 1)}
        </span>
      </div>
    </div>
  )
}
