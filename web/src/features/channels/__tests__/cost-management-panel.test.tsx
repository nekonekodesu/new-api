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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import { api } from '@/lib/api'

import { CostManagementPanel } from '../components/cost-management-panel'

const i18n = createInstance()

beforeEach(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: false,
    nsSeparator: false,
    resources: { en },
    interpolation: { escapeValue: false },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('CostManagementPanel user cost data status display', () => {
  it('renders Insufficient Data for unlogged historical usage while keeping 100.0% for genuine zero-cost logs', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          users: [
            {
              user_id: 101,
              username: 'unlogged-user',
              display_name: 'Unlogged User',
              group: 'default',
              status: 1,
              remain_quota_usd: 0,
              lifetime_used_usd: 20,
              topup_total_usd: 20,
              online_topup_usd: 0,
              online_paid_money: 0,
              system_topup_usd: 20,
              consumed_usd: 20,
              covered_revenue_usd: 0,
              uncovered_revenue_usd: 20,
              cost_data_status: 'insufficient',
              call_count: 0,
              prompt_tokens: 0,
              completion_tokens: 0,
              total_tokens: 0,
              revenue_usd: 20,
              cost_usd: 0,
              gross_profit_usd: 0,
              gross_margin_pct: 0,
              model_breakdowns: [],
            },
            {
              user_id: 103,
              username: 'zerocost-user',
              display_name: 'Zero Cost User',
              group: 'default',
              status: 1,
              remain_quota_usd: 0,
              lifetime_used_usd: 5,
              topup_total_usd: 5,
              online_topup_usd: 0,
              online_paid_money: 0,
              system_topup_usd: 5,
              consumed_usd: 5,
              covered_revenue_usd: 5,
              uncovered_revenue_usd: 0,
              cost_data_status: 'complete',
              call_count: 10,
              prompt_tokens: 500,
              completion_tokens: 500,
              total_tokens: 1000,
              revenue_usd: 5,
              cost_usd: 0,
              gross_profit_usd: 5,
              gross_margin_pct: 100,
              model_breakdowns: [],
            },
          ],
          summary: {
            user_count: 2,
            total_topup_usd: 25,
            total_consumed_usd: 25,
            total_revenue_usd: 5,
            covered_revenue_usd: 5,
            uncovered_revenue_usd: 20,
            cost_data_status: 'partial',
            total_cost_usd: 0,
            total_profit_usd: 5,
            overall_margin_pct: 100,
            total_calls: 10,
            total_tokens: 1000,
          },
        },
      },
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <CostManagementPanel mode='users' />
        </QueryClientProvider>
      </I18nextProvider>
    )

    const unloggedLabel = await screen.findByText('unlogged-user')
    const unloggedRow = unloggedLabel.closest('tr')
    expect(unloggedRow).not.toBeNull()
    if (unloggedRow) {
      expect(
        within(unloggedRow).getByText('Insufficient Data')
      ).toBeInTheDocument()
      expect(within(unloggedRow).queryByText('100.0%')).toBeNull()
    }

    const zeroCostLabel = screen.getByText('zerocost-user')
    const zeroCostRow = zeroCostLabel.closest('tr')
    expect(zeroCostRow).not.toBeNull()
    if (zeroCostRow) {
      expect(within(zeroCostRow).getByText('100.0%')).toBeInTheDocument()
    }

    expect(screen.getByText('Partial Period')).toBeInTheDocument()
  })
})
