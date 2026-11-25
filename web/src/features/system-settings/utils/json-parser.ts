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
export type JsonParseResult<T> =
  | { success: true; data: T }
  | { success: false; error: string }

export interface SafeJsonParseOptions<T> {
  fallback?: T
  silent?: boolean
  context?: string
}

export interface SafeJsonParseWithValidationOptions<T> {
  fallback: T
  validator: (data: unknown) => data is T
  validatorMessage?: string
  context?: string
  silent?: boolean
}

export interface JsonErrorPosition {
  line?: number
  column?: number
  position?: number
  missingCommaLine?: number
}

export function extractJsonErrorPosition(
  error: unknown,
  jsonString: string
): JsonErrorPosition {
  if (!(error instanceof Error)) return {}

  const message = error.message
  const isMissingCommaError =
    message.includes("Expected ','") ||
    message.includes('Expected property name') ||
    message.includes('Unexpected string')

  const withHint = (pos: JsonErrorPosition): JsonErrorPosition => ({
    ...pos,
    missingCommaLine:
      isMissingCommaError && pos.line && pos.line > 1
        ? pos.line - 1
        : undefined,
  })

  // Try to extract position from error message
  // Format 1: "Unexpected token } in JSON at position 15"
  const positionMatch = message.match(/at position (\d+)/i)
  if (positionMatch) {
    const position = parseInt(positionMatch[1], 10)
    const { line, column } = getLineAndColumn(jsonString, position)
    return withHint({ line, column, position })
  }

  // Format 2: "JSON.parse: ... at line 2 column 3"
  const lineColMatch = message.match(/at line (\d+) column (\d+)/i)
  if (lineColMatch) {
    return withHint({
      line: parseInt(lineColMatch[1], 10),
      column: parseInt(lineColMatch[2], 10),
    })
  }

  return {}
}

function getLineAndColumn(
  text: string,
  position: number
): { line: number; column: number } {
  const lines = text.substring(0, position).split('\n')
  return {
    line: lines.length,
    column: lines[lines.length - 1].length + 1,
  }
}

export function formatJsonErrorDescription(
  error: unknown,
  jsonString: string
): string | undefined {
  if (!(error instanceof Error)) return undefined

  const position = extractJsonErrorPosition(error, jsonString)
  const message = error.message

  if (position.line && position.column) {
    const hint = position.missingCommaLine
      ? ` (check line ${position.missingCommaLine} for missing comma)`
      : ''
    return `Error at line ${position.line}, column ${position.column}: ${message}${hint}`
  }

  if (position.position !== undefined) {
    return `Error at position ${position.position}: ${message}`
  }

  return message
}

export function formatJsonForEditor(
  value: string,
  options: {
    emptyFallback?: string
    errorFallback?: string
    trimOnError?: boolean
    compact?: boolean
  } = {}
): string {
  const raw = value ?? ''
  const trimmed = raw.trim()
  const target = trimmed || options.emptyFallback || ''
  if (!target) {
    return ''
  }
  try {
    const parsed = JSON.parse(target)
    return options.compact
      ? JSON.stringify(parsed)
      : JSON.stringify(parsed, null, 2)
  } catch {
    if (options.errorFallback !== undefined) {
      return options.errorFallback
    }
    if (trimmed) {
      return options.trimOnError ? trimmed : raw
    }
    return options.trimOnError ? target.trim() : target
  }
}

export function normalizeJsonString(
  value: string,
  emptyFallback = '',
  trimOnError = true
): string {
  return formatJsonForEditor(value, {
    emptyFallback,
    trimOnError,
    compact: true,
  })
}

export function safeJsonParse<T = unknown>(
  value: string | undefined | null,
  options: SafeJsonParseOptions<T> = {}
): T {
  const { fallback, silent = false, context } = options

  if (!value || value.trim() === '') {
    return (fallback ?? null) as T
  }

  const trimmedValue = value.trim()

  try {
    return JSON.parse(trimmedValue) as T
  } catch (error) {
    // Log error for debugging in development
    if (import.meta.env.DEV && !silent) {
      const message = context
        ? `Failed to parse ${context}`
        : 'Invalid JSON format'
      const description = formatJsonErrorDescription(error, trimmedValue)
      // eslint-disable-next-line no-console
      console.error(`[JSON Parse Error] ${message}:`, description)
    }
    return (fallback ?? null) as T
  }
}

export function safeJsonParseWithValidation<T>(
  value: string | undefined | null,
  options: SafeJsonParseWithValidationOptions<T>
): T {
  const {
    fallback,
    validator,
    validatorMessage,
    context,
    silent = false,
  } = options
  const parsed = safeJsonParse(value, { fallback, silent: true, context })

  if (!validator(parsed)) {
    // Log error for debugging in development
    if (import.meta.env.DEV && !silent) {
      const message =
        validatorMessage ??
        (context ? `Invalid ${context} structure` : 'Invalid data structure')
      // eslint-disable-next-line no-console
      console.error(`[JSON Validation Error] ${message}`, { parsed })
    }
    return fallback
  }

  return parsed
}

export function tryJsonParse<T = unknown>(
  value: string | undefined | null
): JsonParseResult<T> {
  if (!value || value.trim() === '') {
    return { success: false, error: 'Empty value' }
  }

  try {
    const data = JSON.parse(value.trim()) as T
    return { success: true, data }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error',
    }
  }
}
