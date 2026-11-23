import type { ApiError, Item } from '~/utils/api'

interface Options {
  intervalMs?: number
  /** Called after each successful poll with the previous and new lists. */
  onUpdate?: (prev: Item[] | undefined, next: Item[]) => void
}

/**
 * Polls GET /api/items. Requests never overlap (the next one is scheduled
 * after the previous finishes), polling pauses while the tab is hidden and
 * refreshes as soon as it is visible again, and on errors the last list is
 * kept so the page does not go blank.
 */
export function usePolledItems({ intervalMs = 2000, onUpdate }: Options = {}) {
  const api = useApi()
  const items = ref<Item[]>()
  const error = ref<ApiError>()

  let timer: ReturnType<typeof setTimeout> | undefined
  let inFlight: Promise<void> | undefined
  let active = true

  async function poll() {
    try {
      const next = await api.listItems()
      const prev = items.value
      items.value = next
      error.value = undefined
      onUpdate?.(prev, next)
    }
    catch (err) {
      error.value = toApiError(err)
    }
  }

  /** Polls now (joining a request already in flight) and reschedules. */
  async function refresh() {
    clearTimeout(timer)
    inFlight ??= poll().finally(() => {
      inFlight = undefined
    })
    await inFlight
    schedule()
  }

  function schedule() {
    clearTimeout(timer)
    if (active && document.visibilityState === 'visible') {
      timer = setTimeout(refresh, intervalMs)
    }
  }

  /** Replaces one item in place, for example with an Update's response. */
  function replace(item: Item) {
    if (items.value) {
      items.value = items.value.map(it => (it.id === item.id ? item : it))
    }
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'visible') {
      void refresh()
    }
    else {
      clearTimeout(timer)
    }
  }

  onMounted(() => {
    document.addEventListener('visibilitychange', onVisibilityChange)
    void refresh()
  })
  onBeforeUnmount(() => {
    active = false
    clearTimeout(timer)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })

  return { items, error, refresh, replace }
}
