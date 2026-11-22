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
/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useState, type ReactNode } from 'react'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ChannelAffinityInfo } from '../types'

export type LogsViewScope = 'all' | 'self'
export type LogsViewAccess = 'self' | 'admin' | 'root'

const AUTO_REFRESH_STORAGE_KEY = 'newapi.usage-logs.auto-refresh-seconds:v3'
export const DEFAULT_AUTO_REFRESH_SECONDS = 1
const MIN_AUTO_REFRESH_SECONDS = 0.5

function normalizeAutoRefreshSeconds(seconds: number): number {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return DEFAULT_AUTO_REFRESH_SECONDS
  }
  if (seconds === 0) {
    return 0
  }
  return Math.max(
    MIN_AUTO_REFRESH_SECONDS,
    Math.min(Math.round(seconds * 10) / 10, 3600)
  )
}

function readStoredAutoRefreshSeconds(): number {
  if (typeof window === 'undefined') return DEFAULT_AUTO_REFRESH_SECONDS
  try {
    const raw = window.localStorage.getItem(AUTO_REFRESH_STORAGE_KEY)
    if (raw === null) return DEFAULT_AUTO_REFRESH_SECONDS
    const parsed = Number(raw)
    if (!Number.isFinite(parsed) || parsed < 0) {
      return DEFAULT_AUTO_REFRESH_SECONDS
    }
    return normalizeAutoRefreshSeconds(parsed)
  } catch {
    return DEFAULT_AUTO_REFRESH_SECONDS
  }
}

export function resolveLogsViewAccess(
  role: number,
  viewScope: LogsViewScope
): LogsViewAccess {
  if (viewScope !== 'all' || role < ROLE.ADMIN) return 'self'
  return role === ROLE.SUPER_ADMIN ? 'root' : 'admin'
}

interface UsageLogsContextValue {
  selectedUserId: number | null
  setSelectedUserId: (userId: number | null) => void
  userInfoDialogOpen: boolean
  setUserInfoDialogOpen: (open: boolean) => void
  affinityTarget: ChannelAffinityInfo | null
  setAffinityTarget: (target: ChannelAffinityInfo | null) => void
  affinityDialogOpen: boolean
  setAffinityDialogOpen: (open: boolean) => void
  sensitiveVisible: boolean
  setSensitiveVisible: (visible: boolean) => void
  viewScope: LogsViewScope
  setViewScope: (scope: LogsViewScope) => void
  autoRefreshSeconds: number
  setAutoRefreshSeconds: (seconds: number) => void
}

const UsageLogsContext = createContext<UsageLogsContextValue | undefined>(
  undefined
)

export function UsageLogsProvider({ children }: { children: ReactNode }) {
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userInfoDialogOpen, setUserInfoDialogOpen] = useState(false)
  const [affinityTarget, setAffinityTarget] =
    useState<ChannelAffinityInfo | null>(null)
  const [affinityDialogOpen, setAffinityDialogOpen] = useState(false)
  const [sensitiveVisible, setSensitiveVisible] = useState(true)
  const [viewScope, setViewScope] = useState<LogsViewScope>('all')
  const [autoRefreshSeconds, setAutoRefreshSecondsState] = useState<number>(
    readStoredAutoRefreshSeconds
  )

  const setAutoRefreshSeconds = (seconds: number) => {
    const normalized = normalizeAutoRefreshSeconds(seconds)
    setAutoRefreshSecondsState(normalized)
    try {
      window.localStorage.setItem(AUTO_REFRESH_STORAGE_KEY, String(normalized))
    } catch {
      // Ignore storage write errors
    }
  }

  return (
    <UsageLogsContext.Provider
      value={{
        selectedUserId,
        setSelectedUserId,
        userInfoDialogOpen,
        setUserInfoDialogOpen,
        affinityTarget,
        setAffinityTarget,
        affinityDialogOpen,
        setAffinityDialogOpen,
        sensitiveVisible,
        setSensitiveVisible,
        viewScope,
        setViewScope,
        autoRefreshSeconds,
        setAutoRefreshSeconds,
      }}
    >
      {children}
    </UsageLogsContext.Provider>
  )
}

export function useUsageLogsContext() {
  const context = useContext(UsageLogsContext)
  if (!context) {
    throw new Error('useUsageLogsContext must be used within UsageLogsProvider')
  }
  return context
}

export function useOptionalUsageLogsContext() {
  return useContext(UsageLogsContext)
}

/**
 * Resolves the effective admin scope for usage logs: whether the current
 * user is allowed to view all users' logs (`canManageScope`), and whether
 * their current view preference (`viewScope`) has that scope active
 * (`isAdminView`). Data fetching and admin-only UI should key off
 * `isAdminView` rather than raw role, so an admin who switches to "only
 * mine" is treated exactly like a regular user for that view.
 */
export function useLogsViewScope() {
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const { viewScope, setViewScope } = useUsageLogsContext()
  const canManageScope = role >= ROLE.ADMIN
  const viewAccess = resolveLogsViewAccess(role, viewScope)
  const isAdminView = viewAccess !== 'self'
  const isRootView = viewAccess === 'root'

  return {
    canManageScope,
    viewScope,
    setViewScope,
    isAdminView,
    isRootView,
    viewAccess,
  }
}
