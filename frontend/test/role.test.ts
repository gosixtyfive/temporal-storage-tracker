import { describe, expect, it } from 'vitest'
import { createRoleState, ROLE_STORAGE_KEY } from '~/utils/role'

function memoryStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial))
  return {
    data,
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  }
}

describe('createRoleState', () => {
  it('defaults to member', () => {
    const { role, isMember, isAdmin } = createRoleState(memoryStorage())
    expect(role.value).toBe('member')
    expect(isMember.value).toBe(true)
    expect(isAdmin.value).toBe(false)
  })

  it('restores a saved admin role', () => {
    expect(createRoleState(memoryStorage({ [ROLE_STORAGE_KEY]: 'admin' })).role.value).toBe('admin')
  })

  it('ignores unknown saved values', () => {
    expect(createRoleState(memoryStorage({ [ROLE_STORAGE_KEY]: 'root' })).role.value).toBe('member')
  })

  it('saves changes', () => {
    const storage = memoryStorage()
    const { setRole, isAdmin } = createRoleState(storage)
    setRole('admin')
    expect(isAdmin.value).toBe(true)
    expect(storage.data.get(ROLE_STORAGE_KEY)).toBe('admin')
    expect(createRoleState(storage).role.value).toBe('admin')
  })

  it('works without storage, or when storage throws', () => {
    const noStorage = createRoleState(null)
    noStorage.setRole('admin')
    expect(noStorage.role.value).toBe('admin')

    const broken = {
      getItem: () => { throw new Error('blocked') },
      setItem: () => { throw new Error('blocked') },
    }
    const state = createRoleState(broken)
    expect(state.role.value).toBe('member')
    state.setRole('admin')
    expect(state.role.value).toBe('admin')
  })
})
