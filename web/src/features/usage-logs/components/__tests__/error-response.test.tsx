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
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next, { createInstance, type i18n } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import * as copyUtil from '@/lib/copy-to-clipboard'

import type { UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import { DetailsDialog } from '../dialogs/details-dialog'

const queryClients: QueryClient[] = []

function makeLog(overrides: Partial<UsageLog>, other: LogOtherData): UsageLog {
  return {
    id: 1,
    user_id: 7,
    created_at: 1_700_000_000,
    type: 5,
    content: 'status_code=502, bad response status code 502',
    username: 'user',
    token_name: 'token',
    model_name: 'gpt-5',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 1,
    is_stream: false,
    channel: 16,
    channel_name: 'upstream-primary',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-403-retry',
    upstream_request_id: '',
    ...overrides,
  }
}

function renderDetails(
  log: UsageLog,
  isAdmin: boolean,
  translations: i18n = i18next
): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClients.push(queryClient)

  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={translations}>
        <DetailsDialog
          log={log}
          isAdmin={isAdmin}
          isRoot={false}
          open
          onOpenChange={() => undefined}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  for (const queryClient of queryClients) {
    queryClient.clear()
  }
  queryClients.length = 0
})

describe('usage log upstream and final error responses', () => {
  test('shows measured request body size between IP and response time', () => {
    renderDetails(
      makeLog({ ip: '192.0.2.1' }, { request_body_size: 406062 }),
      true
    )
    const ip = screen.getByText('IP Address')
    const size = screen.getByText('Request Body Size')
    const timing = screen.getByText('Response Time')
    expect(
      ip.compareDocumentPosition(size) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      size.compareDocumentPosition(timing) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByText(/406,062 bytes/)).toBeVisible()
  })

  test('does not invent a request body size for historical logs', () => {
    renderDetails(makeLog({}, {}), true)
    expect(screen.queryByText('Request Body Size')).not.toBeInTheDocument()
  })

  test.each([
    ['en', '406,062 bytes'],
    ['zhCN', '406,062 bytes'],
    ['zhTW', '406,062 bytes'],
    ['fr', '406 062 bytes'],
    ['ja', '406,062 bytes'],
    ['ru', '406 062 bytes'],
    ['vi', '406.062 bytes'],
    ['invalid_locale', '406,062 bytes'],
  ])(
    'updates body size formatting after switching to %s',
    async (language, expected) => {
      const translations = createInstance()
      const resource = { translation: { '{{bytes}} bytes': '{{bytes}} bytes' } }
      await translations.init({
        lng: 'en',
        fallbackLng: 'en',
        resources: { en: resource, [language]: resource },
      })
      renderDetails(
        makeLog({}, { request_body_size: 406062 }),
        true,
        translations
      )
      expect(screen.getByText('406,062 bytes')).toBeVisible()
      await act(() => translations.changeLanguage(language))
      expect(screen.getByText(expected)).toBeVisible()
    }
  )

  test('shows upstream error responses to admins on a retried consume log', () => {
    const retriedConsumeLog = makeLog(
      {
        type: 2,
        channel: 57,
        channel_name: 'NearToken-Exit',
        content: '',
        prompt_tokens: 120,
        completion_tokens: 30,
      },
      {
        admin_info: {
          use_channel: [16, 57],
          request_policy: [
            {
              attempt: 1,
              channel_id: 16,
              group: 'default',
              elapsed_ms: 18,
              decision: {
                action: 'attempt',
                reason: 'channel_selected',
                source: 'routing',
              },
            },
            {
              attempt: 1,
              channel_id: 16,
              group: 'default',
              status: 403,
              error_code: 'account_deactivated',
              error_source: 'upstream',
              upstream_error:
                '{"error":{"message":"Upstream account suspended","code":"account_deactivated"}}',
              elapsed_ms: 1349,
              decision: {
                action: 'failure',
                reason: 'upstream_failure',
                source: 'upstream',
              },
            },
            {
              attempt: 1,
              channel_id: 16,
              group: 'default',
              status: 403,
              elapsed_ms: 1349,
              decision: {
                action: 'retry',
                reason: 'retry_status_matched',
                source: 'global',
              },
              health: 'unchanged',
            },
            {
              attempt: 2,
              channel_id: 57,
              group: 'default',
              elapsed_ms: 84470,
              decision: {
                action: 'success',
                reason: 'request_completed',
                source: 'upstream',
              },
            },
          ],
        },
      }
    )

    renderDetails(retriedConsumeLog, true)

    expect(screen.getByText('Upstream Error Response')).toBeInTheDocument()
    expect(screen.getByText('#16')).toBeInTheDocument()
    expect(screen.getByText('HTTP 403')).toBeInTheDocument()
    expect(screen.getByText('account_deactivated')).toBeInTheDocument()
    expect(screen.getByText('View upstream error response')).toBeInTheDocument()
    expect(
      screen.getAllByText(/Upstream account suspended/).length
    ).toBeGreaterThanOrEqual(1)
    expect(screen.queryByText('Final Error Response')).toBeNull()
  })

  test('does not show error panels to regular users when a retried consume log succeeded', () => {
    const succeededConsumeLog = makeLog(
      {
        type: 2,
        channel: 57,
        channel_name: '',
        content: '',
        prompt_tokens: 120,
        completion_tokens: 30,
      },
      {
        request_path: '/v1/responses',
      }
    )

    renderDetails(succeededConsumeLog, false)

    expect(screen.queryByText('Upstream Error Response')).toBeNull()
    expect(screen.queryByText('Final Error Response')).toBeNull()
  })

  test('shows sanitized final error response to regular users on failed logs without leaking admin upstream response', async () => {
    vi.spyOn(copyUtil, 'copyToClipboard').mockResolvedValue(true)

    const failedLog = makeLog(
      {
        type: 5,
        channel_name: '',
      },
      {
        status_code: 502,
        error_type: 'openai_error',
        error_code: 'bad_response_status_code',
        last_error_response:
          '{"error":{"message":"Upstream service unavailable (request id: req-403-retry)","type":"openai_error","code":"bad_response_status_code"}}',
        admin_info: {
          upstream_error_response:
            '<html>502 Bad Gateway from internal-node-9</html>',
        },
      }
    )

    renderDetails(failedLog, false)

    expect(screen.getByText('Final Error Response')).toBeInTheDocument()
    expect(screen.getByText('HTTP 502')).toBeInTheDocument()
    expect(screen.getByText('bad_response_status_code')).toBeInTheDocument()
    expect(screen.getByText(/Upstream service unavailable/)).toBeInTheDocument()
    expect(screen.queryByText('Upstream Error Response')).toBeNull()
    expect(screen.queryByText(/internal-node-9/)).toBeNull()

    const copyButtons = screen.getAllByRole('button', {
      name: 'Copy to clipboard',
    })
    await userEvent.click(copyButtons[0])
    expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument()
  })

  test('shows final error response to regular users when a stream ended with an error', () => {
    const streamErrorLog = makeLog(
      {
        type: 2,
        is_stream: true,
        channel_name: '',
        content: '',
      },
      {
        stream_status: {
          status: 'error',
          end_reason: 'scanner_error',
          end_error: 'unexpected EOF while reading stream',
        },
      }
    )

    renderDetails(streamErrorLog, false)

    expect(screen.getByText('Final Error Response')).toBeInTheDocument()
    expect(
      screen.getAllByText(/unexpected EOF while reading stream/).length
    ).toBeGreaterThanOrEqual(1)
  })
})
