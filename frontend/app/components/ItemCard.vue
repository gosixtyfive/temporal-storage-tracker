<script setup lang="ts">
import type { Item } from '~/utils/api'
import { temporalWorkflowUrl } from '~/utils/items'

const props = defineProps<{
  item: Item
  /** Briefly true when a poll brought a new notification for this item. */
  highlighted?: boolean
  /** The action whose Update is in flight, if any. */
  busy?: 'collect' | 'remove'
}>()

const emit = defineEmits<{
  collect: [item: Item]
  remove: [item: Item]
}>()

const config = useRuntimeConfig()
const workflowUrl = computed(() =>
  temporalWorkflowUrl(config.public.temporalUiUrl, config.public.temporalNamespace, props.item.id))
const ended = computed(() => isTerminal(props.item.status))
</script>

<template>
  <UCard
    class="transition-all duration-500"
    :class="[
      ended ? 'opacity-60' : '',
      highlighted ? 'ring-2 ring-warning shadow-lg' : '',
    ]"
    :ui="{ body: 'flex flex-col gap-4' }"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <h3 class="truncate font-semibold" :title="item.itemName">
          {{ item.itemName }}
        </h3>
        <p class="text-sm text-muted">
          {{ item.memberName }}
        </p>
        <p v-if="item.description" class="mt-1 line-clamp-2 text-sm text-toned">
          {{ item.description }}
        </p>
      </div>
      <StatusBadge :status="item.status" class="shrink-0" />
    </div>

    <TimeLeft :item="item" />

    <NotificationList :notifications="item.notifications" />

    <div class="flex flex-wrap items-center justify-between gap-2 border-t border-default pt-3">
      <ULink :to="workflowUrl" target="_blank" class="flex items-center gap-1 text-xs" :title="`Open workflow ${item.id} in the Temporal Web UI`">
        <UIcon name="i-lucide-workflow" class="size-3.5" />
        Workflow
        <UIcon name="i-lucide-external-link" class="size-3" />
      </ULink>
      <div v-if="!ended" class="flex gap-2">
        <UButton
          label="Collect"
          icon="i-lucide-hand"
          size="sm"
          color="success"
          variant="soft"
          :loading="busy === 'collect'"
          :disabled="!!busy"
          @click="emit('collect', item)"
        />
        <UButton
          label="Remove"
          icon="i-lucide-trash-2"
          size="sm"
          color="neutral"
          variant="outline"
          :loading="busy === 'remove'"
          :disabled="!!busy"
          @click="emit('remove', item)"
        />
      </div>
    </div>
  </UCard>
</template>
