import type { NewItem } from '~/utils/api'

// Limits match the Go API's validation (internal/api), which trims fields too.
export const NEW_ITEM_LIMITS = {
  memberName: 100,
  itemName: 100,
  description: 1000,
} as const

export type NewItemField = keyof typeof NEW_ITEM_LIMITS

/** A validation error for one form field, in the shape Nuxt UI's UForm expects. */
export interface FieldError {
  name: NewItemField
  message: string
}

const LABELS: Record<NewItemField, string> = {
  memberName: 'Member name',
  itemName: 'Item name',
  description: 'Description',
}

/** Checks the add-item form the same way the server does. */
export function validateNewItem(state: Partial<NewItem>): FieldError[] {
  const errors: FieldError[] = []
  for (const name of Object.keys(NEW_ITEM_LIMITS) as NewItemField[]) {
    const value = (state[name] ?? '').trim()
    if (value === '' && name !== 'description') {
      errors.push({ name, message: `${LABELS[name]} is required` })
    }
    else if ([...value].length > NEW_ITEM_LIMITS[name]) {
      errors.push({ name, message: `${LABELS[name]} must be at most ${NEW_ITEM_LIMITS[name]} characters` })
    }
  }
  return errors
}

/** The trimmed request body; an empty description is left out. */
export function toNewItem(state: Partial<NewItem>): NewItem {
  const description = (state.description ?? '').trim()
  return {
    memberName: (state.memberName ?? '').trim(),
    itemName: (state.itemName ?? '').trim(),
    ...(description ? { description } : {}),
  }
}

/**
 * Maps a server validation message such as "itemName is required" to the
 * field it is about, so it can be shown under that field. Returns undefined
 * for messages about no particular field.
 */
export function serverFieldError(message: string): FieldError | undefined {
  const field = (Object.keys(NEW_ITEM_LIMITS) as NewItemField[]).find(name => message.startsWith(`${name} `))
  return field ? { name: field, message: message.replace(field, LABELS[field]) } : undefined
}
