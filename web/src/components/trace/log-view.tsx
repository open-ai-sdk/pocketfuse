import { useMemo } from 'react'
import { cn, formatDuration } from '../../lib/utils'
import {
  observationDurationMs,
  observationIsPending,
  observationStartMs,
  observationTokens,
} from '../../lib/trace-tree'
import type { Observation } from '../../lib/api'
import { kindFor } from './kinds'

// Log view: every observation in the trace as one chronological row —
// the "scan the whole run" counterpart to the structured views. Clicking
// a row selects that observation everywhere.

const GRID = 'grid grid-cols-[76px_minmax(0,1fr)_68px_72px_60px] items-center gap-x-3'

export function TraceLogView({
  observations,
  originMs,
  selectedId,
  onSelect,
}: {
  observations: Observation[]
  originMs: number
  selectedId: string
  onSelect: (id: string) => void
}) {
  const rows = useMemo(
    () => [...observations].sort((a, b) => observationStartMs(a) - observationStartMs(b)),
    [observations],
  )

  return (
    <div className='flex flex-col'>
      <div className={cn(GRID, 'border-b border-border-primary px-1 pb-1.5 pt-1')}>
        {['Offset', 'Observation', 'Model', 'Duration', 'Tokens'].map((label, index) => (
          <span
            key={label}
            className={cn(
              'font-mono text-[9.5px] font-medium uppercase tracking-[0.08em] text-content-tertiary',
              index >= 2 && 'text-right',
            )}
          >
            {label}
          </span>
        ))}
      </div>
      {rows.length === 0 ? (
        <p className='px-1 py-8 text-center text-[11px] text-content-tertiary'>
          No observations recorded
        </p>
      ) : (
        rows.map((observation) => {
          const kind = kindFor(observation.type)
          const Icon = kind.icon
          const pending = observationIsPending(observation)
          const error = observation.level?.toUpperCase() === 'ERROR'
          const tokens = observationTokens(observation)
          const offset = Math.max(0, observationStartMs(observation) - originMs)
          const selected = selectedId === observation.id
          return (
            <div
              key={observation.id}
              role='button'
              tabIndex={0}
              aria-pressed={selected}
              onClick={() => onSelect(observation.id)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  onSelect(observation.id)
                }
              }}
              title={error ? observation.statusMessage || observation.name : observation.name}
              className={cn(
                GRID,
                'cursor-pointer border-b border-border-primary px-1 py-1.5 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-interactive-primary',
                selected ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
              )}
            >
              <span className='font-mono text-[10px] tabular-nums text-content-tertiary'>
                +{formatDuration(offset)}
              </span>
              <span className='flex min-w-0 items-center gap-1.5'>
                <Icon className='size-3 shrink-0 text-content-secondary' />
                <span
                  className={cn(
                    'truncate text-[11px] font-medium',
                    error ? 'text-content-error' : 'text-content-primary',
                  )}
                >
                  {observation.name || 'Untitled observation'}
                </span>
                <span className='shrink-0 text-[9.5px] uppercase tracking-wide text-content-tertiary'>
                  {kind.label}
                </span>
              </span>
              <span className='truncate text-right font-mono text-[10px] text-content-secondary'>
                {observation.model || '—'}
              </span>
              <span className='text-right font-mono text-[10px] tabular-nums text-content-secondary'>
                {pending ? 'running' : formatDuration(observationDurationMs(observation))}
              </span>
              <span className='text-right font-mono text-[10px] tabular-nums text-content-secondary'>
                {tokens.total !== undefined ? `${tokens.total}` : '—'}
              </span>
            </div>
          )
        })
      )}
    </div>
  )
}
