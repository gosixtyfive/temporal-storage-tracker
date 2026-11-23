<script setup lang="ts">
import type { AppConfig } from '~/utils/api'
import type { Role } from '~/utils/role'

useHead({ title: 'Storage Tracker' })

const api = useApi()
const { setRole } = useRole()

const config = ref<AppConfig>()
const configError = ref<string>()

onMounted(async () => {
  try {
    config.value = await api.getConfig()
  }
  catch (err) {
    configError.value = toApiError(err).message
  }
})

async function continueAs(role: Role, path: string) {
  setRole(role)
  await navigateTo(path)
}
</script>

<template>
  <div class="flex flex-col gap-10">
    <UPageHero
      title="Makerspace storage, run by a workflow"
      description="Members leave items in the shared storage area for a limited time. Each stored item is a Temporal workflow that reminds the member before time runs out and tells them when it has expired."
      :ui="{ container: 'py-8 sm:py-12 lg:py-16' }"
    />

    <div class="grid gap-6 sm:grid-cols-2">
      <UPageCard
        title="Member"
        description="Store an item and check how much storage time it has left."
        icon="i-lucide-user"
        variant="subtle"
      >
        <UButton label="Continue as Member" trailing-icon="i-lucide-arrow-right" @click="continueAs('member', '/store')" />
      </UPageCard>
      <UPageCard
        title="Admin"
        description="Watch every stored item, see which are expiring or expired, and remove items."
        icon="i-lucide-shield"
        variant="subtle"
      >
        <UButton label="Continue as Admin" trailing-icon="i-lucide-arrow-right" @click="continueAs('admin', '/dashboard')" />
      </UPageCard>
    </div>

    <UCard variant="outline">
      <div class="flex items-start gap-3">
        <UIcon name="i-lucide-timer" class="mt-0.5 size-5 shrink-0 text-primary" />
        <p v-if="config" class="text-sm">
          Items can stay for <strong>{{ formatDuration(config.storageDurationSeconds) }}</strong>.
          The reminder goes out <strong>{{ formatDuration(config.reminderLeadSeconds) }}</strong> before an item expires.
          <span class="text-muted">(The real-world rule is 7 days with a reminder 1 day before; the demo runs faster.)</span>
        </p>
        <p v-else-if="configError" class="text-sm text-error">
          Could not load the storage timing: {{ configError }}. Is the backend running (<code>make server</code>)?
        </p>
        <USkeleton v-else class="h-5 w-80" />
      </div>
    </UCard>
  </div>
</template>
