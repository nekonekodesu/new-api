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
import { act, renderHook } from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { usePayment } from '../hooks/use-payment'

describe('usePayment Taiwan gateway checkout form submission', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('submits a single POST form with ECPay params when backend returns legacy params-only payload', async () => {
    const submittedForms: Array<{
      action: string
      method: string
      fields: Record<string, string>
    }> = []

    vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (
      this: HTMLFormElement
    ) {
      const fields: Record<string, string> = {}
      this.querySelectorAll('input').forEach((input) => {
        fields[input.name] = input.value
      })
      submittedForms.push({
        action: this.getAttribute('action') ?? '',
        method: this.method.toUpperCase(),
        fields,
      })
    })

    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        message: 'success',
        url: 'https://payment.ecpay.com.tw/Cashier/AioCheckOut/V5',
        params: {
          MerchantID: '3002607',
          MerchantTradeNo: 'EP0123456789ABCDEFGH',
          TotalAmount: '64',
          CheckMacValue: '6C51C9E6888DE861FD62FB1DD17029FC742634498FD813DC43D4243B5685B840',
        },
      },
    })

    const { result } = renderHook(() => usePayment())
    let ok = false
    await act(async () => {
      ok = await result.current.processPayment(2, 'ecpay')
    })

    expect(ok).toBe(true)
    expect(postSpy).toHaveBeenCalledWith(
      '/api/user/ecpay/pay',
      { amount: 2 },
      expect.objectContaining({ skipBusinessError: true })
    )
    expect(submittedForms).toEqual([
      {
        action: 'https://payment.ecpay.com.tw/Cashier/AioCheckOut/V5',
        method: 'POST',
        fields: {
          MerchantID: '3002607',
          MerchantTradeNo: 'EP0123456789ABCDEFGH',
          TotalAmount: '64',
          CheckMacValue: '6C51C9E6888DE861FD62FB1DD17029FC742634498FD813DC43D4243B5685B840',
        },
      },
    ])
  })

  it('submits a single POST form with NewebPay MPG 2.0 Version and fields', async () => {
    const submittedForms: Array<{
      action: string
      method: string
      fields: Record<string, string>
    }> = []

    vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (
      this: HTMLFormElement
    ) {
      const fields: Record<string, string> = {}
      this.querySelectorAll('input').forEach((input) => {
        fields[input.name] = input.value
      })
      submittedForms.push({
        action: this.getAttribute('action') ?? '',
        method: this.method.toUpperCase(),
        fields,
      })
    })

    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        message: 'success',
        url: 'https://ccore.newebpay.com/MPG/mpg_gateway',
        params: {
          MerchantID: 'MS12345678',
          TradeInfo: 'cipher-hex',
          TradeSha: 'SHA-HEX',
          Version: '2.0',
        },
      },
    })

    const { result } = renderHook(() => usePayment())
    let ok = false
    await act(async () => {
      ok = await result.current.processPayment(10, 'newebpay')
    })

    expect(ok).toBe(true)
    expect(postSpy).toHaveBeenCalledWith(
      '/api/user/newebpay/pay',
      { amount: 10 },
      expect.objectContaining({ skipBusinessError: true })
    )
    expect(submittedForms).toEqual([
      {
        action: 'https://ccore.newebpay.com/MPG/mpg_gateway',
        method: 'POST',
        fields: {
          MerchantID: 'MS12345678',
          TradeInfo: 'cipher-hex',
          TradeSha: 'SHA-HEX',
          Version: '2.0',
        },
      },
    ])
  })

  it('shows an error toast and does not submit when checkout response lacks form parameters', async () => {
    const submitSpy = vi
      .spyOn(HTMLFormElement.prototype, 'submit')
      .mockImplementation(() => {})
    const toastErrorSpy = vi.spyOn(toast, 'error').mockImplementation(() => '')
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        message: 'success',
        url: 'https://payment.ecpay.com.tw/Cashier/AioCheckOut/V5',
      },
    })

    const { result } = renderHook(() => usePayment())
    let ok = true
    await act(async () => {
      ok = await result.current.processPayment(5, 'ecpay')
    })

    expect(ok).toBe(false)
    expect(submitSpy).not.toHaveBeenCalled()
    expect(toastErrorSpy).toHaveBeenCalled()
  })
})
