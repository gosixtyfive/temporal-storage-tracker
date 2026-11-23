<script setup lang="ts">
useHead({ title: 'Dashboard' })
</script>

<template>
  <div>
    <h1 class="mb-6 text-2xl font-semibold">
      Dashboard
    </h1>
    <RoleHint needs="admin" />
    <UEmpty
      icon="i-lucide-layout-dashboard"
      title="Dashboard coming soon"
      description="This page will list every stored item with its state, time left and notifications."
    />
  </div>
</template>
