<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

const { isAdmin } = useRole()

// Navigation follows the role; every page still loads for either role.
const items = computed<NavigationMenuItem[]>(() => [
  { label: 'Home', icon: 'i-lucide-house', to: '/' },
  isAdmin.value
    ? { label: 'Dashboard', icon: 'i-lucide-layout-dashboard', to: '/dashboard' }
    : { label: 'Store an item', icon: 'i-lucide-package-plus', to: '/store' },
])
</script>

<template>
  <UHeader to="/">
    <template #title>
      <span class="flex items-center gap-2">
        <UIcon name="i-lucide-archive" class="size-5 text-primary" />
        Storage Tracker
      </span>
    </template>

    <UNavigationMenu :items="items" />

    <template #right>
      <RoleSwitch class="hidden sm:flex" />
      <UColorModeButton />
    </template>

    <template #body>
      <div class="flex flex-col gap-4">
        <UNavigationMenu :items="items" orientation="vertical" class="-mx-2.5" />
        <RoleSwitch />
      </div>
    </template>
  </UHeader>
</template>
