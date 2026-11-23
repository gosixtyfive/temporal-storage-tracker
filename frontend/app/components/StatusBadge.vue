<script setup lang="ts">
import type { ItemStatus } from '~/utils/api'
import { STATUS_META } from '~/utils/items'

const props = defineProps<{ status: ItemStatus }>()
const meta = computed(() => STATUS_META[props.status])
</script>

<template>
  <UBadge :color="meta.color" :icon="meta.icon" :label="meta.label" variant="subtle" />
</template>
