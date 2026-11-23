<script setup lang="ts">
import { ROLE_LABELS, type Role } from '~/utils/role'

// Shown on a page meant for the other role. Nothing is enforced (the demo has
// no accounts); this only explains why actions are missing.
const props = defineProps<{
  /** The role this page is meant for. */
  needs: Role
}>()

const { role, setRole } = useRole()
</script>

<template>
  <UAlert
    v-if="role !== props.needs"
    color="neutral"
    variant="subtle"
    icon="i-lucide-info"
    :title="`This page is for the ${ROLE_LABELS[props.needs]} role`"
    :description="`Switch to ${ROLE_LABELS[props.needs]} to use its actions. Anyone can switch; the demo has no accounts.`"
    :actions="[{ label: `Switch to ${ROLE_LABELS[props.needs]}`, color: 'neutral', variant: 'outline', onClick: () => setRole(props.needs) }]"
    class="mb-6"
  />
</template>
