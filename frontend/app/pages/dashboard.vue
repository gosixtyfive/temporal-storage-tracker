<script setup lang="ts">
useHead({ title: 'Dashboard' })
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-2">
      <h1 class="text-2xl font-semibold">
        Dashboard
      </h1>
      <StorageTiming />
    </div>
    <UEmpty
      icon="i-lucide-layout-dashboard"
      title="Dashboard coming soon"
      description="This page will list every stored item with its state, time left and notifications, and let you collect or remove items."
    />
  </div>
</template>
