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
import { Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export interface PaymentMethodRateEntry {
  currency: string
  exchange_rate: number
  fee_rate: number
  fixed_fee: number
}

export interface TaiwanPaymentSettingsValues {
  PaymentMethodsConfig: string
  ECPayMerchantID: string
  ECPayHashKey: string
  ECPayHashIV: string
  ECPayEndpoint: string
  NewebPayMerchantID: string
  NewebPayHashKey: string
  NewebPayHashIV: string
  NewebPayEndpoint: string
}

const SUGGESTED_METHOD_PRESETS: Record<string, PaymentMethodRateEntry> = {
  ecpay: { currency: 'TWD', exchange_rate: 32, fee_rate: 0, fixed_fee: 0 },
  newebpay: { currency: 'TWD', exchange_rate: 32, fee_rate: 0, fixed_fee: 0 },
  epay: { currency: 'CNY', exchange_rate: 7.3, fee_rate: 0, fixed_fee: 0 },
  stripe: { currency: 'USD', exchange_rate: 1, fee_rate: 0, fixed_fee: 0 },
}

function parsePaymentMethodRates(
  raw: string
): Record<string, PaymentMethodRateEntry> {
  if (!raw || !raw.trim()) return {}
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }
    const result: Record<string, PaymentMethodRateEntry> = {}
    for (const [key, val] of Object.entries(parsed)) {
      const normalizedKey = key.trim().toLowerCase()
      if (!normalizedKey || !val || typeof val !== 'object') continue
      const record = val as Record<string, unknown>
      const currency =
        typeof record.currency === 'string' && record.currency.trim()
          ? record.currency.trim().toUpperCase()
          : 'USD'
      const exchangeRate = Number(record.exchange_rate)
      const feeRate = Number(record.fee_rate)
      const fixedFee = Number(record.fixed_fee)
      result[normalizedKey] = {
        currency,
        exchange_rate:
          Number.isFinite(exchangeRate) && exchangeRate > 0 ? exchangeRate : 1,
        fee_rate: Number.isFinite(feeRate) && feeRate >= 0 ? feeRate : 0,
        fixed_fee: Number.isFinite(fixedFee) && fixedFee >= 0 ? fixedFee : 0,
      }
    }
    return result
  } catch {
    return {}
  }
}

interface PaymentMethodRatesSectionProps {
  value: string
  onChange: (nextJson: string) => void
  disabled?: boolean
}

export function PaymentMethodRatesSection(
  props: PaymentMethodRatesSectionProps
) {
  const { t } = useTranslation()
  const [newMethod, setNewMethod] = useState('')
  const [newCurrency, setNewCurrency] = useState('TWD')
  const [newRate, setNewRate] = useState('32')
  const [newFeeRate, setNewFeeRate] = useState('0')
  const [newFixedFee, setNewFixedFee] = useState('0')
  const [errorText, setErrorText] = useState('')

  const methodsMap = useMemo(
    () => parsePaymentMethodRates(props.value),
    [props.value]
  )
  const entries = useMemo(
    () => Object.entries(methodsMap).sort(([a], [b]) => a.localeCompare(b)),
    [methodsMap]
  )

  const commit = (next: Record<string, PaymentMethodRateEntry>) => {
    props.onChange(JSON.stringify(next))
  }

  const updateEntry = (
    method: string,
    patch: Partial<PaymentMethodRateEntry>
  ) => {
    const current = methodsMap[method]
    if (!current) return
    commit({
      ...methodsMap,
      [method]: {
        ...current,
        ...patch,
      },
    })
  }

  const handleAddMethod = () => {
    const methodKey = newMethod.trim().toLowerCase()
    const currency = newCurrency.trim().toUpperCase()
    const exchangeRate = Number(newRate)
    const feeRate = Number(newFeeRate || 0)
    const fixedFee = Number(newFixedFee || 0)

    if (!methodKey) {
      setErrorText(t('Payment method key is required (e.g. ecpay, newebpay)'))
      return
    }
    if (!currency) {
      setErrorText(t('Currency code is required'))
      return
    }
    if (!Number.isFinite(exchangeRate) || exchangeRate <= 0) {
      setErrorText(t('Exchange rate must be greater than 0'))
      return
    }
    if (!Number.isFinite(feeRate) || feeRate < 0) {
      setErrorText(t('Fee rate must be 0 or greater'))
      return
    }
    if (!Number.isFinite(fixedFee) || fixedFee < 0) {
      setErrorText(t('Fixed fee must be 0 or greater'))
      return
    }

    setErrorText('')
    commit({
      ...methodsMap,
      [methodKey]: {
        currency,
        exchange_rate: exchangeRate,
        fee_rate: feeRate,
        fixed_fee: fixedFee,
      },
    })
    setNewMethod('')
  }

  const handleApplyPreset = (key: string) => {
    const preset = SUGGESTED_METHOD_PRESETS[key]
    if (!preset) return
    commit({
      ...methodsMap,
      [key]: { ...preset },
    })
  }

  const handleRemove = (key: string) => {
    const next = { ...methodsMap }
    delete next[key]
    commit(next)
  }

  return (
    <div className='flex flex-col gap-4'>
      <div>
        <h3 className='text-sm font-semibold'>
          {t('Payment Method Exchange Rates & Fees')}
        </h3>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Configure per-gateway settlement currency, USD exchange rate (provider currency per 1 USD), percentage fee rate, and fixed fee. Used when converting top-up orders to system USD balance.'
          )}
        </p>
      </div>

      <div className='flex flex-wrap items-center gap-1.5'>
        <span className='text-muted-foreground text-xs'>
          {t('Quick add preset')}:
        </span>
        {Object.keys(SUGGESTED_METHOD_PRESETS).map((presetKey) => (
          <Button
            key={presetKey}
            type='button'
            variant='outline'
            size='sm'
            disabled={props.disabled || Boolean(methodsMap[presetKey])}
            onClick={() => handleApplyPreset(presetKey)}
            className='h-7 px-2.5 font-mono text-xs'
          >
            + {presetKey}
          </Button>
        ))}
      </div>

      {entries.length > 0 ? (
        <div className='border-border/50 overflow-x-auto rounded-md border'>
          <Table className='min-w-[46rem]'>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Method')}</TableHead>
                <TableHead className='w-28'>{t('Currency')}</TableHead>
                <TableHead className='w-32'>{t('Rate (per USD)')}</TableHead>
                <TableHead className='w-28'>{t('Fee Rate')}</TableHead>
                <TableHead className='w-28'>{t('Fixed Fee')}</TableHead>
                <TableHead className='w-36'>{t('10 USD Preview')}</TableHead>
                <TableHead className='w-14 text-right'>
                  {t('Actions')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map(([method, entry]) => {
                const base = 10 * entry.exchange_rate
                const total = base + base * entry.fee_rate + entry.fixed_fee
                return (
                  <TableRow key={method}>
                    <TableCell className='font-mono text-xs font-semibold'>
                      {method}
                    </TableCell>
                    <TableCell>
                      <Input
                        aria-label={`${t('Currency')} ${method}`}
                        value={entry.currency}
                        disabled={props.disabled}
                        maxLength={8}
                        onChange={(e) =>
                          updateEntry(method, {
                            currency: e.target.value.toUpperCase(),
                          })
                        }
                        className='h-8 font-mono text-xs'
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        aria-label={`${t('Rate (per USD)')} ${method}`}
                        type='number'
                        step='0.0001'
                        min='0.0001'
                        disabled={props.disabled}
                        value={entry.exchange_rate}
                        onChange={(e) => {
                          const parsed = Number(e.target.value)
                          if (Number.isFinite(parsed) && parsed > 0) {
                            updateEntry(method, { exchange_rate: parsed })
                          }
                        }}
                        className='h-8 font-mono text-xs'
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        aria-label={`${t('Fee Rate')} ${method}`}
                        type='number'
                        step='0.001'
                        min='0'
                        disabled={props.disabled}
                        value={entry.fee_rate}
                        onChange={(e) => {
                          const parsed = Number(e.target.value)
                          if (Number.isFinite(parsed) && parsed >= 0) {
                            updateEntry(method, { fee_rate: parsed })
                          }
                        }}
                        className='h-8 font-mono text-xs'
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        aria-label={`${t('Fixed Fee')} ${method}`}
                        type='number'
                        step='0.01'
                        min='0'
                        disabled={props.disabled}
                        value={entry.fixed_fee}
                        onChange={(e) => {
                          const parsed = Number(e.target.value)
                          if (Number.isFinite(parsed) && parsed >= 0) {
                            updateEntry(method, { fixed_fee: parsed })
                          }
                        }}
                        className='h-8 font-mono text-xs'
                      />
                    </TableCell>
                    <TableCell className='text-muted-foreground font-mono text-xs'>
                      {total.toFixed(2)} {entry.currency}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        disabled={props.disabled}
                        aria-label={`${t('Remove payment method rate')} ${method}`}
                        onClick={() => handleRemove(method)}
                        className='h-7 px-2 text-rose-500 hover:text-rose-600'
                      >
                        <Trash2 aria-hidden='true' className='size-3.5' />
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      ) : null}

      <div className='grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-6'>
        <Input
          value={newMethod}
          disabled={props.disabled}
          onChange={(e) => setNewMethod(e.target.value)}
          placeholder={t('Method (e.g. ecpay)')}
          className='h-8 font-mono text-xs'
        />
        <Input
          value={newCurrency}
          disabled={props.disabled}
          onChange={(e) => setNewCurrency(e.target.value.toUpperCase())}
          placeholder={t('Currency (e.g. TWD)')}
          maxLength={8}
          className='h-8 font-mono text-xs'
        />
        <Input
          type='number'
          step='0.0001'
          min='0.0001'
          disabled={props.disabled}
          value={newRate}
          onChange={(e) => setNewRate(e.target.value)}
          placeholder={t('Rate (e.g. 32)')}
          className='h-8 font-mono text-xs'
        />
        <Input
          type='number'
          step='0.001'
          min='0'
          disabled={props.disabled}
          value={newFeeRate}
          onChange={(e) => setNewFeeRate(e.target.value)}
          placeholder={t('Fee rate (e.g. 0.02)')}
          className='h-8 font-mono text-xs'
        />
        <Input
          type='number'
          step='0.01'
          min='0'
          disabled={props.disabled}
          value={newFixedFee}
          onChange={(e) => setNewFixedFee(e.target.value)}
          placeholder={t('Fixed fee')}
          className='h-8 font-mono text-xs'
        />
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={props.disabled}
          onClick={handleAddMethod}
          className='h-8 gap-1 text-xs'
        >
          <Plus aria-hidden='true' className='size-3.5' />
          <span>{t('Add Method')}</span>
        </Button>
      </div>
      {errorText ? <p className='text-destructive text-xs'>{errorText}</p> : null}
    </div>
  )
}

interface TaiwanPaymentSettingsSectionProps {
  values: TaiwanPaymentSettingsValues
  onFieldChange: <K extends keyof TaiwanPaymentSettingsValues>(
    field: K,
    value: TaiwanPaymentSettingsValues[K]
  ) => void
  disabled?: boolean
}

export function TaiwanPaymentSettingsSection(
  props: TaiwanPaymentSettingsSectionProps
) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-col gap-6'>
      {/* ECPay */}
      <div className='border-border/60 flex flex-col gap-4 rounded-lg border p-4'>
        <div>
          <h3 className='text-sm font-semibold'>{t('ECPay (Green World)')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Configure ECPay MerchantID, HashKey, HashIV, and gateway endpoint. Leave empty to disable ECPay top-ups.'
            )}
          </p>
        </div>

        <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='ecpay-merchant-id'>{t('ECPay Merchant ID')}</Label>
            <Input
              id='ecpay-merchant-id'
              disabled={props.disabled}
              value={props.values.ECPayMerchantID}
              onChange={(e) =>
                props.onFieldChange('ECPayMerchantID', e.target.value)
              }
              placeholder='3002607'
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='ecpay-endpoint'>{t('ECPay Endpoint URL')}</Label>
            <Input
              id='ecpay-endpoint'
              disabled={props.disabled}
              value={props.values.ECPayEndpoint}
              onChange={(e) =>
                props.onFieldChange('ECPayEndpoint', e.target.value)
              }
              placeholder='https://payment.ecpay.com.tw/Cashier/AioCheckOut/V5'
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='ecpay-hash-key'>{t('ECPay HashKey')}</Label>
            <PasswordInput
              id='ecpay-hash-key'
              disabled={props.disabled}
              value={props.values.ECPayHashKey}
              onChange={(e) =>
                props.onFieldChange('ECPayHashKey', e.target.value)
              }
              placeholder={t('Enter HashKey')}
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='ecpay-hash-iv'>{t('ECPay HashIV')}</Label>
            <PasswordInput
              id='ecpay-hash-iv'
              disabled={props.disabled}
              value={props.values.ECPayHashIV}
              onChange={(e) =>
                props.onFieldChange('ECPayHashIV', e.target.value)
              }
              placeholder={t('Enter HashIV')}
            />
          </div>
        </div>
      </div>

      {/* NewebPay */}
      <div className='border-border/60 flex flex-col gap-4 rounded-lg border p-4'>
        <div>
          <h3 className='text-sm font-semibold'>{t('NewebPay')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Configure NewebPay MerchantID, 32-byte AES HashKey, 16-byte HashIV, and MPG endpoint. Leave empty to disable NewebPay top-ups.'
            )}
          </p>
        </div>

        <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='newebpay-merchant-id'>
              {t('NewebPay Merchant ID')}
            </Label>
            <Input
              id='newebpay-merchant-id'
              disabled={props.disabled}
              value={props.values.NewebPayMerchantID}
              onChange={(e) =>
                props.onFieldChange('NewebPayMerchantID', e.target.value)
              }
              placeholder='MS12345678'
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='newebpay-endpoint'>
              {t('NewebPay Endpoint URL')}
            </Label>
            <Input
              id='newebpay-endpoint'
              disabled={props.disabled}
              value={props.values.NewebPayEndpoint}
              onChange={(e) =>
                props.onFieldChange('NewebPayEndpoint', e.target.value)
              }
              placeholder='https://core.newebpay.com/MPG/mpg_gateway'
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='newebpay-hash-key'>{t('NewebPay HashKey')}</Label>
            <PasswordInput
              id='newebpay-hash-key'
              disabled={props.disabled}
              value={props.values.NewebPayHashKey}
              onChange={(e) =>
                props.onFieldChange('NewebPayHashKey', e.target.value)
              }
              placeholder={t('32-character AES HashKey')}
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='newebpay-hash-iv'>{t('NewebPay HashIV')}</Label>
            <PasswordInput
              id='newebpay-hash-iv'
              disabled={props.disabled}
              value={props.values.NewebPayHashIV}
              onChange={(e) =>
                props.onFieldChange('NewebPayHashIV', e.target.value)
              }
              placeholder={t('16-character AES HashIV')}
            />
          </div>
        </div>
      </div>
    </div>
  )
}
