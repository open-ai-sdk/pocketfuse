export { cn } from 'cn'
export function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}
export function formatClock(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}
export function formatRelative(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const delta = Date.now() - date.getTime()
  const absolute = Math.abs(delta)
  if (absolute < 60_000) return 'just now'
  if (absolute < 3_600_000) return `${Math.round(absolute / 60_000)}m ago`
  if (absolute < 86_400_000) return `${Math.round(absolute / 3_600_000)}h ago`
  if (absolute < 604_800_000) return `${Math.round(absolute / 86_400_000)}d ago`
  return formatDate(value)
}
export function formatDuration(value?: number) {
  if (value === undefined || !Number.isFinite(value)) return '—'
  if (value < 1_000) return `${Math.max(0, Math.round(value))}ms`
  if (value < 60_000) return `${(value / 1_000).toFixed(2)}s`
  return `${(value / 60_000).toFixed(2)}m`
}
export function formatCost(value?: number) {
  if (value === undefined || !Number.isFinite(value)) return '—'
  if (value === 0) return '$0'
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 4,
  }).format(value)
}
export function formatTokens(value?: number) {
  if (value === undefined || !Number.isFinite(value)) return '—'
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(
    value,
  )
}
// Sum of the values that are known; undefined when none is, so an unreported
// measure stays unknown instead of reading as zero.
export function sumKnown<T>(items: T[], pick: (item: T) => number | undefined) {
  const known = items.map(pick).filter((value): value is number => value !== undefined)
  return known.length > 0 ? known.reduce((total, value) => total + value, 0) : undefined
}
// `130.8K in (124.3K cached) · 2.4K out`, or undefined when nothing was reported.
export function formatUsage(usage?: { input?: number; cachedInput?: number; output?: number }) {
  if (!usage || (usage.input === undefined && usage.output === undefined)) return undefined
  const cached = usage.cachedInput ? ` (${formatTokens(usage.cachedInput)} cached)` : ''
  return `${formatTokens(usage.input)} in${cached} · ${formatTokens(usage.output)} out`
}
export function prettyJson(value: unknown) {
  if (value === undefined || value === null) return ''
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}
