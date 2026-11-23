import { describe, expect, it } from 'vitest'
import { formatDuration } from '~/utils/format'

describe('formatDuration', () => {
  it.each([
    [604800, '7 days'],
    [86400, '1 day'],
    [90000, '1 day 1 hour'],
    [86430, '1 day'], // units stay adjacent
    [420, '7 min'],
    [60, '1 min'],
    [90, '1 min 30 sec'],
    [7, '7 sec'],
    [1, '1 sec'],
    [0, '0 sec'],
    [-5, '0 sec'],
    [59.6, '1 min'],
  ])('%s seconds → %s', (seconds, want) => {
    expect(formatDuration(seconds)).toBe(want)
  })
})
