<script setup lang="ts">
import type { Item } from '~/utils/api'
import { countByStatus, filterItems, newNotifications, NOTIFICATION_META, type StatusFilter } from '~/utils/items'

useHead({ title: 'Dashboard' })

const api = useApi()
const toast = useToast()

const HIGHLIGHT_MS = 4000
const highlighted = ref(new Set<string>())

function highlight(id: string) {
  highlighted.value = new Set(highlighted.value).add(id)
  setTimeout(() => {
    const next = new Set(highlighted.value)
    next.delete(id)
    highlighted.value = next
  }, HIGHLIGHT_MS)
}

const { items, error, refresh, replace } = usePolledItems({
  // Announce what the workflows did since the last poll: reminders, expiry notices.
  onUpdate(prev, next) {
    for (const { item, notification } of newNotifications(prev, next)) {
      const meta = NOTIFICATION_META[notification.kind]
      toast.add({
        title: `${meta.label}: ${item.itemName}`,
        description: `To ${item.memberName}. ${notification.message}`,
        color: meta.color,
        icon: meta.icon,
      })
      highlight(item.id)
    }
  },
})

const filter = ref<StatusFilter>('all')
const counts = computed(() => countByStatus(items.value ?? []))
const shown = computed(() => filterItems(items.value ?? [], filter.value))

const busy = ref<Record<string, 'collect' | 'remove'>>({})

async function act(item: Item, action: 'collect' | 'remove') {
  busy.value = { ...busy.value, [item.id]: action }
  try {
    const updated = action === 'collect' ? await api.collect(item.id) : await api.remove(item.id)
    replace(updated)
    toast.add({
      title: action === 'collect' ? `Collected: ${item.itemName}` : `Removed: ${item.itemName}`,
      description: action === 'remove' ? `${item.memberName} has been notified.` : undefined,
      color: action === 'collect' ? 'success' : 'neutral',
      icon: action === 'collect' ? 'i-lucide-hand' : 'i-lucide-trash-2',
    })
  }
  catch (err) {
    const apiErr = toApiError(err)
    toast.add({
      title: `Could not ${action} ${item.itemName}`,
      description: isUnreachable(apiErr) ? 'Could not reach the server.' : apiErr.message,
      color: 'error',
      icon: 'i-lucide-circle-alert',
    })
    if (apiErr.status === 409) {
      await refresh()
    }
  }
  finally {
    const { [item.id]: _, ...rest } = busy.value
    busy.value = rest
  }
}
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-2">
      <h1 class="text-2xl font-semibold">
        Dashboard
      </h1>
      <StorageTiming />
    </div>

    <UAlert
      v-if="error && items"
      color="warning"
      variant="subtle"
      icon="i-lucide-wifi-off"
      :title="isUnreachable(error) ? 'Cannot reach the server; retrying' : `Could not refresh: ${error.message}; retrying`"
      description="Showing the last known state."
    />

    <template v-if="items">
      <StatusTiles v-model="filter" :counts="counts" />

      <UEmpty
        v-if="!items.length"
        icon="i-lucide-package-open"
        title="No items yet"
        description="Add an item to start its workflow. It will appear here within a couple of seconds."
        :actions="[{ label: 'Add item', icon: 'i-lucide-package-plus', to: '/add' }]"
      />
      <p v-else-if="!shown.length" class="py-8 text-center text-sm text-muted">
        No items with this status.
      </p>
      <div v-else class="grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(min(100%,20rem),1fr))]">
        <ItemCard
          v-for="item in shown"
          :key="item.id"
          :item="item"
          :highlighted="highlighted.has(item.id)"
          :busy="busy[item.id]"
          @collect="act($event, 'collect')"
          @remove="act($event, 'remove')"
        />
      </div>
    </template>

    <UAlert
      v-else-if="error"
      color="error"
      variant="subtle"
      icon="i-lucide-circle-alert"
      :title="isUnreachable(error) ? 'Cannot reach the server' : `Could not load items: ${error.message}`"
      :description="isUnreachable(error) ? 'Is the backend running (make server)? Retrying.' : 'Retrying.'"
    />
    <div v-else class="grid gap-4 sm:grid-cols-2">
      <USkeleton v-for="n in 2" :key="n" class="h-56" />
    </div>
  </div>
</template>
