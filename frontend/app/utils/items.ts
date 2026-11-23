// Pure dashboard logic: status presentation, time left, counts, filtering,
// and spotting new notifications between polls. Unit-tested in test/items.test.ts.
import { isTerminal, ITEM_STATUSES, type Item, type ItemNotification, type ItemStatus, type NotificationKind } from '~/utils/api'
import { formatDuration } from '~/utils/format'

/** The Nuxt UI semantic colors used for item states. */
export type StateColor = 'neutral' | 'primary' | 'warning' | 'error' | 'success' | 'info'

/**
 * Text color class per state color. Spelled out in full because Tailwind only
 * generates classes whose complete names appear in the source.
 */
export const TEXT_COLOR: Record<StateColor, string> = {
  neutral: 'text-muted',
  primary: 'text-primary',
  warning: 'text-warning',
  error: 'text-error',
  success: 'text-success',
  info: 'text-info',
}

export const STATUS_META: Record<ItemStatus, { label: string, color: StateColor, icon: string }> = {
  Stored: { label: 'Stored', color: 'primary', icon: 'i-lucide-package' },
  ExpiringSoon: { label: 'Expiring soon', color: 'warning', icon: 'i-lucide-clock-alert' },
  Expired: { label: 'Expired', color: 'error', icon: 'i-lucide-alarm-clock-off' },
  Collected: { label: 'Collected', color: 'success', icon: 'i-lucide-hand' },
  RemovedByAdmin: { label: 'Removed', color: 'neutral', icon: 'i-lucide-trash-2' },
}

export const NOTIFICATION_META: Record<NotificationKind, { label: string, color: StateColor, icon: string }> = {
  ExpiringSoon: { label: 'Reminder sent', color: 'warning', icon: 'i-lucide-bell' },
  Expired: { label: 'Expiry notice sent', color: 'error', icon: 'i-lucide-bell-ring' },
  RemovedByAdmin: { label: 'Removal notice sent', color: 'neutral', icon: 'i-lucide-bell-minus' },
}

export interface TimeLeft {
  /** Where the item is on the clock. Terminal items are 'ended' regardless of time. */
  phase: 'stored' | 'expiringSoon' | 'expired' | 'ended'
  /** For example "5 min 20 sec left", "Expired 2 min ago", "Collected 3 min ago". */
  label: string
  /** Share of the storage time used, 0 to 100. */
  progress: number
  color: StateColor
}

/**
 * Time left for an item at nowMs (the browser clock), from the timestamps the
 * workflow set. The countdown follows the clock, so it can briefly run ahead
 * of the status, which changes when the workflow's timer has fired and the
 * next poll has seen it.
 */
export function timeLeft(item: Item, nowMs: number): TimeLeft {
  const storedAt = Date.parse(item.storedAt)
  const reminderAt = Date.parse(item.reminderAt)
  const expiresAt = Date.parse(item.expiresAt)
  const progressAt = (t: number) => clamp(((t - storedAt) / Math.max(1, expiresAt - storedAt)) * 100, 0, 100)
  const ago = (t: number) => formatDuration(Math.max(0, nowMs - t) / 1000)

  if (isTerminal(item.status)) {
    const endedAt = item.endedAt ? Date.parse(item.endedAt) : nowMs
    return {
      phase: 'ended',
      label: `${STATUS_META[item.status].label} ${ago(endedAt)} ago`,
      progress: progressAt(endedAt),
      color: STATUS_META[item.status].color,
    }
  }
  if (nowMs >= expiresAt) {
    return { phase: 'expired', label: `Expired ${ago(expiresAt)} ago`, progress: 100, color: 'error' }
  }
  return {
    phase: nowMs >= reminderAt ? 'expiringSoon' : 'stored',
    label: `${formatDuration((expiresAt - nowMs) / 1000)} left`,
    progress: progressAt(nowMs),
    color: nowMs >= reminderAt ? 'warning' : 'primary',
  }
}

export type StatusFilter = ItemStatus | 'all'

export function countByStatus(items: readonly Item[]): Record<StatusFilter, number> {
  const counts = Object.fromEntries([['all', items.length], ...ITEM_STATUSES.map(s => [s, 0])]) as Record<StatusFilter, number>
  for (const item of items) {
    counts[item.status]++
  }
  return counts
}

export function filterItems(items: readonly Item[], filter: StatusFilter): Item[] {
  return filter === 'all' ? [...items] : items.filter(item => item.status === filter)
}

export interface NewNotification {
  item: Item
  notification: ItemNotification
}

/**
 * Notifications in next that prev did not have, for items prev already had.
 * Items seen for the first time are not announced: on the first load, or for
 * a just-added item, their notifications are not news.
 */
export function newNotifications(prev: readonly Item[] | undefined, next: readonly Item[]): NewNotification[] {
  if (!prev) {
    return []
  }
  const seen = new Map(prev.map(item => [item.id, new Set(item.notifications.map(n => n.kind))]))
  const out: NewNotification[] = []
  for (const item of next) {
    const kinds = seen.get(item.id)
    if (!kinds) {
      continue
    }
    for (const notification of item.notifications) {
      if (!kinds.has(notification.kind)) {
        out.push({ item, notification })
      }
    }
  }
  return out
}

/** Link to an item's workflow in the Temporal Web UI (the workflow ID is the item ID). */
export function temporalWorkflowUrl(baseUrl: string, namespace: string, itemId: string): string {
  return `${baseUrl.replace(/\/$/, '')}/namespaces/${encodeURIComponent(namespace)}/workflows/${encodeURIComponent(itemId)}`
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n))
}
