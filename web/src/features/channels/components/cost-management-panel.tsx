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
import {
  ChevronDown,
  ChevronRight,
  Coins,
  Cpu,
  DollarSign,
  Layers,
  Percent,
  RefreshCw,
  Search,
  Server,
  TrendingUp,
  Users,
  Wallet,
} from 'lucide-react'
import { Fragment, useMemo, useState } from 'react'
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
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
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
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  getModelCostAnalysis,
  getUserCostAnalysis,
  type ModelCostAnalysisRow,
  type UserCostAnalysisRow,
} from '../api'

type TimeRangeKey = 'all' | '1d' | '7d' | '30d' | '90d'

function computeStartTimestamp(range: TimeRangeKey): number | undefined {
  const nowSec = Math.floor(Date.now() / 1000)
  switch (range) {
    case '1d':
      return nowSec - 86400
    case '7d':
      return nowSec - 7 * 86400
    case '30d':
      return nowSec - 30 * 86400
    case '90d':
      return nowSec - 90 * 86400
    case 'all':
    default:
      return undefined
  }
}

function formatUSD(val: number, locale?: string): string {
  if (!Number.isFinite(val)) {
    return formatBillingCurrencyFromUSD(0, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
      locale,
    })
  }
  return formatBillingCurrencyFromUSD(val, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
    locale,
  })
}

function formatUnitUSD(val: number, locale?: string): string {
  if (!Number.isFinite(val) || val <= 0) {
    return formatBillingCurrencyFromUSD(0, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
      locale,
    })
  }
  return formatBillingCurrencyFromUSD(val, {
    digitsLarge: 4,
    digitsSmall: 4,
    abbreviate: false,
    locale,
  })
}

function getUpstreamCurrencySymbol(curr: string | undefined): string {
  const c = (curr || '').toUpperCase().trim()
  if (c === 'CNY') return '¥'
  if (c === 'TWD' || c === 'NTD') return 'NT$'
  return '$'
}

function formatRawUpstreamAmount(
  val: number | undefined,
  curr: string | undefined
): string {
  if (typeof val !== 'number' || !Number.isFinite(val) || val <= 0) {
    return ''
  }
  const sym = getUpstreamCurrencySymbol(curr)
  return `${sym}${Number(val.toFixed(4))}`
}

function getMarginBadgeVariant(
  marginPct: number
): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (marginPct < 0) {
    return 'destructive'
  }
  if (marginPct >= 25) {
    return 'default'
  }
  return 'secondary'
}

export interface CostManagementPanelProps {
  mode: 'models' | 'users'
}

export function CostManagementPanel(props: CostManagementPanelProps) {
  const { t, i18n } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // Subscribe to currency config so switching display currency re-renders immediately
  useSystemConfigStore((state) => state.config.currency)

  const [timeRange, setTimeRange] = useState<TimeRangeKey>('30d')
  const [keyword, setKeyword] = useState('')
  const [onlyActiveCalls, setOnlyActiveCalls] = useState(false)
  const [expandedModels, setExpandedModels] = useState<Record<string, boolean>>(
    {}
  )
  const [expandedUsers, setExpandedUsers] = useState<Record<number, boolean>>(
    {}
  )

  const startTimestamp = useMemo(
    () => computeStartTimestamp(timeRange),
    [timeRange]
  )

  const modelsCostQuery = useQuery({
    queryKey: ['cost-management-models', timeRange],
    queryFn: async () =>
      requireServerSuccess(
        await getModelCostAnalysis({
          start_timestamp: startTimestamp,
        })
      ),
    enabled: props.mode === 'models',
  })

  const usersCostQuery = useQuery({
    queryKey: ['cost-management-users', timeRange],
    queryFn: async () =>
      requireServerSuccess(
        await getUserCostAnalysis({
          start_timestamp: startTimestamp,
        })
      ),
    enabled: props.mode === 'users',
  })

  const toggleModelExpand = (modelName: string) => {
    setExpandedModels((prev) => ({
      ...prev,
      [modelName]: !prev[modelName],
    }))
  }

  const toggleUserExpand = (userId: number) => {
    setExpandedUsers((prev) => ({
      ...prev,
      [userId]: !prev[userId],
    }))
  }

  if (props.mode === 'models') {
    const isLoading = modelsCostQuery.isLoading
    const rawModels: ModelCostAnalysisRow[] =
      modelsCostQuery.data?.data?.models ?? []
    const summary = modelsCostQuery.data?.data?.summary

    const kw = keyword.trim().toLowerCase()
    const filteredModels = rawModels.filter((row) => {
      if (onlyActiveCalls && row.call_count <= 0) {
        return false
      }
      if (!kw) {
        return true
      }
      return row.model.toLowerCase().includes(kw)
    })

    return (
      <div className='space-y-4'>
        {/* Summary Cards */}
        <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          <Card>
            <CardHeader className='pb-2'>
              <CardDescription className='flex items-center justify-between text-xs'>
                <span>{t('Models & Total Calls')}</span>
                <Cpu
                  aria-hidden='true'
                  className='text-muted-foreground size-4'
                />
              </CardDescription>
              <CardTitle className='text-xl font-bold'>
                {formatNumber(summary?.total_calls ?? 0, intlLocale)}{' '}
                <span className='text-muted-foreground text-xs font-normal'>
                  ({summary?.model_count ?? 0} {t('Models')})
                </span>
              </CardTitle>
            </CardHeader>
          </Card>

          <Card>
            <CardHeader className='pb-2'>
              <CardDescription className='flex items-center justify-between text-xs'>
                <span>{t('Total Token Usage')}</span>
                <Layers
                  aria-hidden='true'
                  className='text-muted-foreground size-4'
                />
              </CardDescription>
              <CardTitle className='text-xl font-bold'>
                {formatNumber(summary?.total_tokens ?? 0, intlLocale)}
              </CardTitle>
            </CardHeader>
          </Card>

          <Card>
            <CardHeader className='pb-2'>
              <CardDescription className='flex items-center justify-between text-xs'>
                <span>{t('Total Revenue (Consumed)')}</span>
                <DollarSign
                  aria-hidden='true'
                  className='size-4 text-emerald-500'
                />
              </CardDescription>
              <CardTitle className='text-xl font-bold text-emerald-600 dark:text-emerald-400'>
                {formatUSD(summary?.total_revenue ?? 0)}
              </CardTitle>
            </CardHeader>
          </Card>

          <Card>
            <CardHeader className='pb-2'>
              <CardDescription className='flex items-center justify-between text-xs'>
                <span>{t('Upstream Cost & Gross Margin')}</span>
                <TrendingUp
                  aria-hidden='true'
                  className='text-primary size-4'
                />
              </CardDescription>
              <CardTitle className='flex items-baseline gap-2 text-xl font-bold'>
                <span>{formatUSD(summary?.total_cost ?? 0)}</span>
                <Badge
                  variant={getMarginBadgeVariant(summary?.margin_pct ?? 0)}
                  className='text-xs'
                >
                  {t('Margin')}: {(summary?.margin_pct ?? 0).toFixed(1)}%
                </Badge>
              </CardTitle>
            </CardHeader>
          </Card>
        </div>

        {/* Filter Bar & Table */}
        <Card>
          <CardHeader className='pb-3'>
            <div className='flex flex-wrap items-center justify-between gap-3'>
              <div>
                <CardTitle className='text-base'>
                  {t('Model Selling Price, Usage & Upstream Cost Breakdown')}
                </CardTitle>
                <CardDescription className='mt-0.5 text-xs'>
                  {t(
                    'Click any model row to expand selling prices across user groups and call counts/costs across upstream channels'
                  )}
                </CardDescription>
              </div>
              <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
                <div className='relative w-full sm:w-56'>
                  <Search
                    aria-hidden='true'
                    className='text-muted-foreground absolute start-2.5 top-2.5 size-3.5'
                  />
                  <Input
                    value={keyword}
                    onChange={(e) => setKeyword(e.target.value)}
                    placeholder={t('Filter models...')}
                    className='h-8 ps-8 text-xs'
                  />
                </div>
                <Select
                  value={timeRange}
                  onValueChange={(val) => setTimeRange(val as TimeRangeKey)}
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
                  variant={onlyActiveCalls ? 'default' : 'outline'}
                  size='sm'
                  className='h-8 flex-1 text-xs sm:flex-initial'
                  onClick={() => setOnlyActiveCalls((prev) => !prev)}
                >
                  {t('Only With Calls')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  className='h-8 flex-1 text-xs sm:flex-initial'
                  onClick={() => void modelsCostQuery.refetch()}
                  disabled={modelsCostQuery.isFetching}
                >
                  <RefreshCw
                    aria-hidden='true'
                    className={`me-1.5 size-3.5 ${modelsCostQuery.isFetching ? 'animate-spin' : ''}`}
                  />
                  {t('Refresh')}
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className='p-0'>
            {isLoading && (
              <div className='space-y-2 p-4'>
                <Skeleton className='h-12 w-full' />
                <Skeleton className='h-12 w-full' />
                <Skeleton className='h-12 w-full' />
              </div>
            )}
            {!isLoading && isMobile && (
              <div className='flex flex-col gap-3 p-3'>
                {filteredModels.length === 0 ? (
                  <div className='text-muted-foreground py-8 text-center text-sm'>
                    {t('No matching models found')}
                  </div>
                ) : (
                  filteredModels.map((row) => {
                    const isExpanded = Boolean(expandedModels[row.model])
                    const unitSuffix =
                      row.quota_type === 1 ? `/${t('call')}` : '/1M'
                    const costRangeIn =
                      row.min_input_cost === row.max_input_cost
                        ? `${formatUnitUSD(row.min_input_cost)}`
                        : `${formatUnitUSD(row.min_input_cost)}~${formatUnitUSD(row.max_input_cost)}`
                    const costRangeOut =
                      row.min_output_cost === row.max_output_cost
                        ? `${formatUnitUSD(row.min_output_cost)}`
                        : `${formatUnitUSD(row.min_output_cost)}~${formatUnitUSD(row.max_output_cost)}`

                    return (
                      <div
                        key={row.model}
                        className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
                      >
                        <button
                          type='button'
                          aria-expanded={isExpanded}
                          onClick={() => toggleModelExpand(row.model)}
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
                            <Badge
                              variant='outline'
                              className='shrink-0 text-[10px]'
                            >
                              {row.quota_type === 1
                                ? t('Per Call')
                                : t('Per Token')}
                            </Badge>
                          </div>
                          <div className='flex shrink-0 items-center gap-1 font-mono text-xs'>
                            <span className='font-semibold'>
                              {formatUSD(row.profit_usd)}
                            </span>
                            <Badge
                              variant={getMarginBadgeVariant(row.margin_pct)}
                              className='text-[10px]'
                            >
                              {row.margin_pct.toFixed(1)}%
                            </Badge>
                          </div>
                        </button>

                        <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 text-xs'>
                          <div className='flex flex-col gap-0.5'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('1x Selling Price (Input / Output)')}
                            </span>
                            <span className='font-mono text-[11px]'>
                              {row.quota_type === 1
                                ? `${formatUnitUSD(row.base_input_price)}${unitSuffix}`
                                : `${formatUnitUSD(row.base_input_price)} / ${formatUnitUSD(row.base_output_price)} (/1M)`}
                            </span>
                          </div>
                          <div className='flex flex-col gap-0.5'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Upstream Cost (Input / Output)')}
                            </span>
                            <span className='font-mono text-[11px]'>
                              {row.quota_type === 1
                                ? `${costRangeIn}${unitSuffix}`
                                : `${costRangeIn} / ${costRangeOut} (/1M)`}
                            </span>
                          </div>
                          <div className='flex flex-col gap-0.5'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Call Count')} / {t('Token Usage (In / Out)')}
                            </span>
                            <span className='font-mono text-[11px]'>
                              {formatNumber(row.call_count, intlLocale)}{' '}
                              {t('calls')} ·{' '}
                              {formatNumber(row.total_tokens, intlLocale)}
                            </span>
                          </div>
                          <div className='flex flex-col gap-0.5'>
                            <span className='text-muted-foreground text-[10px]'>
                              {t('Revenue')} / {t('Upstream Cost')}
                            </span>
                            <span className='font-mono text-[11px]'>
                              <span className='text-emerald-600 dark:text-emerald-400'>
                                {formatUSD(row.revenue_usd)}
                              </span>{' '}
                              / <span>{formatUSD(row.cost_usd)}</span>
                            </span>
                          </div>
                        </div>

                        {isExpanded ? (
                          <div className='border-border/50 flex flex-col gap-3 border-t pt-2.5'>
                            <div className='flex flex-col gap-1.5'>
                              <div className='flex items-center gap-1.5 text-xs font-semibold'>
                                <Layers
                                  aria-hidden='true'
                                  className='text-primary size-3.5'
                                />
                                <span>
                                  {t('Selling Prices & Usage by User Group')} (
                                  {row.group_prices.length})
                                </span>
                              </div>
                              <div className='flex flex-col gap-1.5'>
                                {row.group_prices.map((gp) => (
                                  <div
                                    key={gp.group}
                                    className='bg-muted/30 flex flex-wrap items-center justify-between gap-2 rounded p-2 text-xs'
                                  >
                                    <div className='flex items-center gap-1.5 font-mono'>
                                      <span className='font-semibold'>
                                        {gp.group}
                                      </span>
                                      <Badge
                                        variant='outline'
                                        className='text-[10px]'
                                      >
                                        {gp.group_ratio}x
                                      </Badge>
                                      <span className='text-muted-foreground text-[11px]'>
                                        {gp.quota_type === 1
                                          ? `${formatUnitUSD(gp.input_price)}/${t('call')}`
                                          : `${formatUnitUSD(gp.input_price)} / ${formatUnitUSD(gp.output_price)}`}
                                      </span>
                                    </div>
                                    <div className='flex items-center gap-2 font-mono text-[11px]'>
                                      <span>
                                        {formatNumber(
                                          gp.call_count,
                                          intlLocale
                                        )}{' '}
                                        {t('calls')}
                                      </span>
                                      <span>
                                        <span className='text-emerald-600 dark:text-emerald-400'>
                                          {formatUSD(gp.revenue_usd)}
                                        </span>{' '}
                                        / {formatUSD(gp.cost_usd)}
                                      </span>
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>

                            <div className='flex flex-col gap-1.5'>
                              <div className='flex items-center gap-1.5 text-xs font-semibold'>
                                <Server
                                  aria-hidden='true'
                                  className='text-primary size-3.5'
                                />
                                <span>
                                  {t('Call Counts & Costs by Upstream Channel')}{' '}
                                  ({row.channel_usages.length})
                                </span>
                              </div>
                              <div className='flex flex-col gap-1.5'>
                                {row.channel_usages.map((cu) => (
                                  <div
                                    key={cu.channel_id}
                                    className='bg-muted/30 flex flex-col gap-1 rounded p-2 text-xs'
                                  >
                                    <div className='flex items-center justify-between gap-2'>
                                      <span className='truncate font-medium'>
                                        #{cu.channel_id} {cu.channel_name}
                                      </span>
                                      <Badge
                                        variant={getMarginBadgeVariant(
                                          cu.margin_pct
                                        )}
                                        className='font-mono text-[10px]'
                                      >
                                        {cu.margin_pct.toFixed(1)}%
                                      </Badge>
                                    </div>
                                    <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 font-mono text-[11px]'>
                                      <span>
                                        {cu.quota_type === 1
                                          ? `${formatUnitUSD(cu.input_cost)}/${t('call')}`
                                          : `${formatUnitUSD(cu.input_cost)} / ${formatUnitUSD(cu.output_cost)}`}
                                        {cu.currency &&
                                        cu.raw_input_cost &&
                                        cu.raw_input_cost > 0 ? (
                                          <span className='ms-1 text-[10px] opacity-75'>
                                            (
                                            {cu.quota_type === 1
                                              ? formatRawUpstreamAmount(
                                                  cu.raw_input_cost,
                                                  cu.currency
                                                )
                                              : `${formatRawUpstreamAmount(cu.raw_input_cost, cu.currency)}/${formatRawUpstreamAmount(cu.raw_output_cost, cu.currency)}`}{' '}
                                            {cu.currency} ÷ {cu.exchange_rate})
                                          </span>
                                        ) : null}
                                      </span>
                                      <span>
                                        {formatNumber(
                                          cu.call_count,
                                          intlLocale
                                        )}{' '}
                                        {t('calls')} ·{' '}
                                        <span className='text-emerald-600 dark:text-emerald-400'>
                                          {formatUSD(cu.revenue_usd)}
                                        </span>{' '}
                                        / {formatUSD(cu.cost_usd)}
                                      </span>
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>
                          </div>
                        ) : null}
                      </div>
                    )
                  })
                )}
              </div>
            )}
            {!isLoading && !isMobile && (
              <div className='overflow-x-auto'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-8' />
                      <TableHead>{t('Model')}</TableHead>
                      <TableHead>
                        {t('1x Selling Price (Input / Output)')}
                      </TableHead>
                      <TableHead>
                        {t('Upstream Cost (Input / Output)')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Call Count')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Token Usage (In / Out)')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Revenue')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Upstream Cost')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Gross Profit & Margin')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredModels.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={9}
                          className='text-muted-foreground h-24 text-center text-sm'
                        >
                          {t('No matching models found')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      filteredModels.map((row) => {
                        const isExpanded = Boolean(expandedModels[row.model])
                        const unitSuffix =
                          row.quota_type === 1 ? `/${t('call')}` : '/1M'
                        const costRangeIn =
                          row.min_input_cost === row.max_input_cost
                            ? `${formatUnitUSD(row.min_input_cost)}`
                            : `${formatUnitUSD(row.min_input_cost)}~${formatUnitUSD(row.max_input_cost)}`
                        const costRangeOut =
                          row.min_output_cost === row.max_output_cost
                            ? `${formatUnitUSD(row.min_output_cost)}`
                            : `${formatUnitUSD(row.min_output_cost)}~${formatUnitUSD(row.max_output_cost)}`

                        return (
                          <Fragment key={row.model}>
                            <TableRow
                              tabIndex={0}
                              aria-expanded={isExpanded}
                              className='hover:bg-muted/50 cursor-pointer'
                              onClick={() => toggleModelExpand(row.model)}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                  e.preventDefault()
                                  toggleModelExpand(row.model)
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
                                    className='text-[10px]'
                                  >
                                    {row.quota_type === 1
                                      ? t('Per Call')
                                      : t('Per Token')}
                                  </Badge>
                                </div>
                              </TableCell>
                              <TableCell className='font-mono text-xs'>
                                {row.quota_type === 1 ? (
                                  <span>
                                    {formatUnitUSD(row.base_input_price)}
                                    {unitSuffix}
                                  </span>
                                ) : (
                                  <span>
                                    {formatUnitUSD(row.base_input_price)} /{' '}
                                    {formatUnitUSD(row.base_output_price)}{' '}
                                    <span className='text-muted-foreground text-[10px]'>
                                      (/1M)
                                    </span>
                                  </span>
                                )}
                              </TableCell>
                              <TableCell className='font-mono text-xs'>
                                {row.quota_type === 1 ? (
                                  <span>
                                    {costRangeIn}
                                    {unitSuffix}
                                  </span>
                                ) : (
                                  <span>
                                    {costRangeIn} / {costRangeOut}{' '}
                                    <span className='text-muted-foreground text-[10px]'>
                                      (/1M)
                                    </span>
                                  </span>
                                )}
                              </TableCell>
                              <TableCell className='text-right font-mono text-xs font-medium'>
                                {formatNumber(row.call_count, intlLocale)}
                              </TableCell>
                              <TableCell className='text-right font-mono text-xs'>
                                <div className='flex flex-col items-end'>
                                  <span>
                                    {formatNumber(row.total_tokens, intlLocale)}
                                  </span>
                                  <span className='text-muted-foreground text-[10px]'>
                                    {formatNumber(
                                      row.prompt_tokens,
                                      intlLocale
                                    )}{' '}
                                    /{' '}
                                    {formatNumber(
                                      row.completion_tokens,
                                      intlLocale
                                    )}
                                  </span>
                                </div>
                              </TableCell>
                              <TableCell className='text-right font-mono text-xs font-medium text-emerald-600 dark:text-emerald-400'>
                                {formatUSD(row.revenue_usd)}
                              </TableCell>
                              <TableCell className='text-right font-mono text-xs font-medium'>
                                {formatUSD(row.cost_usd)}
                              </TableCell>
                              <TableCell className='text-right'>
                                <div className='flex items-center justify-end gap-1.5 font-mono text-xs'>
                                  <span>{formatUSD(row.profit_usd)}</span>
                                  <Badge
                                    variant={getMarginBadgeVariant(
                                      row.margin_pct
                                    )}
                                    className='text-[10px]'
                                  >
                                    {row.margin_pct.toFixed(1)}%
                                  </Badge>
                                </div>
                              </TableCell>
                            </TableRow>

                            {isExpanded ? (
                              <TableRow className='bg-muted/20'>
                                <TableCell colSpan={9} className='p-4'>
                                  <div className='grid gap-4 lg:grid-cols-2'>
                                    {/* Group Selling Prices Breakdown */}
                                    <div className='bg-background rounded-lg border p-3'>
                                      <div className='mb-2 flex items-center gap-1.5 text-xs font-semibold'>
                                        <Layers
                                          aria-hidden='true'
                                          className='text-primary size-3.5'
                                        />
                                        <span>
                                          {t(
                                            'Selling Prices & Usage by User Group'
                                          )}{' '}
                                          ({row.group_prices.length})
                                        </span>
                                      </div>
                                      <Table>
                                        <TableHeader>
                                          <TableRow>
                                            <TableHead className='h-8 text-[11px]'>
                                              {t('Group')}
                                            </TableHead>
                                            <TableHead className='h-8 text-[11px]'>
                                              {t('Ratio')}
                                            </TableHead>
                                            <TableHead className='h-8 text-[11px]'>
                                              {t('Group Price (In / Out)')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Calls')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Tokens')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Revenue / Cost')}
                                            </TableHead>
                                          </TableRow>
                                        </TableHeader>
                                        <TableBody>
                                          {row.group_prices.map((gp) => (
                                            <TableRow key={gp.group}>
                                              <TableCell className='py-1.5 font-mono text-xs font-medium'>
                                                {gp.group}
                                              </TableCell>
                                              <TableCell className='py-1.5 font-mono text-xs'>
                                                {gp.group_ratio}x
                                              </TableCell>
                                              <TableCell className='py-1.5 font-mono text-xs'>
                                                {gp.quota_type === 1
                                                  ? `${formatUnitUSD(gp.input_price)}/${t('call')}`
                                                  : `${formatUnitUSD(gp.input_price)} / ${formatUnitUSD(gp.output_price)}`}
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                {formatNumber(
                                                  gp.call_count,
                                                  intlLocale
                                                )}
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                {formatNumber(
                                                  gp.total_tokens,
                                                  intlLocale
                                                )}
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                <span className='text-emerald-600 dark:text-emerald-400'>
                                                  {formatUSD(gp.revenue_usd)}
                                                </span>{' '}
                                                /{' '}
                                                <span>
                                                  {formatUSD(gp.cost_usd)}
                                                </span>
                                              </TableCell>
                                            </TableRow>
                                          ))}
                                        </TableBody>
                                      </Table>
                                    </div>

                                    {/* Upstream Channels Usage & Cost Breakdown */}
                                    <div className='bg-background rounded-lg border p-3'>
                                      <div className='mb-2 flex items-center gap-1.5 text-xs font-semibold'>
                                        <Server
                                          aria-hidden='true'
                                          className='text-primary size-3.5'
                                        />
                                        <span>
                                          {t(
                                            'Call Counts & Costs by Upstream Channel'
                                          )}{' '}
                                          ({row.channel_usages.length})
                                        </span>
                                      </div>
                                      <Table>
                                        <TableHeader>
                                          <TableRow>
                                            <TableHead className='h-8 text-[11px]'>
                                              {t('Channel')}
                                            </TableHead>
                                            <TableHead className='h-8 text-[11px]'>
                                              {t('Upstream Unit Cost')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Calls')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Tokens')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Revenue / Cost')}
                                            </TableHead>
                                            <TableHead className='h-8 text-right text-[11px]'>
                                              {t('Margin')}
                                            </TableHead>
                                          </TableRow>
                                        </TableHeader>
                                        <TableBody>
                                          {row.channel_usages.map((cu) => (
                                            <TableRow key={cu.channel_id}>
                                              <TableCell className='py-1.5 text-xs'>
                                                <div className='flex flex-col'>
                                                  <span className='font-medium'>
                                                    #{cu.channel_id}{' '}
                                                    {cu.channel_name}
                                                  </span>
                                                  {cu.upstream_group && (
                                                    <span className='text-muted-foreground font-mono text-[10px]'>
                                                      {cu.upstream_group} (
                                                      {cu.base_multiplier}x)
                                                    </span>
                                                  )}
                                                  {!cu.upstream_group &&
                                                    cu.base_multiplier &&
                                                    cu.base_multiplier !== 1 && (
                                                      <span className='text-muted-foreground font-mono text-[10px]'>
                                                        {cu.base_multiplier}x
                                                      </span>
                                                    )}
                                                </div>
                                              </TableCell>
                                              <TableCell className='py-1.5 font-mono text-xs'>
                                                <div className='flex flex-col'>
                                                  <span>
                                                    {cu.quota_type === 1
                                                      ? `${formatUnitUSD(cu.input_cost)}/${t('call')}`
                                                      : `${formatUnitUSD(cu.input_cost)} / ${formatUnitUSD(cu.output_cost)}`}
                                                  </span>
                                                  {(cu.from_upstream ||
                                                    cu.channel_type === 60) &&
                                                  cu.currency &&
                                                  cu.raw_input_cost &&
                                                  cu.raw_input_cost > 0 ? (
                                                    <span className='text-muted-foreground text-[10px]'>
                                                      {cu.quota_type === 1
                                                        ? formatRawUpstreamAmount(
                                                            cu.raw_input_cost,
                                                            cu.currency
                                                          )
                                                        : `${formatRawUpstreamAmount(cu.raw_input_cost, cu.currency)} / ${formatRawUpstreamAmount(cu.raw_output_cost, cu.currency)}`}{' '}
                                                      ({cu.currency} ÷{' '}
                                                      {cu.exchange_rate})
                                                    </span>
                                                  ) : null}
                                                </div>
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                {formatNumber(
                                                  cu.call_count,
                                                  intlLocale
                                                )}
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                {formatNumber(
                                                  cu.total_tokens,
                                                  intlLocale
                                                )}
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                <span className='text-emerald-600 dark:text-emerald-400'>
                                                  {formatUSD(cu.revenue_usd)}
                                                </span>{' '}
                                                /{' '}
                                                <span>
                                                  {formatUSD(cu.cost_usd)}
                                                </span>
                                              </TableCell>
                                              <TableCell className='py-1.5 text-right font-mono text-xs'>
                                                <Badge
                                                  variant={getMarginBadgeVariant(
                                                    cu.margin_pct
                                                  )}
                                                  className='text-[10px]'
                                                >
                                                  {cu.margin_pct.toFixed(1)}%
                                                </Badge>
                                              </TableCell>
                                            </TableRow>
                                          ))}
                                        </TableBody>
                                      </Table>
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
      </div>
    )
  }

  // ==================== User Cost & Gross Margin View ====================
  const isLoadingUsers = usersCostQuery.isLoading
  const rawUsers: UserCostAnalysisRow[] = usersCostQuery.data?.data?.users ?? []
  const userSummary = usersCostQuery.data?.data?.summary

  const uKw = keyword.trim().toLowerCase()
  const filteredUsers = rawUsers.filter((u) => {
    if (onlyActiveCalls && u.call_count <= 0 && u.consumed_usd <= 0) {
      return false
    }
    if (!uKw) {
      return true
    }
    return (
      u.username.toLowerCase().includes(uKw) ||
      u.display_name.toLowerCase().includes(uKw) ||
      String(u.user_id).includes(uKw)
    )
  })

  return (
    <div className='space-y-4'>
      {/* User Cost Summary Cards */}
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5'>
        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Total User Top-up')}</span>
              <Wallet
                aria-hidden='true'
                className='text-muted-foreground size-4'
              />
            </CardDescription>
            <CardTitle className='text-xl font-bold'>
              {formatUSD(userSummary?.total_topup_usd ?? 0)}
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Total Consumption (Revenue)')}</span>
              <Coins aria-hidden='true' className='size-4 text-emerald-500' />
            </CardDescription>
            <CardTitle className='text-xl font-bold text-emerald-600 dark:text-emerald-400'>
              {formatUSD(userSummary?.total_consumed_usd ?? 0)}
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Total Upstream Cost')}</span>
              <DollarSign
                aria-hidden='true'
                className='text-muted-foreground size-4'
              />
            </CardDescription>
            <CardTitle className='text-xl font-bold'>
              {formatUSD(userSummary?.total_cost_usd ?? 0)}
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Total Gross Profit')}</span>
              <TrendingUp aria-hidden='true' className='text-primary size-4' />
            </CardDescription>
            <CardTitle className='text-xl font-bold'>
              {formatUSD(userSummary?.total_profit_usd ?? 0)}
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className='pb-2'>
            <CardDescription className='flex items-center justify-between text-xs'>
              <span>{t('Overall Gross Margin')}</span>
              <Percent aria-hidden='true' className='text-primary size-4' />
            </CardDescription>
            <CardTitle className='flex items-center gap-2 text-xl font-bold'>
              <span>{(userSummary?.overall_margin_pct ?? 0).toFixed(2)}%</span>
              <Badge
                variant={getMarginBadgeVariant(
                  userSummary?.overall_margin_pct ?? 0
                )}
                className='text-xs'
              >
                {userSummary?.user_count ?? 0} {t('Users')}
              </Badge>
            </CardTitle>
          </CardHeader>
        </Card>
      </div>

      {/* User Cost Table */}
      <Card>
        <CardHeader className='pb-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div>
              <CardTitle className='flex items-center gap-2 text-base'>
                <Users aria-hidden='true' className='text-primary size-4' />
                <span>
                  {t(
                    'User Top-up, Consumption, Upstream Cost & Gross Margin Analysis'
                  )}
                </span>
              </CardTitle>
              <CardDescription className='mt-0.5 text-xs'>
                {t(
                  'Click any user row to expand per-model and per-channel consumption, cost, and profit breakdown'
                )}
              </CardDescription>
            </div>
            <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
              <div className='relative w-full sm:w-56'>
                <Search
                  aria-hidden='true'
                  className='text-muted-foreground absolute start-2.5 top-2.5 size-3.5'
                />
                <Input
                  value={keyword}
                  onChange={(e) => setKeyword(e.target.value)}
                  placeholder={t('Filter users...')}
                  className='h-8 ps-8 text-xs'
                />
              </div>
              <Select
                value={timeRange}
                onValueChange={(val) => setTimeRange(val as TimeRangeKey)}
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
                variant={onlyActiveCalls ? 'default' : 'outline'}
                size='sm'
                className='h-8 flex-1 text-xs sm:flex-initial'
                onClick={() => setOnlyActiveCalls((prev) => !prev)}
              >
                {t('Only Active Users')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                className='h-8 flex-1 text-xs sm:flex-initial'
                onClick={() => void usersCostQuery.refetch()}
                disabled={usersCostQuery.isFetching}
              >
                <RefreshCw
                  aria-hidden='true'
                  className={`me-1.5 size-3.5 ${usersCostQuery.isFetching ? 'animate-spin' : ''}`}
                />
                {t('Refresh')}
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className='p-0'>
          {isLoadingUsers && (
            <div className='space-y-2 p-4'>
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
            </div>
          )}
          {!isLoadingUsers && isMobile && (
            <div className='flex flex-col gap-3 p-3'>
              {filteredUsers.length === 0 ? (
                <div className='text-muted-foreground py-8 text-center text-sm'>
                  {t('No matching users found')}
                </div>
              ) : (
                filteredUsers.map((u) => {
                  const isExpanded = Boolean(expandedUsers[u.user_id])
                  return (
                    <div
                      key={u.user_id}
                      className='border-border/60 bg-background/60 flex flex-col gap-2.5 rounded-lg border p-3'
                    >
                      <button
                        type='button'
                        aria-expanded={isExpanded}
                        onClick={() => toggleUserExpand(u.user_id)}
                        className='flex w-full items-start justify-between gap-2 text-start'
                      >
                        <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
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
                          <span className='text-muted-foreground font-mono text-xs'>
                            #{u.user_id}
                          </span>
                          <span className='truncate text-xs font-semibold'>
                            {u.username}
                          </span>
                          <Badge
                            variant='outline'
                            className='font-mono text-[10px]'
                          >
                            {u.group}
                          </Badge>
                        </div>
                        <Badge
                          variant={getMarginBadgeVariant(u.gross_margin_pct)}
                          className='shrink-0 font-mono text-xs'
                        >
                          {u.gross_margin_pct.toFixed(1)}%
                        </Badge>
                      </button>

                      <div className='bg-muted/20 grid grid-cols-2 gap-2 rounded-md p-2 text-xs'>
                        <div className='flex flex-col gap-0.5'>
                          <span className='text-muted-foreground text-[10px]'>
                            {t('Top-up Amount')} / {t('Remaining Balance')}
                          </span>
                          <span className='font-mono text-[11px]'>
                            <span className='font-semibold'>
                              {formatUSD(u.topup_total_usd)}
                            </span>{' '}
                            / {formatUSD(u.remain_quota_usd)}
                          </span>
                        </div>
                        <div className='flex flex-col gap-0.5'>
                          <span className='text-muted-foreground text-[10px]'>
                            {t('Calls & Tokens')}
                          </span>
                          <span className='font-mono text-[11px]'>
                            {formatNumber(u.call_count, intlLocale)}{' '}
                            {t('calls')} ·{' '}
                            {formatNumber(u.total_tokens, intlLocale)}
                          </span>
                        </div>
                        <div className='flex flex-col gap-0.5'>
                          <span className='text-muted-foreground text-[10px]'>
                            {t('Consumed (Total Revenue)')} /{' '}
                            {t('Total Upstream Cost')}
                          </span>
                          <span className='font-mono text-[11px]'>
                            <span className='text-emerald-600 dark:text-emerald-400'>
                              {formatUSD(u.revenue_usd)}
                            </span>{' '}
                            / {formatUSD(u.cost_usd)}
                          </span>
                        </div>
                        <div className='flex flex-col gap-0.5'>
                          <span className='text-muted-foreground text-[10px]'>
                            {t('Gross Profit')}
                          </span>
                          <span className='font-mono text-[11px] font-semibold'>
                            {formatUSD(u.gross_profit_usd)}
                          </span>
                        </div>
                      </div>

                      {isExpanded ? (
                        <div className='border-border/50 flex flex-col gap-2 border-t pt-2.5'>
                          <div className='flex flex-wrap items-center justify-between gap-1 text-xs font-semibold'>
                            <span>
                              {t('Per-Model & Per-Channel Cost Breakdown for')}{' '}
                              {u.username} ({u.model_breakdowns.length})
                            </span>
                            <span className='text-muted-foreground font-mono text-[11px]'>
                              {t('Lifetime Consumed')}:{' '}
                              {formatUSD(u.lifetime_used_usd)}
                            </span>
                          </div>
                          {u.model_breakdowns.length === 0 ? (
                            <p className='text-muted-foreground py-3 text-center text-xs'>
                              {t(
                                'No model consumption logs in selected time range'
                              )}
                            </p>
                          ) : (
                            <div className='flex flex-col gap-1.5'>
                              {u.model_breakdowns.map((bd) => (
                                <div
                                  key={`${bd.model}-${bd.channel_id}`}
                                  className='bg-muted/30 flex flex-col gap-1 rounded p-2 text-xs'
                                >
                                  <div className='flex items-center justify-between gap-2'>
                                    <span className='truncate font-mono font-semibold'>
                                      {bd.model}
                                    </span>
                                    <div className='flex items-center gap-1 font-mono'>
                                      <span>{formatUSD(bd.profit_usd)}</span>
                                      <Badge
                                        variant={getMarginBadgeVariant(
                                          bd.margin_pct
                                        )}
                                        className='text-[10px]'
                                      >
                                        {bd.margin_pct.toFixed(1)}%
                                      </Badge>
                                    </div>
                                  </div>
                                  <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 text-[11px]'>
                                    <span className='truncate'>
                                      #{bd.channel_id} {bd.channel_name}
                                    </span>
                                    <span className='font-mono'>
                                      {formatNumber(bd.call_count, intlLocale)}{' '}
                                      {t('calls')} ·{' '}
                                      <span className='text-emerald-600 dark:text-emerald-400'>
                                        {formatUSD(bd.revenue_usd)}
                                      </span>{' '}
                                      / {formatUSD(bd.cost_usd)}
                                    </span>
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      ) : null}
                    </div>
                  )
                })
              )}
            </div>
          )}
          {!isLoadingUsers && !isMobile && (
            <div className='overflow-x-auto'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className='w-8' />
                    <TableHead>{t('User & Group')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Top-up Amount')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Remaining Balance')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Calls & Tokens')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Consumed (Total Revenue)')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Total Upstream Cost')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Gross Profit')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Gross Margin')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredUsers.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className='text-muted-foreground h-24 text-center text-sm'
                      >
                        {t('No matching users found')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredUsers.map((u) => {
                      const isExpanded = Boolean(expandedUsers[u.user_id])
                      return (
                        <Fragment key={u.user_id}>
                          <TableRow
                            tabIndex={0}
                            aria-expanded={isExpanded}
                            className='hover:bg-muted/50 cursor-pointer'
                            onClick={() => toggleUserExpand(u.user_id)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' || e.key === ' ') {
                                e.preventDefault()
                                toggleUserExpand(u.user_id)
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
                                  #{u.user_id}
                                </span>
                                <span className='font-medium'>
                                  {u.username}
                                </span>
                                {u.display_name &&
                                u.display_name !== u.username ? (
                                  <span className='text-muted-foreground text-xs'>
                                    ({u.display_name})
                                  </span>
                                ) : null}
                                <Badge
                                  variant='outline'
                                  className='font-mono text-[10px]'
                                >
                                  {u.group}
                                </Badge>
                              </div>
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs'>
                              <div className='flex flex-col items-end'>
                                <span className='font-semibold'>
                                  {formatUSD(u.topup_total_usd)}
                                </span>
                                {u.online_topup_usd > 0 ||
                                u.system_topup_usd > 0 ? (
                                  <span className='text-muted-foreground text-[10px]'>
                                    {t('Online')}:{' '}
                                    {formatUSD(u.online_topup_usd)} |{' '}
                                    {t('Redeem/Admin')}:{' '}
                                    {formatUSD(u.system_topup_usd)}
                                  </span>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs'>
                              {formatUSD(u.remain_quota_usd)}
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs'>
                              <div className='flex flex-col items-end'>
                                <span>
                                  {formatNumber(u.call_count, intlLocale)}{' '}
                                  {t('calls')}
                                </span>
                                <span className='text-muted-foreground text-[10px]'>
                                  {formatNumber(u.total_tokens, intlLocale)}{' '}
                                  tokens
                                </span>
                              </div>
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs font-medium text-emerald-600 dark:text-emerald-400'>
                              {formatUSD(u.revenue_usd)}
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs font-medium'>
                              {formatUSD(u.cost_usd)}
                            </TableCell>
                            <TableCell className='text-right font-mono text-xs font-medium'>
                              {formatUSD(u.gross_profit_usd)}
                            </TableCell>
                            <TableCell className='text-right'>
                              <Badge
                                variant={getMarginBadgeVariant(
                                  u.gross_margin_pct
                                )}
                                className='font-mono text-xs'
                              >
                                {u.gross_margin_pct.toFixed(1)}%
                              </Badge>
                            </TableCell>
                          </TableRow>

                          {isExpanded ? (
                            <TableRow className='bg-muted/20'>
                              <TableCell colSpan={9} className='p-4'>
                                <div className='bg-background rounded-lg border p-3'>
                                  <div className='mb-2 flex items-center justify-between text-xs font-semibold'>
                                    <span>
                                      {t(
                                        'Per-Model & Per-Channel Cost Breakdown for'
                                      )}{' '}
                                      {u.username} ({u.model_breakdowns.length})
                                    </span>
                                    <span className='text-muted-foreground font-mono text-[11px]'>
                                      {t('Lifetime Consumed')}:{' '}
                                      {formatUSD(u.lifetime_used_usd)}
                                    </span>
                                  </div>
                                  {u.model_breakdowns.length === 0 ? (
                                    <p className='text-muted-foreground py-4 text-center text-xs'>
                                      {t(
                                        'No model consumption logs in selected time range'
                                      )}
                                    </p>
                                  ) : (
                                    <Table>
                                      <TableHeader>
                                        <TableRow>
                                          <TableHead className='h-8 text-[11px]'>
                                            {t('Model')}
                                          </TableHead>
                                          <TableHead className='h-8 text-[11px]'>
                                            {t('Channel')}
                                          </TableHead>
                                          <TableHead className='h-8 text-right text-[11px]'>
                                            {t('Call Count')}
                                          </TableHead>
                                          <TableHead className='h-8 text-right text-[11px]'>
                                            {t('Tokens (In / Out)')}
                                          </TableHead>
                                          <TableHead className='h-8 text-right text-[11px]'>
                                            {t('Revenue')}
                                          </TableHead>
                                          <TableHead className='h-8 text-right text-[11px]'>
                                            {t('Upstream Cost')}
                                          </TableHead>
                                          <TableHead className='h-8 text-right text-[11px]'>
                                            {t('Profit & Margin')}
                                          </TableHead>
                                        </TableRow>
                                      </TableHeader>
                                      <TableBody>
                                        {u.model_breakdowns.map((bd) => (
                                          <TableRow
                                            key={`${bd.model}-${bd.channel_id}`}
                                          >
                                            <TableCell className='py-1.5 font-mono text-xs font-medium'>
                                              {bd.model}
                                            </TableCell>
                                            <TableCell className='py-1.5 text-xs'>
                                              #{bd.channel_id} {bd.channel_name}
                                            </TableCell>
                                            <TableCell className='py-1.5 text-right font-mono text-xs'>
                                              {formatNumber(
                                                bd.call_count,
                                                intlLocale
                                              )}
                                            </TableCell>
                                            <TableCell className='py-1.5 text-right font-mono text-xs'>
                                              {formatNumber(
                                                bd.prompt_tokens,
                                                intlLocale
                                              )}{' '}
                                              /{' '}
                                              {formatNumber(
                                                bd.completion_tokens,
                                                intlLocale
                                              )}
                                            </TableCell>
                                            <TableCell className='py-1.5 text-right font-mono text-xs text-emerald-600 dark:text-emerald-400'>
                                              {formatUSD(bd.revenue_usd)}
                                            </TableCell>
                                            <TableCell className='py-1.5 text-right font-mono text-xs'>
                                              {formatUSD(bd.cost_usd)}
                                            </TableCell>
                                            <TableCell className='py-1.5 text-right font-mono text-xs'>
                                              <span className='me-1.5'>
                                                {formatUSD(bd.profit_usd)}
                                              </span>
                                              <Badge
                                                variant={getMarginBadgeVariant(
                                                  bd.margin_pct
                                                )}
                                                className='text-[10px]'
                                              >
                                                {bd.margin_pct.toFixed(1)}%
                                              </Badge>
                                            </TableCell>
                                          </TableRow>
                                        ))}
                                      </TableBody>
                                    </Table>
                                  )}
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
    </div>
  )
}
