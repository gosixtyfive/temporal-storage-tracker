<script setup lang="ts">
import type { ItemNotification } from '~/utils/api'
import { NOTIFICATION_META, TEXT_COLOR } from '~/utils/items'

const props = defineProps<{ notifications: ItemNotification[] }>()
const now = useNow()
const newestFirst = computed(() => [...props.notifications].reverse())
</script>

<template>
  <div class="flex flex-col gap-2">
    <p class="text-xs font-medium uppercase tracking-wide text-muted">
      Notifications
    </p>
    <p v-if="!newestFirst.length" class="text-sm text-dimmed">
      None yet
    </p>
    <ul v-else class="flex flex-col gap-2">
      <li v-for="n in newestFirst" :key="n.kind" class="flex gap-2 text-sm">
        <UIcon :name="NOTIFICATION_META[n.kind].icon" class="mt-0.5 size-4 shrink-0" :class="TEXT_COLOR[NOTIFICATION_META[n.kind].color]" />
        <div class="min-w-0">
          <p>{{ n.message }}</p>
          <time :datetime="n.sentAt" :title="new Date(n.sentAt).toLocaleString()" class="text-xs text-muted">
            {{ NOTIFICATION_META[n.kind].label }} {{ formatDuration((now - Date.parse(n.sentAt)) / 1000) }} ago
          </time>
        </div>
      </li>
    </ul>
  </div>
</template>
