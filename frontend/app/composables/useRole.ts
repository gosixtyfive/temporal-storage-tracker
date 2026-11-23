import { createRoleState } from '~/utils/role'

let state: ReturnType<typeof createRoleState> | undefined

/** The current role, shared app-wide and remembered in localStorage. */
export function useRole() {
  state ??= createRoleState(safeLocalStorage())
  return state
}

function safeLocalStorage(): Storage | null {
  try {
    return globalThis.localStorage ?? null
  }
  catch {
    return null
  }
}
