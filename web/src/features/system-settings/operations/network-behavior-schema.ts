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
import * as z from 'zod'

const ipv4Segment = '(?:25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)'
const ipv4Regex = new RegExp(
  `^${ipv4Segment}\\.${ipv4Segment}\\.${ipv4Segment}\\.${ipv4Segment}$`
)

function isValidPort(portText: string): boolean {
  if (!/^\d{1,5}$/.test(portText)) {
    return false
  }
  const port = Number(portText)
  return Number.isInteger(port) && port >= 1 && port <= 65535
}

function isUnspecifiedIPv6(host: string): boolean {
  const cleaned = host.replaceAll(':', '')
  return cleaned.length === 0 || /^0+$/.test(cleaned)
}

function isValidIPv6(host: string): boolean {
  if (!host.includes(':')) {
    return false
  }
  if (!/^[0-9a-fA-F:]+$/.test(host)) {
    return false
  }
  return !isUnspecifiedIPv6(host)
}

function isValidIPv4(host: string): boolean {
  return ipv4Regex.test(host) && host !== '0.0.0.0'
}

export function isValidDnsServerEndpoint(raw: string): boolean {
  const trimmed = raw.trim()
  if (!trimmed) {
    return true
  }

  if (trimmed.startsWith('[')) {
    const closeIdx = trimmed.indexOf(']')
    if (closeIdx <= 1) {
      return false
    }
    const ipv6Host = trimmed.slice(1, closeIdx)
    if (!isValidIPv6(ipv6Host)) {
      return false
    }
    const rest = trimmed.slice(closeIdx + 1)
    if (!rest) {
      return true
    }
    if (!rest.startsWith(':')) {
      return false
    }
    return isValidPort(rest.slice(1))
  }

  const firstColon = trimmed.indexOf(':')
  const lastColon = trimmed.lastIndexOf(':')

  if (firstColon !== -1 && firstColon !== lastColon) {
    return isValidIPv6(trimmed)
  }

  if (firstColon !== -1) {
    const host = trimmed.slice(0, firstColon)
    const portText = trimmed.slice(firstColon + 1)
    return isValidIPv4(host) && isValidPort(portText)
  }

  return isValidIPv4(trimmed)
}

function isValidRegexSyntax(expr: string): boolean {
  if (!expr) {
    return false
  }
  const jsCompatible = expr
    .replaceAll(/\(\?[a-zA-Z-]+\)/g, '')
    .replaceAll(/\(\?[a-zA-Z-]+:/g, '(?:')
    .replaceAll('(?P<', '(?<')
  try {
    new RegExp(jsCompatible)
    return true
  } catch {
    return false
  }
}

export function isValidPathScanRuleLine(rawLine: string): boolean {
  const line = rawLine.trim()
  if (!line) {
    return true
  }
  if (line.length > 2048) {
    return false
  }
  if (line.startsWith('~*')) {
    return isValidRegexSyntax(line.slice(2).trim())
  }
  if (line.startsWith('~')) {
    return isValidRegexSyntax(line.slice(1).trim())
  }
  if (line.toLowerCase().startsWith('regex:')) {
    return isValidRegexSyntax(line.slice(6).trim())
  }

  const hasRegexMeta = /[()[\]|\\^$+{}]/.test(line)
  if ((line.startsWith('/') || line.startsWith('*')) && !hasRegexMeta) {
    return !/[?#]/.test(line)
  }

  if (line.startsWith('/') && hasRegexMeta) {
    let expr = line
    if (line.endsWith('/i') && line.length > 3) {
      expr = line.slice(1, -2)
    } else if (
      line.endsWith('/') &&
      line.length > 2 &&
      (line.startsWith('/^') || line.startsWith('/\\') || line.endsWith('$/'))
    ) {
      expr = line.slice(1, -1)
    }
    return isValidRegexSyntax(expr)
  }

  if (hasRegexMeta || line.includes('.*')) {
    return isValidRegexSyntax(line)
  }

  return false
}

export function isValidPathScanRulesText(raw: string): boolean {
  const lines = raw.split('\n')
  let nonEmptyCount = 0
  for (const line of lines) {
    if (!line.trim()) {
      continue
    }
    nonEmptyCount += 1
    if (nonEmptyCount > 500) {
      return false
    }
    if (!isValidPathScanRuleLine(line)) {
      return false
    }
  }
  return true
}

export const createNetworkBehaviorSchema = (t: (key: string) => string) => {
  const dnsFieldSchema = z
    .string()
    .refine(
      isValidDnsServerEndpoint,
      t('DNS server must be a valid IPv4 or IPv6 address (with optional :port)')
    )

  const pathRulesFieldSchema = z
    .string()
    .refine(
      isValidPathScanRulesText,
      t(
        'Each path scan rule must start with / (supports * wildcard) or be a valid regular expression'
      )
    )

  return z.object({
    network_setting: z.object({
      primary_dns: dnsFieldSchema,
      fallback_dns: dnsFieldSchema,
    }),
    path_scan_setting: z.object({
      path_scan_enabled: z.boolean(),
      path_scan_window_seconds: z
        .number()
        .int()
        .min(1)
        .max(86400, t('Path scan window cannot exceed one day')),
      path_scan_max_requests: z
        .number()
        .int()
        .min(1)
        .max(100000, t('Path scan request limit cannot exceed 100000')),
      path_scan_ban_duration: z
        .number()
        .int()
        .min(1)
        .max(36500, t('Path scan ban duration cannot exceed 36500')),
      path_scan_ban_duration_unit: z.enum([
        'second',
        'minute',
        'hour',
        'day',
        'year',
      ]),
      path_scan_direct_paths: pathRulesFieldSchema,
      path_scan_counted_paths: pathRulesFieldSchema,
    }),
  })
}

export type NetworkBehaviorFormValues = z.infer<
  ReturnType<typeof createNetworkBehaviorSchema>
>
