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
  formatJsonErrorDescription,
  formatJsonForEditor as formatJsonForEditorShared,
  normalizeJsonString,
} from '../utils/json-parser'

export function removeTrailingSlash(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ''
  return trimmed.replace(/\/+$/, '')
}

export function formatJsonForEditor(value: string) {
  return formatJsonForEditorShared(value, { trimOnError: true })
}

export function normalizeJsonForComparison(value: string) {
  return normalizeJsonString(value)
}

export function isValidJson(
  value: string,
  predicate?: (parsed: unknown) => boolean
): boolean {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    const parsed = JSON.parse(trimmed)
    if (predicate && !predicate(parsed)) {
      return false
    }
    return true
  } catch {
    return false
  }
}

export function getJsonError(
  value: string,
  predicate?: (parsed: unknown) => boolean
): string | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  try {
    const parsed = JSON.parse(trimmed)
    if (predicate && !predicate(parsed)) {
      return 'JSON structure is invalid'
    }
    return null
  } catch (error) {
    return formatJsonErrorDescription(error, trimmed) ?? 'Invalid JSON'
  }
}
