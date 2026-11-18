import type { GetLogsResponse } from '../types'
import { parseLogOther } from './format'

// The current API renumbers self-log IDs and does not return a database cursor.
// Treat each response as an authoritative page until a cursor contract exists.
export function reconcileUsageLogSnapshot(
  page: NonNullable<GetLogsResponse['data']>
): NonNullable<GetLogsResponse['data']> {
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
