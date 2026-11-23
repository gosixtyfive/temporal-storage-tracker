<script setup lang="ts">
// The app has two views and one switch between them; there are no roles.
// See docs/architecture.md#frontend.
const views = [
  { label: 'Add item', icon: 'i-lucide-package-plus', to: '/add' },
  { label: 'Dashboard', icon: 'i-lucide-layout-dashboard', to: '/dashboard' },
]

const route = useRoute()
</script>

<template>
  <UHeader to="/dashboard" :toggle="false">
    <template #title>
      <span class="flex items-center gap-2">
        <UIcon name="i-lucide-archive" class="size-5 text-primary" />
        <span class="hidden sm:inline">Storage Tracker</span>
      </span>
    </template>

    <template #right>
      <UFieldGroup size="sm" aria-label="View">
        <UButton
          v-for="view in views"
          :key="view.to"
          :to="view.to"
          :label="view.label"
          :icon="view.icon"
          :color="route.path === view.to ? 'primary' : 'neutral'"
          :variant="route.path === view.to ? 'solid' : 'outline'"
          :aria-current="route.path === view.to ? 'page' : undefined"
        />
      </UFieldGroup>
      <UColorModeButton />
    </template>
  </UHeader>
</template>
