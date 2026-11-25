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
import { afterEach, describe, expect, it } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'
import {
  DEFAULT_CURRENCY_CONFIG,
  DEFAULT_CURRENCY_RATES,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { getSitePricingCurrency } from '@/features/model-pricing/currency'
import {
  formatAmount,
  formatBillingAmountUSD,
  formatBillingUnitCostUSD,
  formatCurrencyFromUSD,
  formatRawUpstreamAmount,
  getCurrencyDisplay,
} from '../currency'
import { mapStatusDataToConfig } from '../status-query'

describe('multi-currency exchange rates and formatting', () => {
  afterEach(() => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        currencyRates: { ...DEFAULT_CURRENCY_RATES },
      },
    })
  })

  it('uses NTD/TWD own exchange rate rather than CNY usdExchangeRate', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'NTD',
        usdExchangeRate: 7.3,
        ntdExchangeRate: 32,
        currencyRates: {
          ...DEFAULT_CURRENCY_RATES,
          CNY: { symbol: '¥', rate: 7.3 },
          TWD: { symbol: 'NT$', rate: 32 },
        },
      },
    })

    const { meta } = getCurrencyDisplay()
    expect(meta.kind).toBe('currency')
    if (meta.kind === 'currency') {
      expect(meta.currencyCode).toBe('TWD')
      expect(meta.exchangeRate).toBe(32)
    }
    expect(
      formatCurrencyFromUSD(10, {
        abbreviate: false,
        locale: toIntlLocale('en'),
      })
    ).toBe('NT$320')
    expect(
      formatCurrencyFromUSD(10, {
        abbreviate: false,
        locale: toIntlLocale('zhTW'),
      })
    ).toBe('NT$320')
    expect(
      formatCurrencyFromUSD(-10, {
        abbreviate: false,
        locale: toIntlLocale('zhTW'),
      })
    ).toBe('-NT$320')
  })

  it('formats dynamic currencies via formatAmount and active quotaDisplayType', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'EUR',
        currencyRates: {
          ...DEFAULT_CURRENCY_RATES,
          EUR: { symbol: '€', rate: 0.92 },
          JPY: { symbol: '¥', rate: 155 },
          USDT: { symbol: '₮', rate: 1 },
        },
      },
    })

    expect(
      formatCurrencyFromUSD(10, {
        abbreviate: false,
        locale: toIntlLocale('en'),
      })
    ).toBe('€9.2')
    expect(
      formatAmount(2, 'JPY', {
        abbreviate: false,
        locale: toIntlLocale('ja'),
      })
    ).toBe('￥310')
    expect(
      formatAmount(10, 'USDT', {
        abbreviate: false,
        locale: toIntlLocale('zhCN'),
      })
    ).toBe('₮10')
  })

  it('maps backend status currency_rates and ntd_exchange_rate into system config', () => {
    const mapped = mapStatusDataToConfig({
      quota_display_type: 'TWD',
      usd_exchange_rate: 7.2,
      ntd_exchange_rate: 31.5,
      currency_rates: {
        USD: { symbol: '$', rate: 99 },
        TWD: { symbol: 'NT$', rate: 31.8 },
        EUR: { symbol: '€', rate: 0.91 },
      },
    })

    expect(mapped.currency?.ntdExchangeRate).toBe(31.5)
    expect(mapped.currency?.currencyRates?.USD.rate).toBe(1)
    expect(mapped.currency?.currencyRates?.TWD.rate).toBe(31.8)
    expect(mapped.currency?.currencyRates?.EUR).toEqual({
      symbol: '€',
      rate: 0.91,
    })
  })

  it('normalizes legacy CNY TWD-per-CNY rate <= 5.8 in getSitePricingCurrency', () => {
    expect(
      getSitePricingCurrency({
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        ntdExchangeRate: 32,
        currencyRates: {
          ...DEFAULT_CURRENCY_RATES,
          CNY: { symbol: '¥', rate: 4.75 },
          TWD: { symbol: 'NT$', rate: 32 },
        },
      })
    ).toEqual({
      label: 'CNY',
      symbol: '¥',
      exchangeRate: 32 / 4.75,
    })

    expect(
      getSitePricingCurrency({
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'CNY',
        currencyRates: {
          ...DEFAULT_CURRENCY_RATES,
          CNY: { symbol: '¥', rate: 7.2 },
        },
      })
    ).toEqual({
      label: 'CNY',
      symbol: '¥',
      exchangeRate: 7.2,
    })

    expect(
      getSitePricingCurrency({
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'USD',
      })
    ).toBeNull()

    expect(
      getSitePricingCurrency({
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'TOKENS',
      })
    ).toBeNull()
  })

  it('formats billing amounts, unit costs, and raw upstream amounts consistently', () => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'USD',
      },
    })

    expect(formatBillingAmountUSD(undefined, 'en-US')).toBe('-')
    expect(formatBillingAmountUSD(Number.NaN, 'en-US', '0')).toBe('$0')
    expect(formatBillingAmountUSD(12.345, 'en-US')).toBe('$12.35')

    expect(formatBillingUnitCostUSD(0, 'en-US')).toBe('$0')
    expect(formatBillingUnitCostUSD(0.12345, 'en-US')).toBe('$0.1235')

    expect(formatRawUpstreamAmount(undefined, 'CNY')).toBe('')
    expect(formatRawUpstreamAmount(0, 'CNY')).toBe('')
    expect(formatRawUpstreamAmount(1.23456, 'CNY')).toBe('¥1.2346')
    expect(
      formatRawUpstreamAmount(1.23456, 'TWD', { includeCode: true })
    ).toBe('NT$1.2346 TWD')
    expect(
      formatRawUpstreamAmount(1.5, 'AUTO', { includeCode: true })
    ).toBe('')
  })
})
