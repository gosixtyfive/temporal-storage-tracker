import { computed, ref, watch } from 'vue'

// The demo has no accounts: one person switches between the two roles. The
// role only changes navigation and which actions are shown.
export type Role = 'member' | 'admin'

export const ROLE_STORAGE_KEY = 'storage-tracker.role'

export const ROLE_LABELS: Record<Role, string> = {
  member: 'Member',
  admin: 'Admin',
}

type RoleStorage = Pick<Storage, 'getItem' | 'setItem'>

/**
 * Role state persisted in storage. storage may be null (or throw, as in some
 * private windows); the role then simply isn't remembered. Defaults to member.
 */
export function createRoleState(storage: RoleStorage | null) {
  const role = ref<Role>(read(storage))

  watch(role, (value) => {
    try {
      storage?.setItem(ROLE_STORAGE_KEY, value)
    }
    catch {
      // Not persisted; the in-memory role still works.
    }
  }, { flush: 'sync' })

  return {
    role,
    isAdmin: computed(() => role.value === 'admin'),
    isMember: computed(() => role.value === 'member'),
    setRole: (value: Role) => {
      role.value = value
    },
  }
}

function read(storage: RoleStorage | null): Role {
  try {
    return storage?.getItem(ROLE_STORAGE_KEY) === 'admin' ? 'admin' : 'member'
  }
  catch {
    return 'member'
  }
}
