const UNITS: [seconds: number, singular: string, plural: string][] = [
  [86400, 'day', 'days'],
  [3600, 'hour', 'hours'],
  [60, 'min', 'min'],
  [1, 'sec', 'sec'],
]

/**
 * Formats a duration in seconds with its two largest units, for example
 * "7 days", "1 day 4 hours", "7 min", "1 min 30 sec". Used for the storage
 * timing and for "time left".
 */
export function formatDuration(totalSeconds: number): string {
  let rest = Math.max(0, Math.round(totalSeconds))
  if (rest === 0) {
    return '0 sec'
  }
  const parts: string[] = []
  for (const [size, singular, plural] of UNITS) {
    if (parts.length === 2) {
      break
    }
    const n = Math.floor(rest / size)
    if (n > 0) {
      parts.push(`${n} ${n === 1 ? singular : plural}`)
      rest -= n * size
    }
    else if (parts.length > 0) {
      break // keep the two units adjacent: "1 day", not "1 day 30 sec"
    }
  }
  return parts.join(' ')
}
