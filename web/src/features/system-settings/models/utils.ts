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
import {
  extractJsonErrorPosition,
  formatJsonErrorDescription,
  formatJsonForEditor,
  normalizeJsonString as normalizeJsonStringShared,
} from '../utils/json-parser'

export function formatJsonForTextarea(value: string) {
  return formatJsonForEditor(value)
}

export function normalizeJsonString(value: string) {
  return normalizeJsonStringShared(value)
}

type JsonValidationOptions = {
  allowEmpty?: boolean
  predicate?: (value: unknown) => boolean
  predicateMessage?: string
}

export type JsonValidationError = {
  type: 'required' | 'structure' | 'syntax'
  line?: number
  column?: number
  position?: number
  missingCommaLine?: number
}

function buildSyntaxError(
  error: unknown,
  jsonString: string
): JsonValidationError {
  if (!(error instanceof Error)) {
    return {
      type: 'syntax',
    } satisfies JsonValidationError
  }

  return {
    type: 'syntax',
    ...extractJsonErrorPosition(error, jsonString),
  } satisfies JsonValidationError
}

export function validateJsonString(
  value: string,
  options: JsonValidationOptions = {}
) {
  const { allowEmpty = true, predicate, predicateMessage } = options
  const trimmed = value.trim()

  if (!trimmed) {
    return {
      valid: allowEmpty,
      message: allowEmpty ? undefined : 'Value is required',
      error: allowEmpty
        ? undefined
        : ({
            type: 'required',
          } satisfies JsonValidationError),
    }
  }

  try {
    const parsed = JSON.parse(trimmed)
    if (predicate && !predicate(parsed)) {
      return {
        valid: false,
        message: predicateMessage || 'JSON structure is invalid',
        error: {
          type: 'structure',
        } satisfies JsonValidationError,
      }
    }

    return { valid: true }
  } catch (error: unknown) {
    return {
      valid: false,
      message: formatJsonErrorDescription(error, trimmed) ?? 'Invalid JSON',
      error: buildSyntaxError(error, trimmed),
    }
  }
}
