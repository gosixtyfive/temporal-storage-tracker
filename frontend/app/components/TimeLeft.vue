<script setup lang="ts">
import type { Item } from '~/utils/api'
import { timeLeft } from '~/utils/items'

const props = defineProps<{ item: Item }>()
const now = useNow()
const left = computed(() => timeLeft(props.item, now.value))
</script>

<template>
  <div class="flex flex-col gap-1.5">
    <div class="flex items-baseline justify-between gap-2 text-sm">
      <span class="font-medium tabular-nums" :class="left.phase === 'expired' ? 'text-error' : left.phase === 'expiringSoon' ? 'text-warning' : ''">
        {{ left.label }}
      </span>
      <span v-if="left.phase !== 'ended'" class="text-xs text-muted">
        expires {{ new Date(item.expiresAt).toLocaleTimeString() }}
      </span>
    </div>
    <UProgress :model-value="left.progress" :color="left.color" size="sm" :aria-label="`Storage time used: ${Math.round(left.progress)}%`" />
  </div>
</template>
