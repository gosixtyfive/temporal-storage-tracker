// Typed client for the Go API. Types mirror the JSON in
// docs/architecture.md#http-api (camelCase, RFC 3339 timestamps, seconds).

export type ItemStatus = 'Stored' | 'ExpiringSoon' | 'Expired' | 'Collected' | 'RemovedByAdmin'
export type NotificationKind = 'ExpiringSoon' | 'Expired' | 'RemovedByAdmin'

export const ITEM_STATUSES: readonly ItemStatus[] = ['Stored', 'ExpiringSoon', 'Expired', 'Collected', 'RemovedByAdmin']

export function isTerminal(status: ItemStatus): boolean {
  return status === 'Collected' || status === 'RemovedByAdmin'
}

export interface ItemNotification {
  kind: NotificationKind
  message: string
  sentAt: string
}

export interface Item {
  id: string
  memberName: string
  itemName: string
  description: string
  storageDurationSeconds: number
  storedAt: string
  reminderAt: string
  expiresAt: string
  status: ItemStatus
  endedAt: string | null
  notifications: ItemNotification[]
}

export interface AppConfig {
  storageDurationSeconds: number
  reminderLeadSeconds: number
}

export interface NewItem {
  memberName: string
  itemName: string
  description?: string
}

/** An API failure. status is the HTTP status, or 0 if the server was unreachable. */
export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * A just-stored item is written by its workflow's first step, so the API
 * answers 404 until then. The status view shows such an item as pending.
 */
export function isPending(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404
}

export interface FetchOptions {
  method?: 'GET' | 'POST'
  body?: unknown
  query?: Record<string, string>
}

/** The request function the client uses: Nuxt's $fetch in the app, a fake in tests. */
export type Fetcher = <T>(url: string, options?: FetchOptions) => Promise<T>

export function createApi(fetcher: Fetcher) {
  async function call<T>(url: string, options?: FetchOptions): Promise<T> {
    try {
      return await fetcher<T>(url, options)
    }
    catch (err) {
      throw toApiError(err)
    }
  }
  const itemPath = (id: string) => `/api/items/${encodeURIComponent(id)}`

  return {
    getConfig: () => call<AppConfig>('/api/config'),
    listItems: async (status?: ItemStatus) =>
      (await call<{ items: Item[] }>('/api/items', status ? { query: { status } } : undefined)).items,
    getItem: (id: string) => call<Item>(itemPath(id)),
    createItem: async (input: NewItem) =>
      (await call<{ id: string }>('/api/items', { method: 'POST', body: input })).id,
    collect: (id: string) => call<Item>(`${itemPath(id)}/collect`, { method: 'POST' }),
    remove: (id: string) => call<Item>(`${itemPath(id)}/remove`, { method: 'POST' }),
  }
}

export type Api = ReturnType<typeof createApi>

/**
 * Converts a fetch failure into an ApiError. The Go API answers errors as
 * {"error": "message"}; ofetch (behind $fetch) exposes the status and parsed
 * body on the thrown error.
 */
export function toApiError(err: unknown): ApiError {
  if (err instanceof ApiError) {
    return err
  }
  const e = (err ?? {}) as { status?: unknown, statusCode?: unknown, statusText?: unknown, data?: unknown, message?: unknown }
  const status = typeof e.status === 'number' ? e.status : typeof e.statusCode === 'number' ? e.statusCode : 0
  if (status === 0) {
    return new ApiError(0, 'Could not reach the server')
  }
  const data = e.data as { error?: unknown } | undefined
  if (typeof data?.error === 'string' && data.error) {
    return new ApiError(status, data.error)
  }
  if (typeof e.statusText === 'string' && e.statusText) {
    return new ApiError(status, e.statusText)
  }
  return new ApiError(status, `Request failed (${status})`)
}
