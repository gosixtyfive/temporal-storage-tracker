<script setup lang="ts">
useHead({
  titleTemplate: title => (title && title !== 'Storage Tracker' ? `${title} · Storage Tracker` : 'Storage Tracker'),
})
</script>

<template>
  <UApp>
    <AppHeader />
    <UMain>
      <UContainer class="py-8">
        <NuxtPage />
      </UContainer>
    </UMain>
  </UApp>
</template>
