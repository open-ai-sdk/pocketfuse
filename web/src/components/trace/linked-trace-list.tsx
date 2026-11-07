import { Link } from '@tanstack/react-router'
import { Unlink } from 'lucide-react'
import type { Trace } from '../../lib/api'
import { cn, formatCost, formatDate, formatDuration, formatTokens } from '../../lib/utils'
import { StatusChip } from '../../pages/primitives'
import { Badge } from '../ui'

const CORRELATION_KEYS = ['agent_id', 'job_id', 'plan_id'] as const

// Correlation identifiers the sender attached to a trace.
export function correlationEntries(trace: Trace): [string, string][] {
  const metadata = trace.metadata ?? {}
  return CORRELATION_KEYS.flatMap((key) => {
    const value = metadata[key]
    return typeof value === 'string' || typeof value === 'number'
      ? [[key, String(value)] as [string, string]]
      : []
  })
}

// `thread:9` reads as "Thread 9"; other session ids are shown as sent.
export function sessionLabel(sessionId: string) {
  return sessionId.startsWith('thread:') ? `Thread ${sessionId.slice('thread:'.length)}` : sessionId
}

// Chronological rows for a set of traces. `unlinked` entries matched by time
// only: they are drawn differently because nothing recorded the connection.
export type LinkedTrace = { trace: Trace; unlinked?: boolean }

export function LinkedTraceList({
  items,
  currentId,
  emptyText,
}: {
  items: LinkedTrace[]
  currentId?: string
  emptyText: string
}) {
  if (items.length === 0) {
    return <p className='px-3 py-6 text-center text-[11px] text-content-tertiary'>{emptyText}</p>
  }
  return (
    <ol className='flex flex-col divide-y divide-border-primary'>
      {items.map(({ trace, unlinked }) => (
        <li key={trace.id}>
          <Link
            to='/traces/$traceId'
            params={{ traceId: trace.id }}
            className={cn(
              'flex flex-col gap-1 px-3 py-2.5 hover:bg-surface-tertiary',
              trace.id === currentId && 'bg-interactive-secondary-hover',
              unlinked && 'border-l-2 border-dashed border-l-border-warning',
            )}
          >
            <span className='flex flex-wrap items-center gap-2'>
              <span className='font-mono text-[10px] tabular-nums text-content-tertiary'>
                {formatDate(trace.startTime ?? trace.timestamp)}
              </span>
              <span className='min-w-0 truncate text-xs font-medium text-content-primary'>
                {trace.name || 'Untitled trace'}
              </span>
              <StatusChip status={trace.status} />
              {unlinked && (
                <span title='Ran at the same time, but no identifier connects it. Another thread may own it.'>
                  <Badge tone='amber' className='gap-1 rounded-[3px] px-1.5 text-[10px]'>
                    <Unlink className='size-2.5' />
                    Time match, not linked
                  </Badge>
                </span>
              )}
            </span>
            <span className='flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[10.5px] text-content-secondary'>
              <span className='font-mono tabular-nums'>{formatDuration(trace.duration)}</span>
              {trace.models?.map((model) => (
                <span key={model}>{model}</span>
              ))}
              <span>
                {trace.totalTokens !== undefined
                  ? `${formatTokens(trace.totalTokens)} tokens`
                  : 'tokens —'}
              </span>
              <span>cost {formatCost(trace.totalCost)}</span>
              {correlationEntries(trace).map(([key, value]) => (
                <span
                  key={key}
                  className='font-mono text-content-tertiary'
                  title={`${key}: ${value}`}
                >
                  {key.replace('_id', '')} {value.slice(0, 8)}
                </span>
              ))}
            </span>
          </Link>
        </li>
      ))}
    </ol>
  )
}
