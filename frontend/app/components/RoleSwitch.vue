<script setup lang="ts">
import { ROLE_LABELS, type Role } from '~/utils/role'

const { role, setRole } = useRole()
const roles = Object.keys(ROLE_LABELS) as Role[]
</script>

<template>
  <UFieldGroup size="sm" role="group" aria-label="Role">
    <UButton
      v-for="r in roles"
      :key="r"
      :label="ROLE_LABELS[r]"
      :icon="r === 'admin' ? 'i-lucide-shield' : 'i-lucide-user'"
      :color="role === r ? 'primary' : 'neutral'"
      :variant="role === r ? 'solid' : 'outline'"
      :aria-pressed="role === r"
      @click="setRole(r)"
    />
  </UFieldGroup>
</template>
