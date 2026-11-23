<script setup lang="ts">
import type { AppConfig } from '~/utils/api'

// The storage duration and reminder lead time the server is running with.
const api = useApi()
const config = ref<AppConfig>()
const error = ref<string>()

onMounted(async () => {
  try {
    config.value = await api.getConfig()
  }
  catch (err) {
    const apiErr = toApiError(err)
    error.value = isUnreachable(apiErr) ? `${apiErr.message}. Is the backend running (make server)?` : apiErr.message
  }
})
</script>

<template>
  <p class="flex items-start gap-2 text-sm text-muted">
    <UIcon name="i-lucide-timer" class="mt-0.5 size-4 shrink-0 text-primary" />
    <span v-if="config">
      Items can stay for <strong class="text-default">{{ formatDuration(config.storageDurationSeconds) }}</strong>;
      the reminder goes out <strong class="text-default">{{ formatDuration(config.reminderLeadSeconds) }}</strong> before an item expires.
      (Real world: 7 days, reminder 1 day before.)
    </span>
    <span v-else-if="error" class="text-error">
      Could not load the storage timing: {{ error }}
    </span>
    <USkeleton v-else class="h-4 w-72" />
  </p>
</template>
