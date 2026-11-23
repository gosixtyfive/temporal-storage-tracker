import { FetchError } from 'ofetch'
import { describe, expect, it, vi } from 'vitest'
import { ApiError, createApi, type Fetcher, isPending, isTerminal, toApiError } from '~/utils/api'

const item = {
  id: 'item-1',
  memberName: 'Sam',
  itemName: 'Bike frame',
  description: '',
  storageDurationSeconds: 420,
  storedAt: '2026-11-02T09:00:00Z',
  reminderAt: '2026-11-02T09:06:00Z',
  expiresAt: '2026-11-02T09:07:00Z',
  status: 'Stored',
  endedAt: null,
  notifications: [],
}

/** A fetch failure shaped like ofetch's, which Nuxt's $fetch throws. */
function fetchError(status: number, data?: unknown, statusText = '') {
  const err = new FetchError(`[GET] "/api": ${status}`)
  Object.assign(err, { status, statusCode: status, statusText, data })
  return err
}

describe('createApi', () => {
  it('calls each endpoint with the right method, path and body', async () => {
    const calls: unknown[][] = []
    const fetcher = vi.fn(async (url: string, options?: unknown) => {
      calls.push([url, options])
      if (url === '/api/items' && (options as { method?: string } | undefined)?.method === 'POST') {
        return { id: 'item-1' }
      }
      if (url === '/api/items') {
        return { items: [item] }
      }
      return item
    }) as unknown as Fetcher
    const api = createApi(fetcher)

    expect(await api.createItem({ memberName: 'Sam', itemName: 'Bike frame' })).toBe('item-1')
    expect(await api.listItems()).toEqual([item])
    expect(await api.listItems('Expired')).toEqual([item])
    await api.getItem('item-1')
    await api.collect('item-1')
    await api.remove('item-1')
    await api.getConfig()

    expect(calls).toEqual([
      ['/api/items', { method: 'POST', body: { memberName: 'Sam', itemName: 'Bike frame' } }],
      ['/api/items', undefined],
      ['/api/items', { query: { status: 'Expired' } }],
      ['/api/items/item-1', undefined],
      ['/api/items/item-1/collect', { method: 'POST' }],
      ['/api/items/item-1/remove', { method: 'POST' }],
      ['/api/config', undefined],
    ])
  })

  it('encodes item IDs in paths', async () => {
    const fetcher = vi.fn(async () => item) as unknown as Fetcher
    await createApi(fetcher).getItem('a/b?c')
    expect(fetcher).toHaveBeenCalledWith('/api/items/a%2Fb%3Fc', undefined)
  })

  it('turns failures into ApiErrors', async () => {
    const fetcher = (async () => {
      throw fetchError(409, { error: 'item is already Collected' })
    }) as Fetcher
    const err = await createApi(fetcher).collect('item-1').catch(e => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 409, message: 'item is already Collected' })
  })
})

describe('toApiError', () => {
  it('uses the API error message and status', () => {
    expect(toApiError(fetchError(400, { error: 'itemName is required' })))
      .toMatchObject({ status: 400, message: 'itemName is required' })
  })

  it('falls back to the status text, then a generic message', () => {
    expect(toApiError(fetchError(502, '<html>bad gateway</html>', 'Bad Gateway')))
      .toMatchObject({ status: 502, message: 'Bad Gateway' })
    expect(toApiError(fetchError(500))).toMatchObject({ status: 500, message: 'Request failed (500)' })
  })

  it('reports an unreachable server as status 0', () => {
    expect(toApiError(new TypeError('fetch failed'))).toMatchObject({ status: 0, message: 'Could not reach the server' })
    expect(toApiError(undefined)).toMatchObject({ status: 0 })
  })

  it('passes ApiErrors through', () => {
    const err = new ApiError(404, 'item not found')
    expect(toApiError(err)).toBe(err)
  })
})

describe('isPending', () => {
  it('is true only for a 404 ApiError', () => {
    expect(isPending(new ApiError(404, 'item not found'))).toBe(true)
    expect(isPending(new ApiError(409, 'ended'))).toBe(false)
    expect(isPending(new Error('404'))).toBe(false)
  })
})

describe('isTerminal', () => {
  it('matches the backend terminal statuses', () => {
    expect(isTerminal('Collected')).toBe(true)
    expect(isTerminal('RemovedByAdmin')).toBe(true)
    expect(isTerminal('Stored')).toBe(false)
    expect(isTerminal('ExpiringSoon')).toBe(false)
    expect(isTerminal('Expired')).toBe(false)
  })
})
