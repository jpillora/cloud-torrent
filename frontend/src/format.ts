import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(relativeTime)

// bytes formats a byte count with a metric prefix, e.g. 1500000 -> "1.5 MB".
export function bytes(n: number, decimals = 1): string {
  if (!n || n < 0 || !Number.isFinite(n)) return '0 B'
  const i = Math.floor(Math.floor(Math.log(n) * Math.LOG10E) / 3)
  const f = Math.pow(10, decimals)
  const s = Math.round((n / Math.pow(10, i * 3)) * f) / f
  const prefix = ['', 'K', 'M', 'G', 'T', 'P', 'Z'][i] ?? ''
  return s.toString().replace(/\.0+$/, '') + ' ' + prefix + 'B'
}

// round to one decimal place.
export function round(n: number): number {
  if (typeof n !== 'number' || isNaN(n)) return 0
  return Math.round(n * 10) / 10
}

// ago renders an RFC3339 timestamp as relative time, e.g. "5 minutes ago".
export function ago(t: string): string {
  return dayjs(t).fromNow()
}
