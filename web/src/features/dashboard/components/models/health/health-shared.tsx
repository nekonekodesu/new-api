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
import { RefreshCw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { TimeRangeKey } from '@/lib/time'
import { cn } from '@/lib/utils'

import { formatPercentValue, formatTpsValue } from './health-utils'

export function TpsDistributionBar(props: {
  p5Low: number
  trimmed90: number
  p95High: number
  maxScale: number
}) {
  const { t } = useTranslation()
  if (props.trimmed90 <= 0 && props.p95High <= 0) {
    return <span className='text-muted-foreground font-mono text-xs'>-</span>
  }
  const denom = Math.max(props.maxScale, props.p95High, 100)
  const lowPct = Math.min(Math.max((props.p5Low / denom) * 100, 2), 92)
  const midPct = Math.min(
    Math.max((props.trimmed90 / denom) * 100, lowPct + 2),
    96
  )
  const highPct = Math.min(
    Math.max((props.p95High / denom) * 100, midPct + 2),
    100
  )

  return (
    <div className='flex min-w-44 flex-col gap-1'>
      <div className='flex items-center justify-between gap-2 font-mono text-[11px]'>
        <span
          className='text-amber-600 dark:text-amber-400'
          title={t('5% Low (P5)')}
        >
          {formatTpsValue(props.p5Low)}
        </span>
        <span
          className='text-foreground font-bold'
          title={t('90% Trimmed Mean / Median')}
        >
          {formatTpsValue(props.trimmed90)} t/s
        </span>
        <span
          className='text-emerald-600 dark:text-emerald-400'
          title={t('5% High (P95)')}
        >
          {formatTpsValue(props.p95High)}
        </span>
      </div>
      <div className='bg-muted relative h-1.5 w-full overflow-hidden rounded-full'>
        <div
          className='bg-primary/35 absolute top-0 bottom-0 rounded-full'
          style={{
            left: `${lowPct}%`,
            width: `${Math.max(highPct - lowPct, 4)}%`,
          }}
        />
        <div
          className='bg-primary absolute top-0 bottom-0 w-1.5 -translate-x-1/2 rounded-full'
          style={{ left: `${midPct}%` }}
        />
      </div>
    </div>
  )
}

export function CacheRateDistributionBar(props: {
  p5Low: number
  median: number
  p95High: number
  hasSamples: boolean
}) {
  const { t } = useTranslation()
  if (!props.hasSamples) {
    return <span className='text-muted-foreground font-mono text-xs'>-</span>
  }
  const lowPct = Math.min(Math.max(props.p5Low, 0), 96)
  const midPct = Math.min(Math.max(props.median, lowPct), 98)
  const highPct = Math.min(Math.max(props.p95High, midPct), 100)

  return (
    <div className='flex min-w-44 flex-col gap-1'>
      <div className='flex items-center justify-between gap-2 font-mono text-[11px]'>
        <span
          className='text-amber-600 dark:text-amber-400'
          title={t('5% Low (P5)')}
        >
          {formatPercentValue(props.p5Low, 1)}
        </span>
        <span className='text-foreground font-bold' title={t('Median (P50)')}>
          {formatPercentValue(props.median, 1)}
        </span>
        <span
          className='text-emerald-600 dark:text-emerald-400'
          title={t('95% High (P95)')}
        >
          {formatPercentValue(props.p95High, 1)}
        </span>
      </div>
      <div className='bg-muted relative h-1.5 w-full overflow-hidden rounded-full'>
        <div
          className='absolute top-0 bottom-0 rounded-full bg-sky-500/35'
          style={{
            left: `${lowPct}%`,
            width: `${Math.max(highPct - lowPct, 3)}%`,
          }}
        />
        <div
          className='absolute top-0 bottom-0 w-1.5 -translate-x-1/2 rounded-full bg-sky-500'
          style={{ left: `${midPct}%` }}
        />
      </div>
    </div>
  )
}

export type HealthToolbarControlsProps = {
  keyword: string
  onKeywordChange: (next: string) => void
  timeRange: TimeRangeKey
  onTimeRangeChange: (next: TimeRangeKey) => void
  onlyActiveCalls: boolean
  onToggleOnlyActiveCalls: () => void
  isFetching: boolean
  onRefresh: () => void
  extraSlot?: React.ReactNode
}

export function HealthToolbarControls(props: HealthToolbarControlsProps) {
  const { t } = useTranslation()

  return (
    <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
      {props.extraSlot}
      <div className='relative w-full sm:w-56'>
        <Search
          aria-hidden='true'
          className='text-muted-foreground absolute start-2.5 top-2.5 size-3.5'
        />
        <Input
          value={props.keyword}
          onChange={(e) => props.onKeywordChange(e.target.value)}
          placeholder={t('Filter models or channels...')}
          aria-label={t('Filter models or channels...')}
          className='h-8 ps-8 text-xs'
        />
      </div>
      <Select
        value={props.timeRange}
        onValueChange={(val) => props.onTimeRangeChange(val as TimeRangeKey)}
      >
        <SelectTrigger className='h-8 w-full text-xs sm:w-36'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='1d'>{t('Last 24 Hours')}</SelectItem>
          <SelectItem value='7d'>{t('Last 7 Days')}</SelectItem>
          <SelectItem value='30d'>{t('Last 30 Days')}</SelectItem>
          <SelectItem value='90d'>{t('Last 90 Days')}</SelectItem>
          <SelectItem value='all'>{t('All Time')}</SelectItem>
        </SelectContent>
      </Select>
      <Button
        variant={props.onlyActiveCalls ? 'default' : 'outline'}
        size='sm'
        className='h-8 flex-1 text-xs sm:flex-initial'
        onClick={props.onToggleOnlyActiveCalls}
      >
        {t('Only With Calls')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        className='h-8 flex-1 text-xs sm:flex-initial'
        onClick={props.onRefresh}
        disabled={props.isFetching}
      >
        <RefreshCw
          aria-hidden='true'
          className={cn('me-1.5 size-3.5', props.isFetching && 'animate-spin')}
        />
        {t('Refresh')}
      </Button>
    </div>
  )
}
