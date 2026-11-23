import { describe, expect, it } from 'vitest'
import { NEW_ITEM_LIMITS, serverFieldError, toNewItem, validateNewItem } from '~/utils/newItem'

describe('validateNewItem', () => {
  it('accepts a valid item, with or without a description', () => {
    expect(validateNewItem({ memberName: 'Sam', itemName: 'Bike frame' })).toEqual([])
    expect(validateNewItem({ memberName: 'Sam', itemName: 'Bike frame', description: 'Steel' })).toEqual([])
  })

  it('requires member and item names, ignoring whitespace', () => {
    expect(validateNewItem({ memberName: '  ', itemName: '' })).toEqual([
      { name: 'memberName', message: 'Member name is required' },
      { name: 'itemName', message: 'Item name is required' },
    ])
    expect(validateNewItem({})).toHaveLength(2)
  })

  it('enforces the server limits on trimmed values, counting characters', () => {
    const at = (n: number) => 'x'.repeat(n)
    expect(validateNewItem({ memberName: at(100), itemName: ` ${at(100)} `, description: at(1000) })).toEqual([])
    expect(validateNewItem({ memberName: at(101), itemName: 'x', description: at(1001) })).toEqual([
      { name: 'memberName', message: `Member name must be at most ${NEW_ITEM_LIMITS.memberName} characters` },
      { name: 'description', message: `Description must be at most ${NEW_ITEM_LIMITS.description} characters` },
    ])
    // Multi-byte characters count once each, as on the server.
    expect(validateNewItem({ memberName: '🧰'.repeat(100), itemName: 'x' })).toEqual([])
  })
})

describe('toNewItem', () => {
  it('trims fields and drops an empty description', () => {
    expect(toNewItem({ memberName: ' Sam ', itemName: ' Bike ', description: '  ' }))
      .toEqual({ memberName: 'Sam', itemName: 'Bike' })
    expect(toNewItem({ memberName: 'Sam', itemName: 'Bike', description: ' Steel ' }))
      .toEqual({ memberName: 'Sam', itemName: 'Bike', description: 'Steel' })
  })
})

describe('serverFieldError', () => {
  it('maps server messages to the field they name', () => {
    expect(serverFieldError('itemName is required')).toEqual({ name: 'itemName', message: 'Item name is required' })
    expect(serverFieldError('description is too long')).toEqual({ name: 'description', message: 'Description is too long' })
  })

  it('returns undefined for other messages', () => {
    expect(serverFieldError('invalid JSON body')).toBeUndefined()
    expect(serverFieldError('internal error')).toBeUndefined()
  })
})
