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
import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  ChevronDown,
  ChevronRight,
  Cpu,
  DatabaseZap,
  Gauge,
  HeartPulse,
  Info,
  Server,
  Timer,
  Zap,
} from 'lucide-react'
import { Fragment, useEffect, useMemo, useState } from 'react'
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
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  getModelHealthAnalysis,
  type ChannelHealthSummaryRow,
  type ChannelHealthTrendSeries,
  type ModelChannelHealthDetail,
  type ModelHealthAnalysisRow,
  type ModelHealthTrendPoint,
} from '@/features/channels/api'
import { ChannelTypeLogo } from '@/features/channels/components/channel-type-badge'
import { useIsCompactScreen } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber, formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import {
  computeStartTimestamp,
  type TimeGranularity,
  type TimeRangeKey,
} from '@/lib/time'
import { cn } from '@/lib/utils'

import { ComprehensiveHealthTab } from './health/comprehensive-health-tab'
import { ModelHealthCharts } from './health/health-charts'
import {
  HealthToolbarControls,
  TpsDistributionBar,
} from './health/health-shared'
import {
  formatTpsValue,
  formatTtftSec,
  getAvailabilityBadgeClass,
  normalizeHealthSubTab,
  type HealthEntityViewMode,
  type HealthSubTab,
} from './health/health-utils'
import { ModelCacheAnalysisTab } from './health/model-cache-analysis-tab'

export function ModelHealthPanel() {
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const isMobile = useIsCompactScreen()
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const searchParams = new URLSearchParams(location.searchStr || '')
  const urlSubTab = normalizeHealthSubTab(searchParams.get('tab'))
  const [subTab, setSubTab] = useState<HealthSubTab>(urlSubTab)
  const [viewMode, setViewMode] = useState<HealthEntityViewMode>('models')
  const [timeRange, setTimeRange] = useState<TimeRangeKey>('1d')
  const [keyword, setKeyword] = useState('')
  const [onlyActiveCalls, setOnlyActiveCalls] = useState(false)
  const [expandedModels, setExpandedModels] = useState<Record<string, boolean>>(
    {}
  )
  const [expandedChannels, setExpandedChannels] = useState<
    Record<number, boolean>
  >({})

  useEffect(() => {
    setSubTab(urlSubTab)
  }, [urlSubTab])

  const handleSubTabChange = (raw: string) => {
    const next = normalizeHealthSubTab(raw)
    setSubTab(next)
    void navigate({
      to: '/dashboard/$section',
      params: { section: 'health' },
      search: (prev: Record<string, unknown>) => ({
        ...prev,
        tab: next === 'overview' ? undefined : next,
      }),
    })
  }

  const startTimestamp = useMemo(
    () => computeStartTimestamp(timeRange),
    [timeRange]
  )

  const healthQuery = useQuery({
    queryKey: ['channel-model-health', timeRange],
    queryFn: async () =>
      requireServerSuccess(
        await getModelHealthAnalysis({
          start_timestamp: startTimestamp,
        })
      ),
  })

  const rawModels: ModelHealthAnalysisRow[] = useMemo(
    () => healthQuery.data?.data?.models ?? [],
    [healthQuery.data]
  )
  const rawChannels: ChannelHealthSummaryRow[] = useMemo(
    () => healthQuery.data?.data?.channels ?? [],
    [healthQuery.data]
  )
  const trend: ModelHealthTrendPoint[] = useMemo(
    () => healthQuery.data?.data?.trend ?? [],
    [healthQuery.data]
  )
  const channelTrends: ChannelHealthTrendSeries[] = useMemo(
    () => healthQuery.data?.data?.channel_trends ?? [],
    [healthQuery.data]
  )
  const granularity: TimeGranularity =
    healthQuery.data?.data?.trend_granularity === 'hour' ? 'hour' : 'day'
  const summary = healthQuery.data?.data?.summary

  const kw = keyword.trim().toLowerCase()

  const filteredModels = useMemo(
    () =>
      rawModels.filter((row) => {
        if (onlyActiveCalls && row.request_count <= 0) {
          return false
        }
        if (!kw) {
          return true
        }
        if (row.model.toLowerCase().includes(kw)) {
          return true
        }
        return row.channels.some(
          (ch) =>
            ch.channel_name.toLowerCase().includes(kw) ||
            String(ch.channel_id).includes(kw)
        )
      }),
    [rawModels, onlyActiveCalls, kw]
  )

  const filteredChannels = useMemo(
    () =>
      rawChannels.filter((row) => {
        if (onlyActiveCalls && row.request_count <= 0) {
          return false
        }
        if (!kw) {
          return true
        }
        if (
          row.channel_name.toLowerCase().includes(kw) ||
          String(row.channel_id).includes(kw)
        ) {
          return true
        }
        return row.models.some((m) => m.model.toLowerCase().includes(kw))
      }),
    [rawChannels, onlyActiveCalls, kw]
  )

  const maxModelP95 = useMemo(() => {
    let maxVal = 100
    for (const m of filteredModels) {
      if (m.decode_tps_p95_high > maxVal) {
        maxVal = m.decode_tps_p95_high
      }
    }
    return maxVal
  }, [filteredModels])

  const maxChannelP95 = useMemo(() => {
    let maxVal = 100
    for (const c of filteredChannels) {
      if (c.decode_tps_p95_high > maxVal) {
        maxVal = c.decode_tps_p95_high
      }
    }
    return maxVal
  }, [filteredChannels])

  const toggleModelExpand = (modelName: string) => {
    setExpandedModels((prev) => ({
      ...prev,
      [modelName]: !prev[modelName],
    }))
  }

  const toggleChannelExpand = (channelId: number) => {
    setExpandedChannels((prev) => ({
      ...prev,
      [channelId]: !prev[channelId],
    }))
  }

  return (
    <TooltipProvider>
      <div className='space-y-4'>
        {/* Top Sub-page Tabs: Comprehensive Health / Response Speed / Model Cache Analysis */}
        <Tabs
          value={subTab}
          onValueChange={(value) => handleSubTabChange(String(value))}
          className='w-full'
        >
          <div className='max-w-full overflow-x-auto pb-0.5'>
            <TabsList className='inline-flex h-auto min-w-full justify-start gap-1 p-1 sm:min-w-0 sm:flex-wrap'>
              <TabsTrigger
                value='overview'
                className='shrink-0 gap-1.5 text-xs'
              >
                <HeartPulse aria-hidden='true' className='size-3.5 shrink-0' />
                <span>{t('Comprehensive Model Health')}</span>
              </TabsTrigger>
              <TabsTrigger value='speed' className='shrink-0 gap-1.5 text-xs'>
                <Timer aria-hidden='true' className='size-3.5 shrink-0' />
                <span>{t('Response Speed')}</span>
              </TabsTrigger>
              <TabsTrigger value='cache' className='shrink-0 gap-1.5 text-xs'>
                <DatabaseZap aria-hidden='true' className='size-3.5 shrink-0' />
                <span>{t('Model Cache Analysis')}</span>
              </TabsTrigger>
            </TabsList>
          </div>
        </Tabs>

        {subTab === 'overview' && (
          <ComprehensiveHealthTab
            summary={summary}
            models={filteredModels}
            channels={filteredChannels}
            trend={trend}
            channelTrends={channelTrends}
            granularity={granularity}
            viewMode={viewMode}
            onViewModeChange={setViewMode}
            keyword={keyword}
            onKeywordChange={setKeyword}
            timeRange={timeRange}
            onTimeRangeChange={setTimeRange}
            onlyActiveCalls={onlyActiveCalls}
            onToggleOnlyActiveCalls={() => setOnlyActiveCalls((prev) => !prev)}
            isLoading={healthQuery.isLoading}
            isFetching={healthQuery.isFetching}
            onRefresh={() => void healthQuery.refetch()}
            onSelectSubTab={handleSubTabChange}
            isMobile={isMobile}
            intlLocale={intlLocale}
          />
        )}

        {subTab === 'cache' && (
          <ModelCacheAnalysisTab
            summary={summary}
            models={filteredModels}
            channels={filteredChannels}
            trend={trend}
            channelTrends={channelTrends}
            granularity={granularity}
            viewMode={viewMode}
            onViewModeChange={setViewMode}
            keyword={keyword}
            onKeywordChange={setKeyword}
            timeRange={timeRange}
            onTimeRangeChange={setTimeRange}
            onlyActiveCalls={onlyActiveCalls}
            onToggleOnlyActiveCalls={() => setOnlyActiveCalls((prev) => !prev)}
            isLoading={healthQuery.isLoading}
            isFetching={healthQuery.isFetching}
            onRefresh={() => void healthQuery.refetch()}
            isMobile={isMobile}
            intlLocale={intlLocale}
          />
        )}

        {subTab === 'speed' && (
          <div className='space-y-4'>
            {/* Top Speed KPI Summary Cards */}
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
                  <CardTitle className='flex items-baseline gap-2 text-xl font-bold'>
                    <span className='text-emerald-600 dark:text-emerald-400'>
                      {(summary?.availability ?? 100).toFixed(2)}%
                    </span>
                    <span className='text-muted-foreground font-mono text-xs font-normal'>
                      {formatNumber(summary?.request_count ?? 0, intlLocale)}{' '}
                      {t('calls')} ({summary?.model_count ?? 0} {t('Models')} /{' '}
                      {summary?.channel_count ?? 0} {t('Channels')})
                    </span>
                  </CardTitle>
                </CardHeader>
              </Card>

              <Card>
                <CardHeader className='pb-2'>
                  <CardDescription className='flex items-center justify-between text-xs'>
                    <span className='flex items-center gap-1'>
                      {t('Context Prefill Speed & TTFT')}
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Info className='text-muted-foreground size-3.5 cursor-help' />
                          }
                        />
                        <TooltipContent className='max-w-xs text-xs'>
                          {t(
                            'Prefill TPS = Prompt Tokens ÷ First Token Time (TTFT). Evaluates how fast the upstream processes long context prompts before generating the first token.'
                          )}
                        </TooltipContent>
                      </Tooltip>
                    </span>
                    <Zap aria-hidden='true' className='size-4 text-amber-500' />
                  </CardDescription>
                  <CardTitle className='flex items-baseline gap-2 text-xl font-bold'>
                    <span>
                      {formatTpsValue(summary?.prefill_tps_median ?? 0)}{' '}
                      <span className='text-muted-foreground text-xs font-normal'>
                        tok/s
                      </span>
                    </span>
                    <Badge variant='outline' className='font-mono text-xs'>
                      TTFT: {formatTtftSec(summary?.avg_ttft_sec ?? 0)}
                    </Badge>
                  </CardTitle>
                </CardHeader>
              </Card>

              <Card>
                <CardHeader className='pb-2'>
                  <CardDescription className='flex items-center justify-between text-xs'>
                    <span className='flex items-center gap-1'>
                      {t('Decode TPS (5% Low / 90% / 5% High)')}
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Info className='text-muted-foreground size-3.5 cursor-help' />
                          }
                        />
                        <TooltipContent className='max-w-xs text-xs'>
                          {t(
                            'Post-TTFT Decode Speed = Completion Tokens ÷ (Total Duration - TTFT). Excludes top 5% and bottom 5% outliers for the 90% trimmed average.'
                          )}
                        </TooltipContent>
                      </Tooltip>
                    </span>
                    <Gauge aria-hidden='true' className='text-primary size-4' />
                  </CardDescription>
                  <CardTitle className='flex items-baseline gap-2 font-mono text-lg font-bold'>
                    <span className='text-amber-600 dark:text-amber-400'>
                      {formatTpsValue(summary?.decode_tps_p5_low ?? 0)}
                    </span>
                    <span className='text-muted-foreground text-xs'>/</span>
                    <span className='text-xl'>
                      {formatTpsValue(summary?.decode_tps_trimmed_90 ?? 0)}
                    </span>
                    <span className='text-muted-foreground text-xs'>/</span>
                    <span className='text-emerald-600 dark:text-emerald-400'>
                      {formatTpsValue(summary?.decode_tps_p95_high ?? 0)}
                    </span>
                    <span className='text-muted-foreground text-xs font-normal'>
                      t/s
                    </span>
                  </CardTitle>
                </CardHeader>
              </Card>

              <Card>
                <CardHeader className='pb-2'>
                  <CardDescription className='flex items-center justify-between text-xs'>
                    <span className='flex items-center gap-1'>
                      {t('End-to-End Total TPS Average')}
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Info className='text-muted-foreground size-3.5 cursor-help' />
                          }
                        />
                        <TooltipContent className='max-w-xs text-xs'>
                          {t(
                            'End-to-End Total TPS = Completion Tokens ÷ Total Request Duration (including prefill + generation time).'
                          )}
                        </TooltipContent>
                      </Tooltip>
                    </span>
                    <Activity
                      aria-hidden='true'
                      className='text-muted-foreground size-4'
                    />
                  </CardDescription>
                  <CardTitle className='flex items-baseline gap-2 text-xl font-bold'>
                    <span>
                      {formatTpsValue(summary?.total_tps_avg ?? 0)}{' '}
                      <span className='text-muted-foreground text-xs font-normal'>
                        tok/s
                      </span>
                    </span>
                    <span className='text-muted-foreground font-mono text-xs font-normal'>
                      ({t('90% Trimmed')}:{' '}
                      {formatTpsValue(summary?.total_tps_trimmed_90 ?? 0)} t/s)
                    </span>
                  </CardTitle>
                </CardHeader>
              </Card>
            </div>

            {/* Speed Trend & Ranking Charts */}
            <ModelHealthCharts
              mode='speed'
              entityViewMode={viewMode}
              trend={trend}
              channelTrends={channelTrends}
              granularity={granularity}
              models={filteredModels}
              channels={filteredChannels}
              isLoading={healthQuery.isLoading}
            />

            {/* Main Speed Analysis Card */}
            <Card>
              <CardHeader className='pb-3'>
                <div className='flex flex-wrap items-center justify-between gap-3'>
                  <div className='space-y-1'>
                    <div className='flex flex-wrap items-center gap-2.5'>
                      <CardTitle className='text-base'>
                        {t('Model & Channel Response Speed Matrix')}
                      </CardTitle>
                      <Tabs
                        value={viewMode}
                        onValueChange={(val) =>
                          setViewMode(val as HealthEntityViewMode)
                        }
                      >
                        <TabsList className='h-8'>
                          <TabsTrigger
                            value='models'
                            className='px-2.5 text-xs'
                          >
                            <Cpu
                              aria-hidden='true'
                              className='me-1.5 size-3.5'
                            />
                            {t('By Model Average')} ({filteredModels.length})
                          </TabsTrigger>
                          <TabsTrigger
                            value='channels'
                            className='px-2.5 text-xs'
                          >
                            <Server
                              aria-hidden='true'
                              className='me-1.5 size-3.5'
                            />
                            {t('By Channel Average')} ({filteredChannels.length}
                            )
                          </TabsTrigger>
                        </TabsList>
                      </Tabs>
                    </div>
                    <CardDescription className='text-xs'>
                      {viewMode === 'models'
                        ? t(
                            'Click any model row to compare health, prefill speed, and decode TPS distribution across all upstream channels providing that model'
                          )
                        : t(
                            'Click any channel row to inspect overall channel health and per-model prefill/decode performance breakdown'
                          )}
                    </CardDescription>
                  </div>

                  <HealthToolbarControls
                    keyword={keyword}
                    onKeywordChange={setKeyword}
                    timeRange={timeRange}
                    onTimeRangeChange={setTimeRange}
                    onlyActiveCalls={onlyActiveCalls}
                    onToggleOnlyActiveCalls={() =>
                      setOnlyActiveCalls((prev) => !prev)
                    }
                    isFetching={healthQuery.isFetching}
                    onRefresh={() => void healthQuery.refetch()}
                  />
                </div>
              </CardHeader>

              <CardContent className='p-0'>
                {healthQuery.isLoading && (
                  <div className='space-y-2 p-4'>
                    <Skeleton className='h-12 w-full' />
                    <Skeleton className='h-12 w-full' />
                    <Skeleton className='h-12 w-full' />
                  </div>
                )}

                {!healthQuery.isLoading && viewMode === 'models' && (
                  <ModelsHealthTable
                    rows={filteredModels}
                    expandedModels={expandedModels}
                    onToggleExpand={toggleModelExpand}
                    maxScale={maxModelP95}
                    isMobile={isMobile}
                    intlLocale={intlLocale}
                  />
                )}

                {!healthQuery.isLoading && viewMode === 'channels' && (
                  <ChannelsHealthTable
                    rows={filteredChannels}
                    expandedChannels={expandedChannels}
                    onToggleExpand={toggleChannelExpand}
                    maxScale={maxChannelP95}
                    isMobile={isMobile}
                    intlLocale={intlLocale}
                  />
                )}
              </CardContent>
            </Card>
          </div>
        )}
      </div>
    </TooltipProvider>
  )
}

function ModelsHealthTable(props: {
  rows: ModelHealthAnalysisRow[]
  expandedModels: Record<string, boolean>
  onToggleExpand: (modelName: string) => void
  maxScale: number
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
            const isExpanded = Boolean(props.expandedModels[row.model])
            return (
              <div
                key={row.model}
                className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
              >
                <button
                  type='button'
                  aria-expanded={isExpanded}
                  onClick={() => props.onToggleExpand(row.model)}
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
                    <Badge variant='outline' className='shrink-0 text-[10px]'>
                      {row.channel_count} {t('Channels')}
                    </Badge>
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

                <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 text-xs'>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Context Prefill Speed')} / TTFT
                    </span>
                    <span className='font-mono text-[11px] font-semibold'>
                      {formatTpsValue(row.prefill_tps_median)} tok/s ·{' '}
                      {formatTtftSec(row.avg_ttft_sec)}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Decode TPS (P5 / 90% / P95)')}
                    </span>
                    <span className='font-mono text-[11px] font-semibold'>
                      {formatTpsValue(row.decode_tps_p5_low)} /{' '}
                      {formatTpsValue(row.decode_tps_trimmed_90)} /{' '}
                      {formatTpsValue(row.decode_tps_p95_high)}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Total TPS Avg')} / {t('Decode Avg')}
                    </span>
                    <span className='font-mono text-[11px]'>
                      {formatTpsValue(row.total_tps_avg)} /{' '}
                      {formatTpsValue(row.decode_tps_avg)} t/s
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Call Count')} / {t('Avg Context')}
                    </span>
                    <span className='font-mono text-[11px]'>
                      {formatNumber(row.request_count, props.intlLocale)} ·{' '}
                      {formatCompactNumber(
                        row.avg_prompt_tokens,
                        props.intlLocale
                      )}
                    </span>
                  </div>
                </div>

                {isExpanded && (
                  <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                    <div className='text-xs font-semibold'>
                      {t('Same-Model Channel Health Comparison')} (
                      {row.channels.length})
                    </div>
                    {row.channels.map((ch) => (
                      <MobileChannelHealthCard
                        key={ch.channel_id}
                        detail={ch}
                        showChannelTitle
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
              {t('Calls & Avg Context')}
            </TableHead>
            <TableHead className='text-right'>
              <div className='flex items-center justify-end gap-1'>
                <span>{t('Context Prefill Speed')}</span>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Info className='text-muted-foreground size-3 cursor-help' />
                    }
                  />
                  <TooltipContent className='max-w-xs text-xs'>
                    {t(
                      'Context Tokens ÷ First Token Time (TTFT). Measures prompt processing throughput alongside average TTFT.'
                    )}
                  </TooltipContent>
                </Tooltip>
              </div>
            </TableHead>
            <TableHead>
              <div className='flex items-center gap-1'>
                <span>{t('Decode TPS (5% Low · 90% Median · 5% High)')}</span>
              </div>
            </TableHead>
            <TableHead className='text-right'>
              {t('Post-TTFT Decode Avg')}
            </TableHead>
            <TableHead className='text-right'>{t('Total TPS Avg')}</TableHead>
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
              const isExpanded = Boolean(props.expandedModels[row.model])
              return (
                <Fragment key={row.model}>
                  <TableRow
                    tabIndex={0}
                    aria-expanded={isExpanded}
                    className='hover:bg-muted/50 cursor-pointer'
                    onClick={() => props.onToggleExpand(row.model)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        props.onToggleExpand(row.model)
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
                          {t('In')}:{' '}
                          {formatCompactNumber(
                            row.avg_prompt_tokens,
                            props.intlLocale
                          )}{' '}
                          / {t('Out')}:{' '}
                          {formatCompactNumber(
                            row.avg_output_tokens,
                            props.intlLocale
                          )}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-semibold text-amber-600 dark:text-amber-400'>
                          {formatTpsValue(row.prefill_tps_median)} tok/s
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          TTFT: {formatTtftSec(row.avg_ttft_sec)} (P95:{' '}
                          {formatTtftSec(row.p95_ttft_sec)})
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <TpsDistributionBar
                        p5Low={row.decode_tps_p5_low}
                        trimmed90={row.decode_tps_trimmed_90}
                        p95High={row.decode_tps_p95_high}
                        maxScale={props.maxScale}
                      />
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs font-semibold'>
                      {formatTpsValue(row.decode_tps_avg)} t/s
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-medium'>
                          {formatTpsValue(row.total_tps_avg)} t/s
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          90%: {formatTpsValue(row.total_tps_trimmed_90)} t/s
                        </span>
                      </div>
                    </TableCell>
                  </TableRow>

                  {isExpanded && (
                    <TableRow className='bg-muted/20'>
                      <TableCell colSpan={8} className='p-4'>
                        <div className='bg-background rounded-lg border p-3'>
                          <div className='mb-2.5 flex items-center justify-between'>
                            <div className='flex items-center gap-1.5 text-xs font-semibold'>
                              <Server
                                aria-hidden='true'
                                className='text-primary size-3.5'
                              />
                              <span>
                                {t('Same-Model Channel Health Comparison')} (
                                {row.channels.length})
                              </span>
                            </div>
                            <span className='text-muted-foreground text-[11px]'>
                              {t(
                                'Sorted by call volume & 90% trimmed decode speed'
                              )}
                            </span>
                          </div>
                          <NestedChannelHealthTable
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

function ChannelsHealthTable(props: {
  rows: ChannelHealthSummaryRow[]
  expandedChannels: Record<number, boolean>
  onToggleExpand: (channelId: number) => void
  maxScale: number
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
            const isExpanded = Boolean(props.expandedChannels[row.channel_id])
            return (
              <div
                key={row.channel_id}
                className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
              >
                <button
                  type='button'
                  aria-expanded={isExpanded}
                  onClick={() => props.onToggleExpand(row.channel_id)}
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

                <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 text-xs'>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Context Prefill Speed')} / TTFT
                    </span>
                    <span className='font-mono text-[11px] font-semibold'>
                      {formatTpsValue(row.prefill_tps_median)} tok/s ·{' '}
                      {formatTtftSec(row.avg_ttft_sec)}
                    </span>
                  </div>
                  <div>
                    <span className='text-muted-foreground block text-[10px]'>
                      {t('Decode TPS (P5 / 90% / P95)')}
                    </span>
                    <span className='font-mono text-[11px] font-semibold'>
                      {formatTpsValue(row.decode_tps_p5_low)} /{' '}
                      {formatTpsValue(row.decode_tps_trimmed_90)} /{' '}
                      {formatTpsValue(row.decode_tps_p95_high)}
                    </span>
                  </div>
                </div>

                {isExpanded && (
                  <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                    {row.models.map((m) => (
                      <MobileChannelHealthCard
                        key={`${row.channel_id}-${m.model}`}
                        detail={m}
                        showChannelTitle={false}
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
              {t('Calls & Avg Context')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Context Prefill Speed')}
            </TableHead>
            <TableHead>
              {t('Decode TPS (5% Low · 90% Median · 5% High)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Post-TTFT Decode Avg')}
            </TableHead>
            <TableHead className='text-right'>{t('Total TPS Avg')}</TableHead>
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
              const isExpanded = Boolean(props.expandedChannels[row.channel_id])
              return (
                <Fragment key={row.channel_id}>
                  <TableRow
                    tabIndex={0}
                    aria-expanded={isExpanded}
                    className='hover:bg-muted/50 cursor-pointer'
                    onClick={() => props.onToggleExpand(row.channel_id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        props.onToggleExpand(row.channel_id)
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
                          {t('In')}:{' '}
                          {formatCompactNumber(
                            row.avg_prompt_tokens,
                            props.intlLocale
                          )}{' '}
                          / {t('Out')}:{' '}
                          {formatCompactNumber(
                            row.avg_output_tokens,
                            props.intlLocale
                          )}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-semibold text-amber-600 dark:text-amber-400'>
                          {formatTpsValue(row.prefill_tps_median)} tok/s
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          TTFT: {formatTtftSec(row.avg_ttft_sec)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <TpsDistributionBar
                        p5Low={row.decode_tps_p5_low}
                        trimmed90={row.decode_tps_trimmed_90}
                        p95High={row.decode_tps_p95_high}
                        maxScale={props.maxScale}
                      />
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs font-semibold'>
                      {formatTpsValue(row.decode_tps_avg)} t/s
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs'>
                      <div className='flex flex-col items-end'>
                        <span className='font-medium'>
                          {formatTpsValue(row.total_tps_avg)} t/s
                        </span>
                        <span className='text-muted-foreground text-[10px]'>
                          90%: {formatTpsValue(row.total_tps_trimmed_90)} t/s
                        </span>
                      </div>
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
                              {t('Channel Model Performance Breakdown')} (
                              {row.models.length})
                            </span>
                          </div>
                          <NestedChannelHealthTable
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

function NestedChannelHealthTable(props: {
  items: ModelChannelHealthDetail[]
  showChannelCol: boolean
  intlLocale?: string
}) {
  const { t } = useTranslation()
  const maxLocalP95 = useMemo(() => {
    let m = 100
    for (const item of props.items) {
      if (item.decode_tps_p95_high > m) {
        m = item.decode_tps_p95_high
      }
    }
    return m
  }, [props.items])

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
            {t('Avg Context (In / Out)')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Context Prefill Speed & TTFT')}
          </TableHead>
          <TableHead className='h-8 text-[11px]'>
            {t('Decode TPS (5% Low · 90% · 5% High)')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Post-TTFT Decode Avg')}
          </TableHead>
          <TableHead className='h-8 text-right text-[11px]'>
            {t('Total TPS Avg')}
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
              {formatCompactNumber(item.avg_prompt_tokens, props.intlLocale)} /{' '}
              {formatCompactNumber(item.avg_output_tokens, props.intlLocale)}
            </TableCell>
            <TableCell className='py-1.5 text-right font-mono text-xs'>
              <div className='flex flex-col items-end'>
                <span className='font-semibold text-amber-600 dark:text-amber-400'>
                  {formatTpsValue(item.prefill_tps_median)} tok/s
                </span>
                <span className='text-muted-foreground text-[10px]'>
                  TTFT: {formatTtftSec(item.avg_ttft_sec)}
                </span>
              </div>
            </TableCell>
            <TableCell className='py-1.5'>
              <TpsDistributionBar
                p5Low={item.decode_tps_p5_low}
                trimmed90={item.decode_tps_trimmed_90}
                p95High={item.decode_tps_p95_high}
                maxScale={maxLocalP95}
              />
            </TableCell>
            <TableCell className='py-1.5 text-right font-mono text-xs font-semibold'>
              {formatTpsValue(item.decode_tps_avg)} t/s
            </TableCell>
            <TableCell className='py-1.5 text-right font-mono text-xs'>
              <div className='flex flex-col items-end'>
                <span>{formatTpsValue(item.total_tps_avg)} t/s</span>
                <span className='text-muted-foreground text-[10px]'>
                  90%: {formatTpsValue(item.total_tps_trimmed_90)} t/s
                </span>
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function MobileChannelHealthCard(props: {
  detail: ModelChannelHealthDetail
  showChannelTitle: boolean
  intlLocale?: string
}) {
  const { t } = useTranslation()
  const d = props.detail

  return (
    <div className='bg-muted/30 flex flex-col gap-1.5 rounded p-2 text-xs'>
      <div className='flex items-center justify-between gap-2'>
        {props.showChannelTitle ? (
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
        <span>
          <Timer aria-hidden='true' className='me-1 inline size-3' />
          {t('Prefill')}: {formatTpsValue(d.prefill_tps_median)} tok/s (
          {formatTtftSec(d.avg_ttft_sec)})
        </span>
        <span>
          {t('Decode')}: {formatTpsValue(d.decode_tps_p5_low)} /{' '}
          <strong className='text-foreground'>
            {formatTpsValue(d.decode_tps_trimmed_90)}
          </strong>{' '}
          / {formatTpsValue(d.decode_tps_p95_high)} t/s
        </span>
      </div>
    </div>
  )
}
