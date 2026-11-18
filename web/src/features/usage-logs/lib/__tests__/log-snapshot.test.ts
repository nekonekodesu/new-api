import { describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { reconcileUsageLogSnapshot } from '../log-snapshot'

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
})
