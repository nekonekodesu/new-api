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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { CronJob } from '../../api'
import { CronJobsPanel } from '../cron-jobs-panel'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

const sampleJobs: CronJob[] = [
  {
    id: 'job-1',
    name: 'Sync Models',
    description: 'Sync upstream model metadata',
    enabled: true,
    cron_expr: '*/15 * * * *',
    job_type: 'shell_script',
    script: 'echo sync-models-ok',
    http_method: 'GET',
    http_headers: '',
    http_body: '',
    working_dir: '/srv/app',
    timeout_sec: 90,
    built_in: false,
    last_run_at: 1700000000,
    last_status: 'success',
    last_output: 'Synced 12 models',
    last_duration_ms: 420,
    next_run_at: 1700000900,
    run_count: 1,
  },
  {
    id: 'job-2',
    name: 'Webhook Ping',
    description: 'Notify health endpoint',
    enabled: false,
    cron_expr: '0 * * * *',
    job_type: 'http_request',
    script: 'https://example.com/health',
    http_method: 'POST',
    http_headers: '{"X-Test":"1"}',
    http_body: '{"ok":true}',
    working_dir: '',
    timeout_sec: 30,
    built_in: false,
    last_run_at: 0,
    last_status: '',
    last_output: '',
    last_duration_ms: 0,
    next_run_at: 0,
    run_count: 0,
  },
]

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <CronJobsPanel />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('CronJobsPanel collapsible UI and trigger feedback', () => {
  beforeEach(() => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { jobs: sampleJobs } },
    })
  })

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders tasks collapsed by default and toggles individual row details with aria-expanded', async () => {
    const user = userEvent.setup()
    renderPanel()

    await screen.findByText('Sync Models')
    expect(screen.queryByText('echo sync-models-ok')).toBeNull()
    expect(screen.queryByText('Synced 12 models')).toBeNull()

    const expandButton = screen.getByRole('button', {
      name: 'Expand details Sync Models',
    })
    expect(expandButton.getAttribute('aria-expanded')).toBe('false')

    await user.click(expandButton)
    expect(expandButton.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByText('echo sync-models-ok')).toBeTruthy()
    expect(screen.getByText('Synced 12 models')).toBeTruthy()
    expect(screen.getByText('/srv/app')).toBeTruthy()

    await user.click(expandButton)
    expect(expandButton.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByText('echo sync-models-ok')).toBeNull()
  })

  it('supports expand all and collapse all across tasks', async () => {
    const user = userEvent.setup()
    renderPanel()

    await screen.findByText('Sync Models')
    const expandAllButton = screen.getByRole('button', { name: 'Expand All' })
    await user.click(expandAllButton)

    expect(screen.getByText('echo sync-models-ok')).toBeTruthy()
    expect(screen.getByText('https://example.com/health')).toBeTruthy()

    const collapseAllButton = screen.getByRole('button', {
      name: 'Collapse All',
    })
    await user.click(collapseAllButton)
    expect(screen.queryByText('echo sync-models-ok')).toBeNull()
    expect(screen.queryByText('https://example.com/health')).toBeNull()
  })

  it('auto-expands task details after running a task immediately', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          job: {
            ...sampleJobs[0],
            last_output: 'Fresh run completed',
            last_duration_ms: 150,
          },
        },
      },
    })

    renderPanel()
    await screen.findByText('Sync Models')

    const runButtons = screen.getAllByRole('button', { name: 'Run Now' })
    await user.click(runButtons[0])

    await waitFor(() => {
      expect(
        screen
          .getByRole('button', { name: 'Collapse details Sync Models' })
          .getAttribute('aria-expanded')
      ).toBe('true')
    })
  })
})
