<script setup lang="ts">
useHead({ title: 'Store an item' })
</script>

<template>
  <div>
    <h1 class="mb-6 text-2xl font-semibold">
      Store an item
    </h1>
    <RoleHint needs="member" />
    <UEmpty
      icon="i-lucide-package-plus"
      title="Store form coming soon"
      description="This page will register an item you are leaving in the storage area."
    />
  </div>
</template>
