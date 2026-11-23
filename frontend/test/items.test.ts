import { describe, expect, it } from 'vitest'
import type { Item, ItemStatus } from '~/utils/api'
import { countByStatus, filterItems, newNotifications, temporalWorkflowUrl, timeLeft } from '~/utils/items'

const t0 = Date.parse('2026-11-02T09:00:00Z')
const sec = 1000
const min = 60 * sec

// A 7-minute item: reminder at 6 min, expiry at 7 min.
function item(overrides: Partial<Item> = {}): Item {
  return {
    id: 'item-1',
    memberName: 'Sam',
    itemName: 'Bike frame',
    description: '',
    storageDurationSeconds: 420,
    storedAt: new Date(t0).toISOString(),
    reminderAt: new Date(t0 + 6 * min).toISOString(),
    expiresAt: new Date(t0 + 7 * min).toISOString(),
    status: 'Stored',
    endedAt: null,
    notifications: [],
    ...overrides,
  }
}

describe('timeLeft', () => {
  it('counts down before the reminder', () => {
    expect(timeLeft(item(), t0 + 90 * sec)).toEqual({
      phase: 'stored',
      label: '5 min 30 sec left',
      progress: (90 / 420) * 100,
      color: 'primary',
    })
  })

  it('switches to expiring soon at the reminder time', () => {
    expect(timeLeft(item(), t0 + 6 * min)).toMatchObject({ phase: 'expiringSoon', label: '1 min left', color: 'warning' })
    expect(timeLeft(item({ status: 'ExpiringSoon' }), t0 + 6 * min + 45 * sec))
      .toMatchObject({ phase: 'expiringSoon', label: '15 sec left' })
  })

  it('shows how long ago it expired, full progress', () => {
    expect(timeLeft(item({ status: 'Expired' }), t0 + 9 * min)).toEqual({
      phase: 'expired',
      label: 'Expired 2 min ago',
      progress: 100,
      color: 'error',
    })
    // The clock can pass expiry before the poll brings the Expired status.
    expect(timeLeft(item({ status: 'ExpiringSoon' }), t0 + 7 * min)).toMatchObject({ phase: 'expired', label: 'Expired 0 sec ago' })
  })

  it('shows how long ago an ended item ended, with progress frozen at that time', () => {
    const collected = item({ status: 'Collected', endedAt: new Date(t0 + 210 * sec).toISOString() })
    expect(timeLeft(collected, t0 + 7 * min)).toEqual({
      phase: 'ended',
      label: 'Collected 3 min 30 sec ago',
      progress: 50,
      color: 'success',
    })
    const removed = item({ status: 'RemovedByAdmin', endedAt: new Date(t0 + 10 * min).toISOString() })
    expect(timeLeft(removed, t0 + 11 * min)).toMatchObject({ phase: 'ended', label: 'Removed 1 min ago', progress: 100, color: 'neutral' })
  })

  it('clamps progress when the browser clock is behind the server', () => {
    expect(timeLeft(item(), t0 - 5 * sec)).toMatchObject({ progress: 0, label: '7 min 5 sec left' })
  })
})

describe('countByStatus and filterItems', () => {
  const items = (['Stored', 'Stored', 'Expired', 'Collected'] as ItemStatus[]).map((status, i) => item({ id: `item-${i}`, status }))

  it('counts every status, including zeros, and all', () => {
    expect(countByStatus(items)).toEqual({ all: 4, Stored: 2, ExpiringSoon: 0, Expired: 1, Collected: 1, RemovedByAdmin: 0 })
    expect(countByStatus([])).toMatchObject({ all: 0, Stored: 0 })
  })

  it('filters by status, keeping order', () => {
    expect(filterItems(items, 'Stored').map(i => i.id)).toEqual(['item-0', 'item-1'])
    expect(filterItems(items, 'RemovedByAdmin')).toEqual([])
    expect(filterItems(items, 'all')).toEqual(items)
    expect(filterItems(items, 'all')).not.toBe(items)
  })
})

describe('newNotifications', () => {
  const reminder = { kind: 'ExpiringSoon' as const, message: 'soon', sentAt: new Date(t0 + 6 * min).toISOString() }
  const expiry = { kind: 'Expired' as const, message: 'expired', sentAt: new Date(t0 + 7 * min).toISOString() }

  it('announces nothing on the first load', () => {
    expect(newNotifications(undefined, [item({ notifications: [reminder] })])).toEqual([])
  })

  it('finds notifications added since the previous poll', () => {
    const before = [item({ notifications: [reminder] }), item({ id: 'item-2' })]
    const afterItem = item({ status: 'Expired', notifications: [reminder, expiry] })
    const found = newNotifications(before, [afterItem, item({ id: 'item-2' })])
    expect(found).toEqual([{ item: afterItem, notification: expiry }])
  })

  it('does not announce notifications of items seen for the first time', () => {
    expect(newNotifications([], [item({ notifications: [reminder] })])).toEqual([])
  })
})

describe('temporalWorkflowUrl', () => {
  it('builds the Web UI link for the item\'s workflow', () => {
    expect(temporalWorkflowUrl('http://localhost:8233/', 'default', 'item-1'))
      .toBe('http://localhost:8233/namespaces/default/workflows/item-1')
  })
})
