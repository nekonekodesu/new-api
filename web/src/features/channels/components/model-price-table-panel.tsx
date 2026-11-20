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
import { Check, Crown, RefreshCw, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useMediaQuery } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import {
  formatBillingCurrencyFromUSD,
  getAvailableDisplayCurrencies,
  setDisplayCurrency,
} from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { applyModelPriceTable, getModelPriceTable } from '../api'
import { ChannelTypeLogo } from './channel-type-badge'

export function ModelPriceTablePanel() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [searchModel, setSearchModel] = useState('')
  const isMobile = useMediaQuery('(max-width: 640px)')
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const currencyConfig = useSystemConfigStore((state) => state.config.currency)
  const currencyOptions = getAvailableDisplayCurrencies(currencyConfig).slice(
    0,
    3
  )
  const rawDisplayType = (currencyConfig.quotaDisplayType || 'USD')
    .trim()
    .toUpperCase()
  const normalizedDisplayType =
    rawDisplayType === 'NTD' ? 'TWD' : rawDisplayType
  const activeCurrencyCode = currencyOptions.some(
    (o) => o.code === normalizedDisplayType
  )
    ? normalizedDisplayType
    : 'USD'

  const formatCost = (val: number) =>
    formatBillingCurrencyFromUSD(val, {
      digitsLarge: 4,
      digitsSmall: 4,
      abbreviate: false,
      locale: intlLocale,
    })

  const formatRawCurrency = (rawVal: number | undefined, curr: string) => {
    if (typeof rawVal !== 'number' || !Number.isFinite(rawVal) || rawVal <= 0) {
      return null
    }
    const c = (curr || '').toUpperCase().trim()
    if (!c || c === 'AUTO') return null
    let sym = '$'
    if (c === 'CNY') {
      sym = '¥'
    } else if (c === 'TWD' || c === 'NTD') {
      sym = 'NT$'
    }
    return `${sym}${Number(rawVal.toFixed(4))} ${c}`
  }

  const priceTableQuery = useQuery({
    queryKey: ['channel-model-price-table'],
    queryFn: async () => requireServerSuccess(await getModelPriceTable()),
  })

  const rows = useMemo(
    () => priceTableQuery.data?.data?.rows ?? [],
    [priceTableQuery.data]
  )
  const config = priceTableQuery.data?.data?.config
  const highestPricePriority = config?.highest_price_priority ?? true

  const filteredRows = useMemo(() => {
    const kw = searchModel.trim().toLowerCase()
    if (!kw) return rows
    return rows.filter(
      (r) =>
        r.model.toLowerCase().includes(kw) ||
        r.channel_name.toLowerCase().includes(kw) ||
        String(r.channel_id).includes(kw)
    )
  }, [rows, searchModel])

  const applyMutation = useMutation({
    mutationFn: async (payload: {
      highest_price_priority?: boolean
      model?: string
      channel_id?: number
    }) => requireServerSuccess(await applyModelPriceTable(payload)),
    onSuccess: (res) => {
      const updated = res.data?.updated_models ?? 0
      toast.success(
        `${t('Model base prices (1x) synced')}: ${updated} ${t('models')}`
      )
      void queryClient.invalidateQueries({
        queryKey: ['channel-model-price-table'],
      })
    },
    onError: (err) => {
      handleServerError(err, t('Failed to apply model price policy'))
    },
  })

  const handleToggleHighestPriority = (checked: boolean) => {
    applyMutation.mutate({ highest_price_priority: checked })
  }

  const handleSelectChannelPrice = (modelName: string, channelId: number) => {
    applyMutation.mutate({
      highest_price_priority: false,
      model: modelName,
      channel_id: channelId,
    })
  }

  if (priceTableQuery.isLoading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-16 w-full rounded-xl' />
        <Skeleton className='h-80 w-full rounded-xl' />
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      {/* Search Bar & Highest Price Priority Controls */}
      <div className='border-border/60 bg-card flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4'>
        <div className='flex w-full flex-wrap items-center gap-3 sm:w-auto sm:gap-4'>
          <div className='relative w-full sm:w-72'>
            <Search
              aria-hidden='true'
              className='text-muted-foreground absolute start-2.5 top-1/2 size-4 -translate-y-1/2'
            />
            <Input
              value={searchModel}
              onChange={(e) => setSearchModel(e.target.value)}
              placeholder={t('Search model name or channel...')}
              className='h-9 ps-8 text-sm'
            />
          </div>

          <div className='flex items-center gap-2'>
            <Checkbox
              id='highest-price-priority-checkbox'
              checked={highestPricePriority}
              onCheckedChange={(val) =>
                handleToggleHighestPriority(Boolean(val))
              }
            />
            <Label
              htmlFor='highest-price-priority-checkbox'
              className='cursor-pointer text-sm font-medium'
            >
              {t('Select highest price')}
            </Label>
          </div>
        </div>

        <div className='flex w-full flex-wrap items-center gap-2 sm:w-auto'>
          {currencyOptions.length > 1 && (
            <ToggleGroup
              value={[activeCurrencyCode]}
              onValueChange={(values) => {
                if (values.length > 0 && values[0]) {
                  setDisplayCurrency(values[0])
                }
              }}
              variant='outline'
              size='sm'
              aria-label={t('Currency')}
            >
              {currencyOptions.map((opt) => (
                <ToggleGroupItem
                  key={opt.code}
                  value={opt.code}
                  title={`1 USD = ${opt.rate} ${opt.code}`}
                >
                  {opt.symbol} {opt.code}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          )}
          <Badge
            variant='outline'
            className='max-w-full text-xs whitespace-normal'
          >
            {t('Base Price: 1x (Group multiplier configured per group)')}
          </Badge>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void priceTableQuery.refetch()}
            disabled={priceTableQuery.isFetching}
            className='gap-1.5'
          >
            <RefreshCw
              aria-hidden='true'
              className={cn(
                'size-3.5',
                priceTableQuery.isFetching && 'animate-spin'
              )}
            />
            <span>{t('Refresh')}</span>
          </Button>
          <Button
            size='sm'
            className='w-full sm:w-auto'
            onClick={() =>
              applyMutation.mutate({
                highest_price_priority: highestPricePriority,
              })
            }
            disabled={applyMutation.isPending}
          >
            {t('Sync Selected Prices to System (1x)')}
          </Button>
        </div>
      </div>

      {/* Model Price & Telemetry Table / Mobile Cards */}
      {isMobile ? (
        <div className='flex flex-col gap-2.5'>
          {filteredRows.length === 0 ? (
            <div className='border-border/60 bg-card text-muted-foreground rounded-xl border p-8 text-center text-sm'>
              {t('No matching channel models found')}
            </div>
          ) : (
            filteredRows.map((row) => (
              <div
                key={`${row.channel_id}-${row.model}`}
                className={cn(
                  'border-border/60 bg-card flex flex-col gap-2.5 rounded-xl border p-3.5 text-xs',
                  row.is_selected_price &&
                    'border-emerald-500/40 bg-emerald-500/[0.04] dark:bg-emerald-400/[0.05]'
                )}
              >
                <div className='flex items-start justify-between gap-2'>
                  <div className='min-w-0'>
                    <div className='font-mono text-sm font-semibold break-all'>
                      {row.model}
                    </div>
                    <div className='text-muted-foreground mt-1 flex flex-wrap items-center gap-1.5'>
                      <span className='font-mono'>#{row.channel_id}</span>
                      <ChannelTypeLogo type={row.channel_type} size={14} />
                      <span className='text-foreground font-medium'>
                        {row.channel_name}
                      </span>
                      <Badge
                        variant='outline'
                        className='px-1.5 py-0 font-mono text-[10px]'
                      >
                        {row.base_multiplier}x ({row.currency})
                      </Badge>
                    </div>
                  </div>
                  <div className='shrink-0'>
                    {row.is_selected_price ? (
                      <Badge className='gap-1 bg-emerald-600 text-white hover:bg-emerald-600'>
                        <Crown aria-hidden='true' className='size-3' />
                        <span>{t('Active Price')}</span>
                      </Badge>
                    ) : (
                      <Button
                        variant='outline'
                        size='sm'
                        className='h-7 gap-1 px-2 text-xs'
                        onClick={() =>
                          handleSelectChannelPrice(row.model, row.channel_id)
                        }
                        disabled={applyMutation.isPending}
                      >
                        <Check aria-hidden='true' className='size-3' />
                        <span>{t('Use This Channel')}</span>
                      </Button>
                    )}
                  </div>
                </div>

                <div className='bg-muted/30 grid grid-cols-2 gap-2 rounded-lg p-2.5'>
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Input Cost')} (/1M)
                    </div>
                    <div className='font-mono font-semibold'>
                      {formatCost(row.input_cost)}
                    </div>
                    {formatRawCurrency(row.raw_input_cost, row.currency) ? (
                      <div className='text-muted-foreground font-mono text-[10px]'>
                        {formatRawCurrency(row.raw_input_cost, row.currency)}
                        {row.exchange_rate > 0 &&
                        Math.abs(row.exchange_rate - 1) > 1e-4
                          ? ` ÷ ${row.exchange_rate}`
                          : ''}
                      </div>
                    ) : null}
                  </div>
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Output Cost')} (/1M)
                    </div>
                    <div className='font-mono font-semibold'>
                      {formatCost(row.output_cost)}
                    </div>
                    {formatRawCurrency(row.raw_output_cost, row.currency) ? (
                      <div className='text-muted-foreground font-mono text-[10px]'>
                        {formatRawCurrency(row.raw_output_cost, row.currency)}
                        {row.exchange_rate > 0 &&
                        Math.abs(row.exchange_rate - 1) > 1e-4
                          ? ` ÷ ${row.exchange_rate}`
                          : ''}
                      </div>
                    ) : null}
                  </div>
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Cache Ratio')} / {t('Cache Write Ratio')}
                    </div>
                    <div className='font-mono'>
                      {row.cache_ratio > 0
                        ? row.cache_ratio.toFixed(4).replace(/\.?0+$/, '')
                        : '-'}{' '}
                      /{' '}
                      {row.create_cache_ratio > 0
                        ? row.create_cache_ratio
                            .toFixed(4)
                            .replace(/\.?0+$/, '')
                        : '-'}
                    </div>
                  </div>
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Availability')}
                    </div>
                    <div
                      className={cn(
                        'font-mono font-semibold text-rose-600 dark:text-rose-400',
                        row.availability >= 90 &&
                          'text-amber-600 dark:text-amber-400',
                        row.availability >= 99 &&
                          'text-emerald-600 dark:text-emerald-400'
                      )}
                    >
                      {row.availability.toFixed(2)}%
                    </div>
                  </div>
                </div>

                <div className='text-muted-foreground flex flex-wrap items-center justify-between gap-2 text-[11px]'>
                  <span className='font-mono'>
                    TPS (min/avg/max):{' '}
                    {row.tps_min > 0 ? row.tps_min.toFixed(0) : '-'} /{' '}
                    {row.tps_avg > 0 ? row.tps_avg.toFixed(0) : '-'} /{' '}
                    {row.tps_max > 0 ? row.tps_max.toFixed(0) : '-'}
                  </span>
                  <span className='font-mono'>
                    {t('Avg First Token')}:{' '}
                    {row.avg_ttft > 0 ? `${row.avg_ttft.toFixed(2)}s` : '-'}
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      ) : (
        <div className='border-border/60 bg-card overflow-hidden rounded-xl border'>
          <div className='max-h-[65dvh] overflow-auto'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Channel')}</TableHead>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead className='text-right'>
                    {t('Input Cost')} (/1M)
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Output Cost')} (/1M)
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Cache Ratio')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Cache Write Ratio')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Availability')}
                  </TableHead>
                  <TableHead className='text-right'>tps(min)</TableHead>
                  <TableHead className='text-right'>tps(avg)</TableHead>
                  <TableHead className='text-right'>tps(max)</TableHead>
                  <TableHead className='text-right'>
                    {t('Avg First Token')} (s)
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Price Authority')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredRows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={12}
                      className='text-muted-foreground h-28 text-center text-sm'
                    >
                      {t('No matching channel models found')}
                    </TableCell>
                  </TableRow>
                ) : (
                  filteredRows.map((row) => (
                    <TableRow
                      key={`${row.channel_id}-${row.model}`}
                      className={cn(
                        row.is_selected_price &&
                          'bg-emerald-500/[0.04] dark:bg-emerald-400/[0.05]'
                      )}
                    >
                      <TableCell>
                        <div className='flex items-center gap-2'>
                          <span className='text-muted-foreground font-mono text-xs'>
                            {row.channel_id}.
                          </span>
                          <ChannelTypeLogo type={row.channel_type} size={15} />
                          <span className='font-medium'>
                            {row.channel_name}
                          </span>
                          <Badge
                            variant='outline'
                            className='px-1.5 py-0 font-mono text-[10px]'
                          >
                            {row.base_multiplier}x ({row.currency})
                          </Badge>
                        </div>
                      </TableCell>
                      <TableCell className='font-mono text-xs font-semibold'>
                        {row.model}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        <div className='flex flex-col items-end'>
                          <span>{formatCost(row.input_cost)}</span>
                          {formatRawCurrency(
                            row.raw_input_cost,
                            row.currency
                          ) ? (
                            <span className='text-muted-foreground text-[10px]'>
                              {formatRawCurrency(
                                row.raw_input_cost,
                                row.currency
                              )}
                              {row.exchange_rate > 0 &&
                              Math.abs(row.exchange_rate - 1) > 1e-4
                                ? ` ÷ ${row.exchange_rate}`
                                : ''}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        <div className='flex flex-col items-end'>
                          <span>{formatCost(row.output_cost)}</span>
                          {formatRawCurrency(
                            row.raw_output_cost,
                            row.currency
                          ) ? (
                            <span className='text-muted-foreground text-[10px]'>
                              {formatRawCurrency(
                                row.raw_output_cost,
                                row.currency
                              )}
                              {row.exchange_rate > 0 &&
                              Math.abs(row.exchange_rate - 1) > 1e-4
                                ? ` ÷ ${row.exchange_rate}`
                                : ''}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {row.cache_ratio > 0
                          ? row.cache_ratio.toFixed(4).replace(/\.?0+$/, '')
                          : '-'}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {row.create_cache_ratio > 0
                          ? row.create_cache_ratio
                              .toFixed(4)
                              .replace(/\.?0+$/, '')
                          : '-'}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {(() => {
                          if (row.availability >= 99) {
                            return (
                              <span className='font-semibold text-emerald-600 dark:text-emerald-400'>
                                {row.availability.toFixed(2)}%
                              </span>
                            )
                          }
                          if (row.availability >= 90) {
                            return (
                              <span className='font-semibold text-amber-600 dark:text-amber-400'>
                                {row.availability.toFixed(2)}%
                              </span>
                            )
                          }
                          return (
                            <span className='font-semibold text-rose-600 dark:text-rose-400'>
                              {row.availability.toFixed(2)}%
                            </span>
                          )
                        })()}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {row.tps_min > 0 ? row.tps_min.toFixed(0) : '-'}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {row.tps_avg > 0 ? row.tps_avg.toFixed(0) : '-'}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {row.tps_max > 0 ? row.tps_max.toFixed(0) : '-'}
                      </TableCell>
                      <TableCell className='text-right font-mono text-xs'>
                        {row.avg_ttft > 0 ? row.avg_ttft.toFixed(2) : '-'}
                      </TableCell>
                      <TableCell className='text-right'>
                        {row.is_selected_price ? (
                          <Badge className='gap-1 bg-emerald-600 text-white hover:bg-emerald-600'>
                            <Crown aria-hidden='true' className='size-3' />
                            <span>{t('Active Price')}</span>
                          </Badge>
                        ) : (
                          <Button
                            variant='ghost'
                            size='sm'
                            className='h-7 gap-1 px-2 text-xs'
                            onClick={() =>
                              handleSelectChannelPrice(
                                row.model,
                                row.channel_id
                              )
                            }
                            disabled={applyMutation.isPending}
                          >
                            <Check aria-hidden='true' className='size-3' />
                            <span>{t('Use This Channel')}</span>
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </div>
      )}
    </div>
  )
}
