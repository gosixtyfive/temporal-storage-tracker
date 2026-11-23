<script setup lang="ts">
const route = useRoute()
const id = computed(() => String(route.params.id))

useHead({ title: 'Item status' })
</script>

<template>
  <div>
    <h1 class="mb-6 text-2xl font-semibold">
      Item status
    </h1>
    <UEmpty
      icon="i-lucide-package-search"
      title="Item status coming soon"
      :description="`This page will show the details, time left and notifications for ${id}.`"
    />
  </div>
</template>
