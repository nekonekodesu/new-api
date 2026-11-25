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
import { describe, expect, it } from 'vitest'

import { formatBinaryBytes, formatBytes } from '../format'

describe('byte size formatters', () => {
  it('formats decimal bytes with 1000-based units in formatBytes', () => {
    expect(formatBytes(0, 'en-US')).toBe('0 B')
    expect(formatBytes(1000, 'en-US')).toBe('1 KB')
    expect(formatBytes(1500 * 1000, 'en-US')).toBe('1.5 MB')
  })

  it('formats binary bytes with 1024-based units in formatBinaryBytes', () => {
    expect(formatBinaryBytes(undefined)).toBe('-')
    expect(formatBinaryBytes(null)).toBe('-')
    expect(formatBinaryBytes(Number.NaN)).toBe('-')
    expect(formatBinaryBytes(0)).toBe('0 B')
    expect(formatBinaryBytes(-1024)).toBe('-1 KB')
    expect(formatBinaryBytes(1024)).toBe('1 KB')
    expect(formatBinaryBytes(1536, 2)).toBe('1.5 KB')
    expect(formatBinaryBytes(1024 * 1024 * 1024 * 1.25, 2)).toBe('1.25 GB')
  })
})
