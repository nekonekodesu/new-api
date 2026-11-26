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
export type HealthSubTab = 'overview' | 'speed' | 'cache'
export type HealthEntityViewMode = 'models' | 'channels'
export type CachePercentileScope = 'all' | 'hit'

export function normalizeHealthSubTab(
  raw: string | null | undefined
): HealthSubTab {
  if (raw === 'speed' || raw === 'cache') {
    return raw
  }
  return 'overview'
}

export function formatTpsValue(val: number): string {
  if (!Number.isFinite(val) || val <= 0) {
    return '-'
  }
  if (val >= 1000) {
    return `${(val / 1000).toFixed(1)}k`
  }
  if (val >= 100) {
    return val.toFixed(0)
  }
  return val.toFixed(1)
}

export function formatTtftSec(val: number): string {
  if (!Number.isFinite(val) || val <= 0) {
    return '-'
  }
  return `${val.toFixed(2)}s`
}

export function formatPercentValue(val: number, digits = 1): string {
  if (!Number.isFinite(val) || val < 0) {
    return '0.0%'
  }
  return `${Math.min(val, 100).toFixed(digits)}%`
}

export function getAvailabilityBadgeClass(avail: number): string {
  if (avail >= 99) {
    return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
  }
  if (avail >= 90) {
    return 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400'
  }
  return 'border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400'
}

export function getCacheRateBadgeClass(rate: number): string {
  if (rate >= 50) {
    return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
  }
  if (rate >= 20) {
    return 'border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400'
  }
  if (rate > 0) {
    return 'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400'
  }
  return 'border-border bg-muted/40 text-muted-foreground'
}
