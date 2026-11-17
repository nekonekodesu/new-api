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
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

export interface AdminUpstreamErrorEntry {
  key: string
  attempt?: number
  channelId?: number
  status?: number
  errorCode?: string
  response: string
}

export interface UserFinalErrorEntry {
  status?: number
  errorType?: string
  errorCode?: string
  response: string
}

export function formatErrorResponseText(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) {
    return ''
  }
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (parsed !== null && typeof parsed === 'object') {
        return JSON.stringify(parsed, null, 2)
      }
    } catch {
      return trimmed
    }
  }
  return trimmed
}

export function resolveAdminUpstreamErrors(
  log: UsageLog,
  other: LogOtherData | null,
  fallbackDetails: string
): AdminUpstreamErrorEntry[] {
  const policyEvents = other?.admin_info?.request_policy ?? []
  const fromPolicy: AdminUpstreamErrorEntry[] = []

  for (let i = 0; i < policyEvents.length; i++) {
    const event = policyEvents[i]
    const formatted = event.upstream_error
      ? formatErrorResponseText(event.upstream_error)
      : ''
    if (!formatted) {
      continue
    }
    if (event.decision.action !== 'failure' && fromPolicy.length > 0) {
      continue
    }
    fromPolicy.push({
      key: `policy-${i}-${event.attempt}-${event.channel_id ?? 0}`,
      attempt: event.attempt > 0 ? event.attempt : undefined,
      channelId: event.channel_id,
      status: event.status,
      errorCode: event.error_code,
      response: formatted,
    })
  }

  if (fromPolicy.length > 0) {
    return fromPolicy
  }

  const directUpstream = other?.admin_info?.upstream_error_response
    ? formatErrorResponseText(other.admin_info.upstream_error_response)
    : ''
  if (directUpstream) {
    return [
      {
        key: 'admin-upstream-direct',
        channelId: log.channel > 0 ? log.channel : undefined,
        status: other?.status_code,
        errorCode: other?.error_code,
        response: directUpstream,
      },
    ]
  }

  if (log.type === 5) {
    const fallback = formatErrorResponseText(fallbackDetails)
    if (fallback) {
      return [
        {
          key: 'admin-upstream-fallback',
          channelId: log.channel > 0 ? log.channel : undefined,
          status: other?.status_code,
          errorCode: other?.error_code,
          response: fallback,
        },
      ]
    }
  }

  return []
}

export function resolveUserFinalError(
  log: UsageLog,
  other: LogOtherData | null,
  fallbackDetails: string
): UserFinalErrorEntry | null {
  const explicitFinal = other?.last_error_response
    ? formatErrorResponseText(other.last_error_response)
    : ''
  if (explicitFinal) {
    return {
      status: other?.status_code,
      errorType: other?.error_type,
      errorCode: other?.error_code,
      response: explicitFinal,
    }
  }

  if (log.type === 5) {
    const fallback = formatErrorResponseText(fallbackDetails)
    if (fallback) {
      return {
        status: other?.status_code,
        errorType: other?.error_type,
        errorCode: other?.error_code,
        response: fallback,
      }
    }
  }

  if (other?.stream_status && other.stream_status.status !== 'ok') {
    const streamErrors = Array.isArray(other.stream_status.errors)
      ? other.stream_status.errors
      : []
    const lastStreamError =
      other.stream_status.end_error || streamErrors.at(-1) || ''
    const formatted = formatErrorResponseText(lastStreamError)
    if (formatted) {
      return {
        response: formatted,
      }
    }
  }

  return null
}
