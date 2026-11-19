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
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  DEFAULT_CURRENCY_CONFIG,
  DEFAULT_CURRENCY_RATES,
  type CurrencyRateConfig,
} from '@/stores/system-config-store'

import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const CURRENCY_CODE_REGEX = /^[A-Z]{2,8}$/

function parseCurrencyRatesJson(
  raw: string | undefined,
  cnyRate: number,
  ntdRate: number
): Record<string, CurrencyRateConfig> {
  const result: Record<string, CurrencyRateConfig> = {
    ...DEFAULT_CURRENCY_RATES,
    CNY: { symbol: '¥', rate: cnyRate > 0 ? cnyRate : 7.3 },
    TWD: { symbol: 'NT$', rate: ntdRate > 0 ? ntdRate : 32 },
  }
  if (!raw || !raw.trim()) {
    return result
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return result
    }
    for (const [rawCode, entry] of Object.entries(parsed)) {
      const code = rawCode.trim().toUpperCase()
      if (!CURRENCY_CODE_REGEX.test(code)) continue
      const normalized = code === 'NTD' ? 'TWD' : code
      if (!entry || typeof entry !== 'object') continue
      const record = entry as Record<string, unknown>
      const rate = Number(record.rate)
      const symbol =
        typeof record.symbol === 'string' && record.symbol.trim()
          ? record.symbol.trim()
          : (result[normalized]?.symbol ?? normalized)
      if (normalized === 'USD') {
        result.USD = { symbol, rate: 1 }
      } else if (Number.isFinite(rate) && rate > 0) {
        result[normalized] = { symbol, rate }
      }
    }
  } catch {
    return result
  }
  return result
}

const createPricingSchema = (t: (key: string) => string) =>
  z
    .object({
      QuotaPerUnit: z.coerce.number().min(0, t('Value must be at least 0')),
      USDExchangeRate: z.coerce
        .number()
        .min(0.0001, t('Exchange rate must be greater than 0')),
      NTDExchangeRate: z.coerce
        .number()
        .min(0.0001, t('Exchange rate must be greater than 0')),
      DisplayInCurrencyEnabled: z.boolean(),
      DisplayTokenStatEnabled: z.boolean(),
      general_setting: z.object({
        quota_display_type: z.string().min(1),
        custom_currency_symbol: z.string().max(8).optional(),
        custom_currency_exchange_rate: z.coerce
          .number()
          .min(0.0001, t('Exchange rate must be greater than 0'))
          .optional(),
        currency_rates: z.string().optional(),
      }),
    })
    .superRefine((data, ctx) => {
      const displayType = data.general_setting.quota_display_type

      if (displayType === 'CUSTOM') {
        if (!data.general_setting.custom_currency_symbol?.trim()) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['general_setting', 'custom_currency_symbol'],
            message: t('Custom currency symbol is required'),
          })
        }

        if (data.general_setting.custom_currency_exchange_rate == null) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['general_setting', 'custom_currency_exchange_rate'],
            message: t('Exchange rate is required'),
          })
        }
      }
    })

type PricingFormValues = z.infer<ReturnType<typeof createPricingSchema>>

type PricingSectionProps = {
  defaultValues: PricingFormValues
}

export function PricingSection({ defaultValues }: PricingSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const pricingSchema = createPricingSchema(t)

  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<PricingFormValues>({
      resolver: zodResolver(pricingSchema) as Resolver<
        PricingFormValues,
        unknown,
        PricingFormValues
      >,
      defaultValues,
      onSubmit: async (data, changedFields) => {
        const updates: Record<string, unknown> = { ...changedFields }
        if (
          ('USDExchangeRate' in updates || 'NTDExchangeRate' in updates) &&
          !('general_setting.currency_rates' in updates)
        ) {
          const synced = parseCurrencyRatesJson(
            data.general_setting.currency_rates,
            data.USDExchangeRate,
            data.NTDExchangeRate
          )
          if (data.USDExchangeRate > 0) {
            synced.CNY = {
              symbol: synced.CNY?.symbol || '¥',
              rate: data.USDExchangeRate,
            }
          }
          if (data.NTDExchangeRate > 0) {
            synced.TWD = {
              symbol: synced.TWD?.symbol || 'NT$',
              rate: data.NTDExchangeRate,
            }
          }
          updates['general_setting.currency_rates'] = JSON.stringify(synced)
        }

        for (const [key, value] of Object.entries(updates)) {
          if (value === undefined || value === null) continue
          if (typeof value === 'object') continue

          let serialized: string | boolean = value as string | boolean

          if (typeof value === 'boolean') {
            serialized = String(value)
          } else if (typeof value === 'number') {
            serialized = Number.isFinite(value) ? String(value) : '0'
          }

          await updateOption.mutateAsync({
            key,
            value: serialized,
          })
        }
      },
    })

  const displayType = form.watch('general_setting.quota_display_type') ?? 'USD'
  const displayInCurrencyEnabled = form.watch('DisplayInCurrencyEnabled')
  const usdExchangeRate = form.watch('USDExchangeRate')
  const ntdExchangeRate = form.watch('NTDExchangeRate')
  const currencyRatesJson = form.watch('general_setting.currency_rates')
  const showTokensOnlyOption = displayType === 'TOKENS'
  const showQuotaPerUnit =
    displayType === 'TOKENS' ||
    defaultValues.QuotaPerUnit !== DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const showDisplayInCurrencyOption = displayInCurrencyEnabled === false

  const [newCode, setNewCode] = useState('')
  const [newSymbol, setNewSymbol] = useState('')
  const [newRate, setNewRate] = useState('')
  const [addError, setAddError] = useState('')

  const currencyRatesMap = useMemo(
    () =>
      parseCurrencyRatesJson(
        currencyRatesJson,
        Number(usdExchangeRate),
        Number(ntdExchangeRate)
      ),
    [currencyRatesJson, usdExchangeRate, ntdExchangeRate]
  )

  const currencyEntries = useMemo(() => {
    const entries = Object.entries(currencyRatesMap)
    entries.sort(([a], [b]) => {
      if (a === 'USD') return -1
      if (b === 'USD') return 1
      return a.localeCompare(b)
    })
    return entries
  }, [currencyRatesMap])

  const commitCurrencyRates = (next: Record<string, CurrencyRateConfig>) => {
    form.setValue('general_setting.currency_rates', JSON.stringify(next), {
      shouldDirty: true,
    })
    if (next.CNY && next.CNY.rate > 0 && next.CNY.rate !== usdExchangeRate) {
      form.setValue('USDExchangeRate', next.CNY.rate, { shouldDirty: true })
    }
    if (next.TWD && next.TWD.rate > 0 && next.TWD.rate !== ntdExchangeRate) {
      form.setValue('NTDExchangeRate', next.TWD.rate, { shouldDirty: true })
    }
  }

  const handleUpdateCurrencyEntry = (
    code: string,
    patch: Partial<CurrencyRateConfig>
  ) => {
    const current = currencyRatesMap[code]
    if (!current) return
    let nextRate = current.rate
    if (code === 'USD') {
      nextRate = 1
    } else if (patch.rate !== undefined) {
      nextRate = patch.rate
    }
    const nextSymbol =
      patch.symbol !== undefined ? patch.symbol : current.symbol
    commitCurrencyRates({
      ...currencyRatesMap,
      [code]: {
        symbol: nextSymbol,
        rate: nextRate,
      },
    })
  }

  const handleAddCurrency = () => {
    const code = newCode.trim().toUpperCase()
    const symbol = newSymbol.trim()
    const rate = Number(newRate)
    if (!CURRENCY_CODE_REGEX.test(code) || code === 'TOKENS' || code === 'CUSTOM') {
      setAddError(t('Currency code must be 2-8 uppercase letters (e.g. HKD, JPY)'))
      return
    }
    if (!symbol) {
      setAddError(t('Currency symbol is required'))
      return
    }
    if (!Number.isFinite(rate) || rate <= 0) {
      setAddError(t('Exchange rate must be greater than 0'))
      return
    }
    setAddError('')
    commitCurrencyRates({
      ...currencyRatesMap,
      [code === 'NTD' ? 'TWD' : code]: {
        symbol,
        rate: code === 'USD' ? 1 : rate,
      },
    })
    setNewCode('')
    setNewSymbol('')
    setNewRate('')
  }

  const handleRemoveCurrency = (code: string) => {
    if (code === 'USD') return
    const next = { ...currencyRatesMap }
    delete next[code]
    commitCurrencyRates(next)
    if (
      displayType === code ||
      (code === 'TWD' && displayType === 'NTD')
    ) {
      form.setValue('general_setting.quota_display_type', 'USD', {
        shouldDirty: true,
      })
    }
  }

  const displayModeItems = useMemo(() => {
    const items = currencyEntries.map(([code, entry]) => ({
      value: code === 'TWD' && displayType === 'NTD' ? 'NTD' : code,
      label: `${code} (${entry.symbol})`,
    }))
    items.push({ value: 'CUSTOM', label: t('Custom Currency') })
    if (showTokensOnlyOption) {
      items.push({ value: 'TOKENS', label: t('Tokens Only') })
    }
    return items
  }, [currencyEntries, displayType, showTokensOnlyOption, t])

  return (
    <>
      <FormNavigationGuard when={isDirty} />

      <SettingsSection title={t('Pricing & Display')}>
        <Form {...form}>
          <SettingsForm onSubmit={handleSubmit}>
            <SettingsPageFormActions
              onSave={handleSubmit}
              onReset={handleReset}
              isSaving={updateOption.isPending || isSubmitting}
              isResetDisabled={!isDirty}
            />
            <FormDirtyIndicator isDirty={isDirty} />
            {showQuotaPerUnit && (
              <FormField
                control={form.control}
                name='QuotaPerUnit'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Quota Per Unit')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        value={field.value as number}
                        disabled
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Number of tokens per unit quota')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name='general_setting.quota_display_type'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Display Mode')}</FormLabel>
                  <Select
                    items={displayModeItems}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder={t('Select display mode')} />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {displayModeItems.map((item) => (
                          <SelectItem key={item.value} value={item.value}>
                            {item.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t('Choose how quota values are shown to users')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Multi-Currency Exchange Rates Table */}
            <div className='border-border/60 flex flex-col gap-3 rounded-lg border p-4'>
              <div>
                <h4 className='text-sm font-medium'>
                  {t('Currency Exchange Rates')}
                </h4>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Configure exchange rates relative to 1 USD. All currencies listed here are available as display modes and for multi-currency conversion.'
                  )}
                </p>
              </div>

              <div className='border-border/50 overflow-x-auto rounded-md border'>
                <Table className='min-w-[28rem]'>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-28'>
                        {t('Currency Code')}
                      </TableHead>
                      <TableHead className='w-32'>{t('Symbol')}</TableHead>
                      <TableHead>{t('Rate (1 USD =)')}</TableHead>
                      <TableHead className='w-16 text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {currencyEntries.map(([code, entry]) => (
                      <TableRow key={code}>
                        <TableCell className='font-mono text-xs font-semibold'>
                          {code}
                        </TableCell>
                        <TableCell>
                          <Input
                            aria-label={`${t('Symbol')} ${code}`}
                            value={entry.symbol}
                            maxLength={8}
                            onChange={(e) =>
                              handleUpdateCurrencyEntry(code, {
                                symbol: e.target.value,
                              })
                            }
                            className='h-8 text-xs'
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            aria-label={`${t('Rate (1 USD =)')} ${code}`}
                            type='number'
                            step='0.0001'
                            min='0.0001'
                            disabled={code === 'USD'}
                            value={entry.rate}
                            onChange={(e) => {
                              const parsed = Number(e.target.value)
                              if (Number.isFinite(parsed) && parsed > 0) {
                                handleUpdateCurrencyEntry(code, {
                                  rate: parsed,
                                })
                              }
                            }}
                            className='h-8 font-mono text-xs'
                          />
                        </TableCell>
                        <TableCell className='text-right'>
                          {code !== 'USD' ? (
                            <Button
                              type='button'
                              variant='ghost'
                              size='sm'
                              aria-label={`${t('Remove currency')} ${code}`}
                              onClick={() => handleRemoveCurrency(code)}
                              className='h-7 px-2 text-rose-500 hover:text-rose-600'
                            >
                              <Trash2 aria-hidden='true' className='size-3.5' />
                            </Button>
                          ) : null}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>

              <div className='grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4'>
                <Input
                  value={newCode}
                  onChange={(e) => setNewCode(e.target.value.toUpperCase())}
                  placeholder={t('Code (e.g. HKD)')}
                  maxLength={8}
                  className='h-8 font-mono text-xs'
                />
                <Input
                  value={newSymbol}
                  onChange={(e) => setNewSymbol(e.target.value)}
                  placeholder={t('Symbol (e.g. HK$)')}
                  maxLength={8}
                  className='h-8 text-xs'
                />
                <Input
                  type='number'
                  step='0.0001'
                  min='0.0001'
                  value={newRate}
                  onChange={(e) => setNewRate(e.target.value)}
                  placeholder={t('Rate (e.g. 7.8)')}
                  className='h-8 font-mono text-xs'
                />
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={handleAddCurrency}
                  className='h-8 gap-1 text-xs'
                >
                  <Plus aria-hidden='true' className='size-3.5' />
                  <span>{t('Add Currency')}</span>
                </Button>
              </div>
              {addError ? (
                <p className='text-destructive text-xs'>{addError}</p>
              ) : null}
            </div>

            {displayType !== 'TOKENS' && displayType !== 'NTD' && (
              <FormField
                control={form.control}
                name='USDExchangeRate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {displayType === 'CNY'
                        ? t('CNY per USD')
                        : t('USD Exchange Rate')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Real exchange rate between USD and your payment gateway currency'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {displayType === 'NTD' && (
              <FormField
                control={form.control}
                name='NTDExchangeRate'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('TWD per USD')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        step='0.01'
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>{t('Real exchange rate between USD and New Taiwan dollar')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {displayType === 'CUSTOM' && (
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='general_setting.custom_currency_symbol'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Custom Currency Symbol')}</FormLabel>
                      <FormControl>
                        <Input
                          type='text'
                          value={field.value ?? ''}
                          onChange={field.onChange}
                          name={field.name}
                          onBlur={field.onBlur}
                          ref={field.ref}
                          maxLength={8}
                          placeholder={t('e.g. ¥ or HK$')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Prefix used when displaying prices')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='general_setting.custom_currency_exchange_rate'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Units per USD')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          step='0.01'
                          value={field.value ?? ''}
                          onChange={(e) =>
                            field.onChange(
                              e.target.value === ''
                                ? undefined
                                : e.target.valueAsNumber
                            )
                          }
                          name={field.name}
                          onBlur={field.onBlur}
                          ref={field.ref}
                          placeholder={t('e.g. 8 means 1 USD = 8 units')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Conversion rate from USD to your custom currency')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            )}

            {showDisplayInCurrencyOption && (
              <FormField
                control={form.control}
                name='DisplayInCurrencyEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Display in Currency')}</FormLabel>
                      <FormDescription>
                        {displayType === 'TOKENS'
                          ? t(
                              'Tokens-only mode will show raw quota values regardless of this toggle.'
                            )
                          : t('Show prices in currency instead of quota.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name='DisplayTokenStatEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Display Token Statistics')}</FormLabel>
                    <FormDescription>
                      {t('Show token usage statistics in the UI')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </SettingsForm>
        </Form>
      </SettingsSection>
    </>
  )
}
