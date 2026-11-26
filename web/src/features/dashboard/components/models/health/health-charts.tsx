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
import { VChart } from '@visactor/react-vchart'
import { BarChart3, ChevronDown, ChevronUp } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import type {
  ChannelHealthSummaryRow,
  ChannelHealthTrendSeries,
  ModelHealthAnalysisRow,
  ModelHealthTrendPoint,
} from '@/features/channels/api'
import { getDashboardChartColors } from '@/features/dashboard/lib/charts'
import { useThemeRadiusPx } from '@/lib/theme-radius'
import { formatChartTime, type TimeGranularity } from '@/lib/time'
import { VCHART_OPTION } from '@/lib/vchart'

import type {
  CachePercentileScope,
  HealthEntityViewMode,
  HealthSubTab,
} from './health-utils'

let vchartThemePromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

type ChartTabOption = {
  id: string
  labelKey: string
}

const OVERVIEW_CHART_TABS: readonly ChartTabOption[] = [
  { id: 'channel-availability', labelKey: 'Channel Success Rate Trend' },
  { id: 'cache-band', labelKey: 'Cache Rate Trend (P5 / P50 / P95)' },
  { id: 'latency-speed', labelKey: 'TTFT & Decode TPS Trend' },
]

const SPEED_CHART_TABS: readonly ChartTabOption[] = [
  { id: 'tps-ranking', labelKey: 'TPS Distribution (P5 / 90% / P95)' },
  { id: 'ttft-trend', labelKey: 'TTFT Trend (P50 / Avg / P95)' },
  { id: 'tps-trend', labelKey: 'Decode TPS Trend (P5 / 90% / P95)' },
]

const CACHE_CHART_TABS: readonly ChartTabOption[] = [
  { id: 'cache-trend', labelKey: 'Cache Rate Trend (P5 / P50 / P95)' },
  { id: 'channel-cache-trend', labelKey: 'Channel Avg Cache Rate Trend' },
  { id: 'cache-ranking', labelKey: 'Cache Distribution Comparison' },
]

function getChartTabsForMode(mode: HealthSubTab): readonly ChartTabOption[] {
  if (mode === 'speed') {
    return SPEED_CHART_TABS
  }
  if (mode === 'cache') {
    return CACHE_CHART_TABS
  }
  return OVERVIEW_CHART_TABS
}

function truncateLabel(text: string, maxLen = 22): string {
  if (text.length <= maxLen) {
    return text
  }
  return `${text.slice(0, maxLen - 1)}…`
}

export type ModelHealthChartsProps = {
  mode: HealthSubTab
  entityViewMode: HealthEntityViewMode
  cacheScope?: CachePercentileScope
  trend: ModelHealthTrendPoint[]
  channelTrends: ChannelHealthTrendSeries[]
  granularity: TimeGranularity
  models: ModelHealthAnalysisRow[]
  channels: ChannelHealthSummaryRow[]
  isLoading?: boolean
}

export function ModelHealthCharts(props: ModelHealthChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const { customization } = useThemeCustomization()
  const chartRadius = useThemeRadiusPx(
    '--radius-md',
    `${customization.preset}:${customization.radius}`
  )

  const tabs = getChartTabsForMode(props.mode)
  const [selectedTab, setSelectedTab] = useState<string>(tabs[0].id)
  const [collapsed, setCollapsed] = useState(false)
  const [themeReady, setThemeReady] = useState(false)

  const activeChartTab = tabs.some((item) => item.id === selectedTab)
    ? selectedTab
    : tabs[0].id

  useEffect(() => {
    let active = true
    const syncTheme = async () => {
      if (!vchartThemePromise) {
        vchartThemePromise = import('@visactor/vchart').then(
          (m) => m.ThemeManager
        )
      }
      const ThemeManager = await vchartThemePromise
      if (!active) return
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    void syncTheme()
    return () => {
      active = false
    }
  }, [resolvedTheme])

  const hitOnly = props.cacheScope === 'hit'

  const chartPayload = useMemo(() => {
    const activePoints = props.trend.filter((pt) => pt.request_count > 0)
    const palette = getDashboardChartColors(10)

    if (activeChartTab === 'channel-availability') {
      const rows: Array<{
        time: string
        series: string
        value: number
        calls: number
        errors: number
      }> = []

      for (const pt of activePoints) {
        rows.push({
          time: formatChartTime(pt.timestamp, props.granularity),
          series: t('Overall Success Rate'),
          value: pt.availability,
          calls: pt.request_count,
          errors: pt.error_count,
        })
      }

      for (const ch of props.channelTrends.slice(0, 6)) {
        const label = `#${ch.channel_id} ${truncateLabel(ch.channel_name, 16)}`
        for (const pt of ch.points) {
          if (pt.request_count <= 0) continue
          rows.push({
            time: formatChartTime(pt.timestamp, props.granularity),
            series: label,
            value: pt.availability,
            calls: pt.request_count,
            errors: pt.error_count,
          })
        }
      }

      if (rows.length === 0) {
        return null
      }

      return {
        type: 'line',
        data: [{ id: 'healthData', values: rows }],
        xField: 'time',
        yField: 'value',
        seriesField: 'series',
        color: palette,
        line: { style: { curveType: 'monotone', lineWidth: 2 } },
        point: { visible: rows.length <= 48, style: { size: 4 } },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true, autoRotate: false } },
          {
            orient: 'left',
            min: 0,
            max: 100,
            label: { formatMethod: (val: number) => `${val}%` },
          },
        ],
        tooltip: {
          visible: true,
          dimension: {
            content: [
              {
                key: (d: Record<string, unknown>) => String(d.series ?? ''),
                value: (d: Record<string, unknown>) =>
                  `${Number(d.value ?? 0).toFixed(2)}% (${Number(d.calls ?? 0)} ${t('calls')}, ${Number(d.errors ?? 0)} ${t('Errors')})`,
              },
            ],
          },
        },
      }
    }

    if (activeChartTab === 'cache-band' || activeChartTab === 'cache-trend') {
      const consumePoints = activePoints.filter((pt) => pt.success_count > 0)
      if (consumePoints.length === 0) {
        return null
      }
      const rows: Array<{ time: string; metric: string; value: number }> = []
      for (const pt of consumePoints) {
        const timeStr = formatChartTime(pt.timestamp, props.granularity)
        const p5 = hitOnly ? pt.hit_cache_rate_p5_low : pt.cache_rate_p5_low
        const p50 = hitOnly ? pt.hit_cache_rate_median : pt.cache_rate_median
        const p95 = hitOnly
          ? pt.hit_cache_rate_p95_high
          : pt.cache_rate_p95_high
        rows.push(
          {
            time: timeStr,
            metric: t('Weighted Avg Cache Rate'),
            value: pt.weighted_cache_rate,
          },
          {
            time: timeStr,
            metric: t('Median (P50)'),
            value: p50,
          },
          {
            time: timeStr,
            metric: t('95% High (P95)'),
            value: p95,
          },
          {
            time: timeStr,
            metric: t('5% Low (P5)'),
            value: p5,
          }
        )
      }

      return {
        type: 'line',
        data: [{ id: 'cacheTrend', values: rows }],
        xField: 'time',
        yField: 'value',
        seriesField: 'metric',
        color: ['#0ea5e9', '#6366f1', '#10b981', '#f59e0b'],
        line: { style: { curveType: 'monotone', lineWidth: 2 } },
        point: { visible: consumePoints.length <= 36, style: { size: 4 } },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true } },
          {
            orient: 'left',
            min: 0,
            max: 100,
            label: { formatMethod: (val: number) => `${val}%` },
          },
        ],
        tooltip: {
          visible: true,
          dimension: {
            content: [
              {
                key: (d: Record<string, unknown>) => String(d.metric ?? ''),
                value: (d: Record<string, unknown>) =>
                  `${Number(d.value ?? 0).toFixed(2)}%`,
              },
            ],
          },
        },
      }
    }

    if (activeChartTab === 'channel-cache-trend') {
      const rows: Array<{
        time: string
        series: string
        value: number
        p50: number
      }> = []
      for (const ch of props.channelTrends.slice(0, 6)) {
        const label = `#${ch.channel_id} ${truncateLabel(ch.channel_name, 16)}`
        for (const pt of ch.points) {
          if (pt.success_count <= 0) continue
          rows.push({
            time: formatChartTime(pt.timestamp, props.granularity),
            series: label,
            value: pt.weighted_cache_rate,
            p50: hitOnly ? pt.hit_cache_rate_median : pt.cache_rate_median,
          })
        }
      }
      if (rows.length === 0) {
        return null
      }

      return {
        type: 'line',
        data: [{ id: 'chCacheTrend', values: rows }],
        xField: 'time',
        yField: 'value',
        seriesField: 'series',
        color: palette,
        line: { style: { curveType: 'monotone', lineWidth: 2 } },
        point: { visible: rows.length <= 48, style: { size: 4 } },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true } },
          {
            orient: 'left',
            min: 0,
            max: 100,
            label: { formatMethod: (val: number) => `${val}%` },
          },
        ],
        tooltip: {
          visible: true,
          dimension: {
            content: [
              {
                key: (d: Record<string, unknown>) => String(d.series ?? ''),
                value: (d: Record<string, unknown>) =>
                  `${Number(d.value ?? 0).toFixed(2)}% (P50: ${Number(d.p50 ?? 0).toFixed(1)}%)`,
              },
            ],
          },
        },
      }
    }

    if (activeChartTab === 'latency-speed' || activeChartTab === 'tps-trend') {
      const consumePoints = activePoints.filter((pt) => pt.success_count > 0)
      if (consumePoints.length === 0) {
        return null
      }
      const rows: Array<{ time: string; metric: string; value: number }> = []
      for (const pt of consumePoints) {
        const timeStr = formatChartTime(pt.timestamp, props.granularity)
        rows.push(
          {
            time: timeStr,
            metric: t('90% Trimmed Decode TPS'),
            value: pt.decode_tps_trimmed_90,
          },
          {
            time: timeStr,
            metric: t('95% High (P95)'),
            value: pt.decode_tps_p95_high,
          },
          {
            time: timeStr,
            metric: t('5% Low (P5)'),
            value: pt.decode_tps_p5_low,
          }
        )
      }

      return {
        type: 'line',
        data: [{ id: 'tpsTrend', values: rows }],
        xField: 'time',
        yField: 'value',
        seriesField: 'metric',
        color: ['#6366f1', '#10b981', '#f59e0b'],
        line: { style: { curveType: 'monotone', lineWidth: 2 } },
        point: { visible: consumePoints.length <= 36, style: { size: 4 } },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true } },
          {
            orient: 'left',
            min: 0,
            label: { formatMethod: (val: number) => `${val} t/s` },
          },
        ],
        tooltip: {
          visible: true,
          dimension: {
            content: [
              {
                key: (d: Record<string, unknown>) => String(d.metric ?? ''),
                value: (d: Record<string, unknown>) =>
                  `${Number(d.value ?? 0).toFixed(1)} tok/s`,
              },
            ],
          },
        },
      }
    }

    if (activeChartTab === 'ttft-trend') {
      const ttftPoints = activePoints.filter(
        (pt) => pt.avg_ttft_sec > 0 || pt.median_ttft_sec > 0
      )
      if (ttftPoints.length === 0) {
        return null
      }
      const rows: Array<{ time: string; metric: string; value: number }> = []
      for (const pt of ttftPoints) {
        const timeStr = formatChartTime(pt.timestamp, props.granularity)
        rows.push(
          {
            time: timeStr,
            metric: t('Median TTFT (P50)'),
            value: pt.median_ttft_sec,
          },
          {
            time: timeStr,
            metric: t('Average TTFT'),
            value: pt.avg_ttft_sec,
          },
          {
            time: timeStr,
            metric: t('P95 TTFT'),
            value: pt.p95_ttft_sec,
          }
        )
      }

      return {
        type: 'line',
        data: [{ id: 'ttftTrend', values: rows }],
        xField: 'time',
        yField: 'value',
        seriesField: 'metric',
        color: ['#10b981', '#0ea5e9', '#f59e0b'],
        line: { style: { curveType: 'monotone', lineWidth: 2 } },
        point: { visible: ttftPoints.length <= 36, style: { size: 4 } },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true } },
          {
            orient: 'left',
            min: 0,
            label: { formatMethod: (val: number) => `${val}s` },
          },
        ],
        tooltip: {
          visible: true,
          dimension: {
            content: [
              {
                key: (d: Record<string, unknown>) => String(d.metric ?? ''),
                value: (d: Record<string, unknown>) =>
                  `${Number(d.value ?? 0).toFixed(2)}s`,
              },
            ],
          },
        },
      }
    }

    if (activeChartTab === 'tps-ranking') {
      const items =
        props.entityViewMode === 'channels'
          ? props.channels
              .filter((item) => item.decode_tps_trimmed_90 > 0)
              .slice(0, 10)
              .map((item) => ({
                target: `#${item.channel_id} ${truncateLabel(item.channel_name, 16)}`,
                p5: item.decode_tps_p5_low,
                mid: item.decode_tps_trimmed_90,
                p95: item.decode_tps_p95_high,
              }))
          : props.models
              .filter((item) => item.decode_tps_trimmed_90 > 0)
              .slice(0, 10)
              .map((item) => ({
                target: truncateLabel(item.model, 22),
                p5: item.decode_tps_p5_low,
                mid: item.decode_tps_trimmed_90,
                p95: item.decode_tps_p95_high,
              }))

      if (items.length === 0) {
        return null
      }

      const rows: Array<{ target: string; metric: string; value: number }> = []
      for (const item of items) {
        rows.push(
          { target: item.target, metric: t('5% Low (P5)'), value: item.p5 },
          {
            target: item.target,
            metric: t('90% Trimmed Mean / Median'),
            value: item.mid,
          },
          { target: item.target, metric: t('95% High (P95)'), value: item.p95 }
        )
      }

      return {
        type: 'bar',
        data: [{ id: 'tpsRank', values: rows }],
        xField: ['target', 'metric'],
        yField: 'value',
        seriesField: 'metric',
        color: ['#f59e0b', '#6366f1', '#10b981'],
        bar: {
          style: {
            cornerRadius: [chartRadius, chartRadius, 0, 0],
          },
        },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true } },
          {
            orient: 'left',
            min: 0,
            label: { formatMethod: (val: number) => `${val} t/s` },
          },
        ],
      }
    }

    if (activeChartTab === 'cache-ranking') {
      const items =
        props.entityViewMode === 'channels'
          ? props.channels
              .filter((item) => item.total_input_tokens > 0)
              .slice(0, 10)
              .map((item) => ({
                target: `#${item.channel_id} ${truncateLabel(item.channel_name, 16)}`,
                p5: hitOnly
                  ? item.hit_cache_rate_p5_low
                  : item.cache_rate_p5_low,
                p50: hitOnly
                  ? item.hit_cache_rate_median
                  : item.cache_rate_median,
                p95: hitOnly
                  ? item.hit_cache_rate_p95_high
                  : item.cache_rate_p95_high,
                weighted: item.weighted_cache_rate,
              }))
          : props.models
              .filter((item) => item.total_input_tokens > 0)
              .slice(0, 10)
              .map((item) => ({
                target: truncateLabel(item.model, 22),
                p5: hitOnly
                  ? item.hit_cache_rate_p5_low
                  : item.cache_rate_p5_low,
                p50: hitOnly
                  ? item.hit_cache_rate_median
                  : item.cache_rate_median,
                p95: hitOnly
                  ? item.hit_cache_rate_p95_high
                  : item.cache_rate_p95_high,
                weighted: item.weighted_cache_rate,
              }))

      if (items.length === 0) {
        return null
      }

      const rows: Array<{ target: string; metric: string; value: number }> = []
      for (const item of items) {
        rows.push(
          { target: item.target, metric: t('5% Low (P5)'), value: item.p5 },
          { target: item.target, metric: t('Median (P50)'), value: item.p50 },
          { target: item.target, metric: t('95% High (P95)'), value: item.p95 },
          {
            target: item.target,
            metric: t('Weighted Avg Cache Rate'),
            value: item.weighted,
          }
        )
      }

      return {
        type: 'bar',
        data: [{ id: 'cacheRank', values: rows }],
        xField: ['target', 'metric'],
        yField: 'value',
        seriesField: 'metric',
        color: ['#f59e0b', '#6366f1', '#10b981', '#0ea5e9'],
        bar: {
          style: {
            cornerRadius: [chartRadius, chartRadius, 0, 0],
          },
        },
        legends: { visible: true, orient: 'top' },
        axes: [
          { orient: 'bottom', label: { autoHide: true } },
          {
            orient: 'left',
            min: 0,
            max: 100,
            label: { formatMethod: (val: number) => `${val}%` },
          },
        ],
      }
    }

    return null
  }, [
    activeChartTab,
    chartRadius,
    hitOnly,
    props.channelTrends,
    props.channels,
    props.entityViewMode,
    props.granularity,
    props.models,
    props.trend,
    t,
  ])

  const chartKey = [
    props.mode,
    activeChartTab,
    props.entityViewMode,
    props.cacheScope ?? 'all',
    props.trend.length,
    resolvedTheme,
    customization.preset,
  ].join(':')

  return (
    <Card className='overflow-hidden'>
      <div className='flex flex-col gap-2 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex items-center gap-2'>
          <IconBadge tone='chart-2' size='sm'>
            <BarChart3 />
          </IconBadge>
          <span className='text-sm font-semibold'>
            {t('Telemetry Trend & Distribution Charts')}
          </span>
        </div>

        <div className='flex flex-wrap items-center gap-2'>
          <div className='bg-muted/60 inline-flex max-w-full overflow-x-auto rounded-lg border p-0.5'>
            {tabs.map((tab) => (
              <button
                key={tab.id}
                type='button'
                onClick={() => setSelectedTab(tab.id)}
                className={`shrink-0 rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  activeChartTab === tab.id
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                {t(tab.labelKey)}
              </button>
            ))}
          </div>

          <Button
            variant='ghost'
            size='sm'
            className='h-7 gap-1 px-2 text-xs'
            onClick={() => setCollapsed((prev) => !prev)}
          >
            {collapsed ? (
              <ChevronDown aria-hidden='true' className='size-3.5' />
            ) : (
              <ChevronUp aria-hidden='true' className='size-3.5' />
            )}
            <span>{collapsed ? t('Expand Charts') : t('Collapse Charts')}</span>
          </Button>
        </div>
      </div>

      {!collapsed && (
        <div className='h-72 p-2 sm:h-80 sm:p-3'>
          {themeReady && chartPayload ? (
            <VChart
              key={chartKey}
              spec={{
                ...chartPayload,
                background: 'transparent',
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
              }}
              options={VCHART_OPTION}
            />
          ) : (
            <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
              {props.isLoading
                ? t('Loading chart telemetry...')
                : t('No telemetry samples in the selected time range')}
            </div>
          )}
        </div>
      )}
    </Card>
  )
}
