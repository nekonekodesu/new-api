import { describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import {
  extractAdminDeltaCursor,
  mergeUsageLogDelta,
  reconcileUsageLogSnapshot,
} from '../log-snapshot'

const log = (id: number, request_id: string, quota: number): UsageLog => ({
  id,
  user_id: 1,
  created_at: id,
  type: 2,
  content: '',
  username: '',
  token_name: '',
  model_name: 'm',
  quota,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  is_stream: false,
  channel: 1,
  channel_name: '',
  token_id: 1,
  group: 'default',
  ip: '',
  other: '',
  request_id,
  upstream_request_id: '',
})

describe('usage log snapshots', () => {
  test('removes an active row when its completed request appears in the same snapshot', () => {
    const active = { ...log(-1, 'req-1', 0), other: '{"in_progress":true}' }
    const completed = log(1, 'req-1', 42)
    expect(
      reconcileUsageLogSnapshot({
        items: [active, completed],
        total: 2,
        page: 1,
        page_size: 20,
      })
    ).toEqual({ items: [completed], total: 1, page: 1, page_size: 20 })
  })
  test('preserves multiple persisted events for the same request', () => {
    const items = [log(1, 'req-1', 42), { ...log(2, 'req-1', 0), type: 5 }]
    expect(
      reconcileUsageLogSnapshot({ items, total: 2, page: 1, page_size: 20 })
        .items
    ).toEqual(items)
  })
  test('repeated snapshots do not increase the server total', () => {
    const snapshot = {
      items: [log(1, 'req-1', 42)],
      total: 500,
      page: 1,
      page_size: 20,
    }
    expect(
      reconcileUsageLogSnapshot(reconcileUsageLogSnapshot(snapshot))
    ).toEqual(snapshot)
  })
  test('an empty server snapshot stays empty', () => {
    expect(
      reconcileUsageLogSnapshot({ items: [], total: 0, page: 1, page_size: 20 })
        .items
    ).toEqual([])
  })
  test('identical request IDs belonging to different users do not hide active requests', () => {
    const items = [
      { ...log(-1, 'same', 0), user_id: 2, other: '{"in_progress":true}' },
      log(1, 'same', 42),
    ]
    expect(
      reconcileUsageLogSnapshot({ items, total: 2, page: 1, page_size: 20 })
        .items
    ).toEqual(items)
  })

  test('extracts newest persisted cursor while skipping in-progress rows', () => {
    const active = { ...log(-1, 'req-active', 0), other: '{"in_progress":true}' }
    const persisted = { ...log(42, 'req-42', 10), created_at: 1700000100 }
    expect(
      extractAdminDeltaCursor({
        items: [active, persisted],
        total: 100,
        page: 1,
        page_size: 100,
      })
    ).toEqual({ createdAt: 1700000100, id: 42 })
  })

  test('merges lightweight delta updates without duplicating rows or inflating total', () => {
    const activeOld = {
      ...log(-1, 'req-live', 0),
      completion_tokens: 10,
      other: '{"in_progress":true}',
    }
    const persisted1 = log(10, 'req-10', 5)
    const persisted2 = log(9, 'req-9', 5)
    const previous = {
      items: [activeOld, persisted1, persisted2],
      total: 301,
      page: 1,
      page_size: 3,
    }

    // Empty delta with updated active row keeps persisted total exact
    const activeUpdated = {
      ...log(-1, 'req-live', 0),
      completion_tokens: 55,
      other: '{"in_progress":true}',
    }
    const mergedLive = mergeUsageLogDelta(
      previous,
      { items: [activeUpdated], total: 1, page: 1, page_size: 3 },
      3
    )
    expect(mergedLive.items).toEqual([activeUpdated, persisted1, persisted2])
    expect(mergedLive.total).toBe(301)

    // Active row finishes and becomes persisted row id=11; trims to pageSize=3
    const completed11 = log(11, 'req-live', 25)
    const mergedDone = mergeUsageLogDelta(
      mergedLive,
      { items: [completed11], total: 1, page: 1, page_size: 3 },
      3
    )
    expect(mergedDone.items).toEqual([completed11, persisted1, persisted2])
    expect(mergedDone.total).toBe(301)

    // Multiple new rows arriving in one batch (e.g. after minimizing/restoring window),
    // even if ordered oldest-to-newest, must be sorted newest-first and advance cursor to the newest row.
    const batch12 = { ...log(12, 'req-12', 10), created_at: 1700000200 }
    const batch13 = { ...log(13, 'req-13', 15), created_at: 1700000300 }
    const batch14 = { ...log(14, 'req-14', 20), created_at: 1700000400 }
    const mergedBatch = mergeUsageLogDelta(
      mergedDone,
      {
        items: [batch12, batch13, batch14],
        total: 3,
        page: 1,
        page_size: 4,
      },
      4
    )
    expect(mergedBatch.items).toEqual([batch14, batch13, batch12, completed11])
    expect(extractAdminDeltaCursor(mergedBatch)).toEqual({
      createdAt: 1700000400,
      id: 14,
    })
  })
})
