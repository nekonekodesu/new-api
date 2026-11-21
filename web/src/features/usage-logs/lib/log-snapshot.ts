import type { GetLogsResponse } from '../types'
import { parseLogOther } from './format'

type LogPageData = NonNullable<GetLogsResponse['data']>

function isInProgressRow(row: LogPageData['items'][number]): boolean {
  return (
    'other' in row &&
    typeof row.other === 'string' &&
    Boolean(parseLogOther(row.other)?.in_progress)
  )
}

export function hasInProgressUsageLogs(
  page: LogPageData | undefined
): boolean {
  if (!page || page.items.length === 0) return false
  return page.items.some(isInProgressRow)
}

function comparePersistedLogsDesc(
  a: LogPageData['items'][number],
  b: LogPageData['items'][number]
): number {
  const aCreated =
    'created_at' in a && typeof a.created_at === 'number' ? a.created_at : 0
  const bCreated =
    'created_at' in b && typeof b.created_at === 'number' ? b.created_at : 0
  if (bCreated !== aCreated) {
    return bCreated - aCreated
  }
  const aId = typeof a.id === 'number' ? a.id : 0
  const bId = typeof b.id === 'number' ? b.id : 0
  return bId - aId
}

export function extractAdminDeltaCursor(
  page: LogPageData | undefined
): { createdAt: number; id: number } | undefined {
  if (!page || page.items.length === 0) return undefined
  let newest: { createdAt: number; id: number } | undefined
  for (const row of page.items) {
    if (
      !isInProgressRow(row) &&
      'created_at' in row &&
      typeof row.created_at === 'number' &&
      row.created_at > 0 &&
      typeof row.id === 'number' &&
      row.id > 0
    ) {
      if (
        !newest ||
        row.created_at > newest.createdAt ||
        (row.created_at === newest.createdAt && row.id > newest.id)
      ) {
        newest = { createdAt: row.created_at, id: row.id }
      }
    }
  }
  return newest
}

export function reconcileUsageLogSnapshot(page: LogPageData): LogPageData {
  const completed = new Set<string>()
  for (const row of page.items) {
    if (
      'request_id' in row &&
      row.request_id &&
      !parseLogOther(row.other)?.in_progress
    ) {
      completed.add(JSON.stringify([row.user_id, row.request_id]))
    }
  }
  const items = page.items.filter(
    (row) =>
      !(
        'request_id' in row &&
        row.request_id &&
        parseLogOther(row.other)?.in_progress &&
        completed.has(JSON.stringify([row.user_id, row.request_id]))
      )
  ) as typeof page.items
  return {
    ...page,
    items,
    total: Math.max(0, page.total - (page.items.length - items.length)),
  }
}

export function mergeUsageLogDelta(
  previous: LogPageData,
  delta: LogPageData,
  pageSize: number
): LogPageData {
  const prevActiveCount = previous.items.filter(isInProgressRow).length
  const prevPersisted = previous.items.filter((row) => !isInProgressRow(row))
  const prevPersistedTotal = Math.max(0, previous.total - prevActiveCount)

  const deltaActive = delta.items.filter(isInProgressRow)
  const deltaPersisted = delta.items.filter((row) => !isInProgressRow(row))

  const existingIds = new Set<number>()
  for (const row of prevPersisted) {
    if (typeof row.id === 'number' && row.id > 0) {
      existingIds.add(row.id)
    }
  }

  const newlyAddedPersisted = deltaPersisted.filter((row) => {
    if (typeof row.id !== 'number' || row.id <= 0) return true
    if (existingIds.has(row.id)) return false
    existingIds.add(row.id)
    return true
  })

  const mergedPersisted = [...newlyAddedPersisted, ...prevPersisted].sort(
    comparePersistedLogsDesc
  )

  const combined = [...deltaActive, ...mergedPersisted].slice(
    0,
    Math.max(pageSize, deltaActive.length)
  ) as LogPageData['items']

  return reconcileUsageLogSnapshot({
    ...previous,
    items: combined,
    total:
      prevPersistedTotal + newlyAddedPersisted.length + deltaActive.length,
  })
}
