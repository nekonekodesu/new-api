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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import * as channelsApi from '@/features/channels/api'

import { ModelHealthPanel } from '../models/model-health-panel'

vi.mock('@visactor/react-vchart', () => ({
  VChart: (props: { spec?: { type?: string } }) => (
    <div data-testid='vchart-mock' data-chart-type={props.spec?.type ?? ''} />
  ),
}))

vi.mock('@visactor/vchart', () => ({
  ThemeManager: {
    setCurrentTheme: vi.fn(),
  },
}))

const SAMPLE_HEALTH_PAYLOAD: NonNullable<
  Awaited<ReturnType<typeof channelsApi.getModelHealthAnalysis>>['data']
> = {
  summary: {
    model_count: 1,
    channel_count: 2,
    request_count: 100,
    success_count: 98,
    error_count: 2,
    availability: 98.0,
    avg_ttft_sec: 0.85,
    median_ttft_sec: 0.72,
    p95_ttft_sec: 1.45,
    prefill_tps_avg: 2400,
    prefill_tps_median: 2600,
    decode_tps_p5_low: 42,
    decode_tps_median: 68,
    decode_tps_trimmed_90: 70,
    decode_tps_p95_high: 112,
    decode_tps_avg: 71,
    total_tps_p5_low: 35,
    total_tps_median: 58,
    total_tps_trimmed_90: 60,
    total_tps_p95_high: 96,
    total_tps_avg: 61,
    total_input_tokens: 200_000,
    cache_read_tokens: 120_000,
    cache_write_tokens: 20_000,
    cache_hit_call_count: 60,
    cache_hit_call_ratio: 61.22,
    weighted_cache_rate: 60.0,
    cache_rate_p5_low: 0.0,
    cache_rate_median: 48.5,
    cache_rate_p95_high: 90.0,
    cache_rate_avg: 46.2,
    hit_cache_rate_p5_low: 35.0,
    hit_cache_rate_median: 75.0,
    hit_cache_rate_p95_high: 92.5,
    hit_cache_rate_avg: 72.1,
  },
  models: [
    {
      model: 'claude-sonnet-4-5',
      channel_count: 2,
      request_count: 100,
      success_count: 98,
      error_count: 2,
      availability: 98.0,
      avg_prompt_tokens: 2040,
      avg_output_tokens: 512,
      avg_ttft_sec: 0.85,
      median_ttft_sec: 0.72,
      p95_ttft_sec: 1.45,
      prefill_tps_avg: 2400,
      prefill_tps_median: 2600,
      decode_tps_p5_low: 42,
      decode_tps_median: 68,
      decode_tps_trimmed_90: 70,
      decode_tps_p95_high: 112,
      decode_tps_avg: 71,
      total_tps_p5_low: 35,
      total_tps_median: 58,
      total_tps_trimmed_90: 60,
      total_tps_p95_high: 96,
      total_tps_avg: 61,
      total_input_tokens: 200_000,
      cache_read_tokens: 120_000,
      cache_write_tokens: 20_000,
      cache_hit_call_count: 60,
      cache_hit_call_ratio: 61.22,
      weighted_cache_rate: 60.0,
      cache_rate_p5_low: 0.0,
      cache_rate_median: 48.5,
      cache_rate_p95_high: 90.0,
      cache_rate_avg: 46.2,
      hit_cache_rate_p5_low: 35.0,
      hit_cache_rate_median: 75.0,
      hit_cache_rate_p95_high: 92.5,
      hit_cache_rate_avg: 72.1,
      channels: [
        {
          channel_id: 11,
          channel_name: 'Anthropic Primary',
          channel_type: 14,
          channel_status: 1,
          model: 'claude-sonnet-4-5',
          request_count: 80,
          success_count: 79,
          error_count: 1,
          availability: 98.75,
          avg_prompt_tokens: 2100,
          avg_output_tokens: 520,
          avg_ttft_sec: 0.78,
          median_ttft_sec: 0.68,
          p95_ttft_sec: 1.25,
          prefill_tps_avg: 2600,
          prefill_tps_median: 2750,
          decode_tps_p5_low: 48,
          decode_tps_median: 72,
          decode_tps_trimmed_90: 74,
          decode_tps_p95_high: 115,
          decode_tps_avg: 75,
          total_tps_p5_low: 40,
          total_tps_median: 62,
          total_tps_trimmed_90: 64,
          total_tps_p95_high: 98,
          total_tps_avg: 65,
          total_input_tokens: 160_000,
          cache_read_tokens: 112_000,
          cache_write_tokens: 16_000,
          cache_hit_call_count: 55,
          cache_hit_call_ratio: 69.62,
          weighted_cache_rate: 70.0,
          cache_rate_p5_low: 10.0,
          cache_rate_median: 65.0,
          cache_rate_p95_high: 92.0,
          cache_rate_avg: 60.0,
          hit_cache_rate_p5_low: 40.0,
          hit_cache_rate_median: 78.0,
          hit_cache_rate_p95_high: 94.0,
          hit_cache_rate_avg: 76.0,
        },
      ],
    },
  ],
  channels: [
    {
      channel_id: 11,
      channel_name: 'Anthropic Primary',
      channel_type: 14,
      channel_status: 1,
      model_count: 1,
      request_count: 80,
      success_count: 79,
      error_count: 1,
      availability: 98.75,
      avg_prompt_tokens: 2100,
      avg_output_tokens: 520,
      avg_ttft_sec: 0.78,
      median_ttft_sec: 0.68,
      p95_ttft_sec: 1.25,
      prefill_tps_avg: 2600,
      prefill_tps_median: 2750,
      decode_tps_p5_low: 48,
      decode_tps_median: 72,
      decode_tps_trimmed_90: 74,
      decode_tps_p95_high: 115,
      decode_tps_avg: 75,
      total_tps_p5_low: 40,
      total_tps_median: 62,
      total_tps_trimmed_90: 64,
      total_tps_p95_high: 98,
      total_tps_avg: 65,
      total_input_tokens: 160_000,
      cache_read_tokens: 112_000,
      cache_write_tokens: 16_000,
      cache_hit_call_count: 55,
      cache_hit_call_ratio: 69.62,
      weighted_cache_rate: 70.0,
      cache_rate_p5_low: 10.0,
      cache_rate_median: 65.0,
      cache_rate_p95_high: 92.0,
      cache_rate_avg: 60.0,
      hit_cache_rate_p5_low: 40.0,
      hit_cache_rate_median: 78.0,
      hit_cache_rate_p95_high: 94.0,
      hit_cache_rate_avg: 76.0,
      models: [],
    },
  ],
  trend_granularity: 'hour',
  trend: [
    {
      timestamp: 1795651200,
      request_count: 100,
      success_count: 98,
      error_count: 2,
      availability: 98.0,
      avg_ttft_sec: 0.85,
      median_ttft_sec: 0.72,
      p95_ttft_sec: 1.45,
      decode_tps_p5_low: 42,
      decode_tps_trimmed_90: 70,
      decode_tps_p95_high: 112,
      weighted_cache_rate: 60.0,
      cache_rate_p5_low: 0.0,
      cache_rate_median: 48.5,
      cache_rate_p95_high: 90.0,
      hit_cache_rate_p5_low: 35.0,
      hit_cache_rate_median: 75.0,
      hit_cache_rate_p95_high: 92.5,
    },
  ],
  channel_trends: [
    {
      channel_id: 11,
      channel_name: 'Anthropic Primary',
      channel_type: 14,
      total_calls: 80,
      points: [
        {
          timestamp: 1795651200,
          request_count: 80,
          success_count: 79,
          error_count: 1,
          availability: 98.75,
          avg_ttft_sec: 0.78,
          median_ttft_sec: 0.68,
          p95_ttft_sec: 1.25,
          decode_tps_p5_low: 48,
          decode_tps_trimmed_90: 74,
          decode_tps_p95_high: 115,
          weighted_cache_rate: 70.0,
          cache_rate_p5_low: 10.0,
          cache_rate_median: 65.0,
          cache_rate_p95_high: 92.0,
          hit_cache_rate_p5_low: 40.0,
          hit_cache_rate_median: 78.0,
          hit_cache_rate_p95_high: 94.0,
        },
      ],
    },
  ],
}

async function renderHealthPanel(initialPath = '/dashboard/health') {
  vi.spyOn(channelsApi, 'getModelHealthAnalysis').mockResolvedValue({
    success: true,
    data: SAMPLE_HEALTH_PAYLOAD,
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: ModelHealthPanel }),
    history: createMemoryHistory({ initialEntries: [initialPath] }),
  })
  await router.load()

  return render(
    <ThemeProvider>
      <ThemeCustomizationProvider>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ThemeCustomizationProvider>
    </ThemeProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('ModelHealthPanel 3-subpage telemetry suite', () => {
  it('opens on Comprehensive Model Health by default and displays TTFT, Decode TPS, Cache Hit Rate, and trend charts', async () => {
    await renderHealthPanel('/dashboard/health')

    const overviewTab = await screen.findByRole('tab', {
      name: 'Comprehensive Model Health',
    })
    expect(overviewTab).toHaveAttribute('aria-selected', 'true')
    expect(
      await screen.findByText('Comprehensive Model & Channel Health Summary')
    ).toBeVisible()

    expect(screen.getAllByText('0.72s').length).toBeGreaterThan(0)
    expect(screen.getAllByText('60.0%').length).toBeGreaterThan(0)
    expect(await screen.findByTestId('vchart-mock')).toBeInTheDocument()
  })

  it('switches to Response Speed and Model Cache Analysis while sharing query cache and supporting P5 / P50 / P95 cache scope toggle', async () => {
    const user = userEvent.setup()
    await renderHealthPanel('/dashboard/health')

    await user.click(await screen.findByRole('tab', { name: 'Response Speed' }))
    expect(
      await screen.findByText('Model & Channel Response Speed Matrix')
    ).toBeVisible()
    expect(channelsApi.getModelHealthAnalysis).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('tab', { name: 'Model Cache Analysis' }))
    expect(
      await screen.findByText('Model & Channel Prompt Cache Matrix')
    ).toBeVisible()

    // Default scope ('all') shows P50 = 48.5%; toggling 'Cache-Hit Calls Only' shows P50 = 75.0%.
    expect(screen.getAllByText('48.5%').length).toBeGreaterThan(0)
    await user.click(screen.getByRole('tab', { name: 'Cache-Hit Calls Only' }))
    expect(screen.getAllByText('75.0%').length).toBeGreaterThan(0)

    // Expanding the model row reveals per-channel cache comparison.
    const table = screen.getByRole('table')
    const modelRow = within(table).getByText('claude-sonnet-4-5').closest('tr')
    expect(modelRow).not.toBeNull()
    if (modelRow) {
      await user.click(modelRow)
    }
    expect(
      await screen.findByText(/Same-Model Channel Cache Comparison/)
    ).toBeVisible()
    expect(screen.getByText('Anthropic Primary')).toBeVisible()
  })
})
