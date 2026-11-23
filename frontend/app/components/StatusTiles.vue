<script setup lang="ts">
import { ITEM_STATUSES } from '~/utils/api'
import { STATUS_META, type StatusFilter, TEXT_COLOR } from '~/utils/items'

// One tile per status with its count; clicking a tile filters the dashboard.
defineProps<{ counts: Record<StatusFilter, number> }>()
const filter = defineModel<StatusFilter>({ required: true })

const tiles = [
  { value: 'all' as StatusFilter, label: 'All', icon: 'i-lucide-list', color: 'neutral' as const },
  ...ITEM_STATUSES.map(s => ({ value: s as StatusFilter, ...STATUS_META[s] })),
]
</script>

<template>
  <div class="grid grid-cols-3 gap-2 sm:grid-cols-6" role="group" aria-label="Filter by status">
    <button
      v-for="tile in tiles"
      :key="tile.value"
      type="button"
      :aria-pressed="filter === tile.value"
      class="flex flex-col items-start gap-1 rounded-md border px-3 py-2 text-left transition-colors"
      :class="filter === tile.value
        ? 'border-primary bg-primary/10'
        : 'border-default hover:bg-elevated'"
      @click="filter = tile.value"
    >
      <span class="flex items-center gap-1.5 text-xs text-muted">
        <UIcon :name="tile.icon" class="size-3.5" :class="TEXT_COLOR[tile.color]" />
        {{ tile.label }}
      </span>
      <span class="text-xl font-semibold tabular-nums">{{ counts[tile.value] }}</span>
    </button>
  </div>
</template>
