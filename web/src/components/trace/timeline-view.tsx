import { useMemo } from 'react'
import { cn, formatDuration } from '../../lib/utils'
import {
  flattenTree,
  observationIsPending,
  observationOffsetMs,
  type FlatNode,
  type TreeNode,
} from '../../lib/trace-tree'
import { kindFor, toneBarClass } from './kinds'

// Timeline view: a Gantt lane per observation over the trace's time
// window. Rows mirror the tree's DFS order and indent guides so both
// views read as the same structure — the lane just trades nesting for
// position in time. Bar colour encodes the observation type; error
// level and in-flight spans override it.
//
// The rows scroll natively (wheel/scrollbar) and the region is focusable
// so arrow keys scroll it too; the axis stays pinned above.

const TICKS = [0, 0.25, 0.5, 0.75, 1]

export function TimelineView({
  roots,
  timeWindow,
  selectedId,
  onSelect,
}: {
  roots: TreeNode[]
  timeWindow: { originMs: number; endMs: number }
  selectedId: string
  onSelect: (id: string) => void
}) {
  const rows = useMemo(() => flattenTree(roots, new Set()), [roots])
  const span = Math.max(1, timeWindow.endMs - timeWindow.originMs)

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      {/* axis: label column + mono tick marks over the lane column */}
      <div className='sticky top-0 z-10 grid shrink-0 grid-cols-[minmax(120px,36%)_minmax(0,1fr)] items-end gap-x-2 border-b border-border-primary bg-surface-primary px-2.5 pb-1 pt-2'>
        <span className='truncate pl-0.5 font-mono text-[9.5px] font-medium uppercase tracking-[0.08em] text-content-tertiary'>
          Observation
        </span>
        <div className='relative h-4'>
          {TICKS.map((tick) => (
            <span
              key={tick}
              className='absolute top-0 font-mono text-[9.5px] tabular-nums text-content-tertiary'
              style={{
                left: `${tick * 100}%`,
                transform:
                  tick === 1 ? 'translateX(-100%)' : tick === 0 ? undefined : 'translateX(-50%)',
              }}
            >
              {formatDuration(tick * span)}
            </span>
          ))}
        </div>
      </div>
      <div
        role='region'
        aria-label='Timeline rows'
        tabIndex={0}
        onKeyDown={(event) => {
          // Native arrow scrolling on a focused div is inconsistent
          // across browsers — handle it explicitly.
          if (event.target !== event.currentTarget) return
          const el = event.currentTarget
          const page = el.clientHeight * 0.9
          const delta =
            event.key === 'Home'
              ? -el.scrollTop
              : event.key === 'End'
                ? el.scrollHeight - el.scrollTop
                : event.key === 'ArrowUp'
                  ? -48
                  : event.key === 'ArrowDown'
                    ? 48
                    : event.key === 'PageUp'
                      ? -page
                      : event.key === 'PageDown'
                        ? page
                        : undefined
          if (delta === undefined) return
          event.preventDefault()
          el.scrollBy({ top: delta })
        }}
        className='min-h-0 flex-1 overflow-auto p-1.5 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-interactive-primary'
      >
        {rows.length === 0 ? (
          <p className='px-2 py-8 text-center text-[11px] text-content-tertiary'>
            No observations to chart
          </p>
        ) : (
          <div className='flex flex-col'>
            {rows.map((flat) => (
              <TimelineRow
                key={flat.node.observation.id}
                flat={flat}
                span={span}
                originMs={timeWindow.originMs}
                selected={selectedId === flat.node.observation.id}
                onSelect={() => onSelect(flat.node.observation.id)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function TimelineRow({
  flat,
  span,
  originMs,
  selected,
  onSelect,
}: {
  flat: FlatNode
  span: number
  originMs: number
  selected: boolean
  onSelect: () => void
}) {
  const { node, treeLines } = flat
  const observation = node.observation
  const kind = kindFor(observation.type)
  const pending = observationIsPending(observation)
  const error = observation.level?.toUpperCase() === 'ERROR'
  const offset = observationOffsetMs(node, originMs)
  const duration = node.durationMs
  const left = Math.max(0, Math.min(100, ((node.startMs - originMs) / span) * 100))
  const rawWidth = pending ? 100 - left : ((duration ?? 0) / span) * 100
  const width = Math.max(0, Math.min(rawWidth, 100 - left))
  const name = observation.name || 'Untitled observation'

  return (
    <div
      role='button'
      tabIndex={0}
      aria-pressed={selected}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onSelect()
        }
      }}
      title={`${kind.label} · ${name} · +${formatDuration(offset)} · ${
        pending ? 'running' : formatDuration(duration)
      }`}
      className={cn(
        'grid cursor-pointer grid-cols-[minmax(120px,36%)_minmax(0,1fr)] items-center gap-x-2 rounded-[2px] py-1 pr-2 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-interactive-primary',
        selected ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
      )}
    >
      {/* name column: ancestor guides + name + duration */}
      <span className='flex min-w-0 items-center pl-0.5'>
        {treeLines.map((hasLaterSiblings, depth) => (
          <span key={depth} className='relative h-4 w-3.5 shrink-0'>
            {hasLaterSiblings && (
              <span className='absolute left-[7px] h-full w-px bg-border-primary' />
            )}
          </span>
        ))}
        <span className='truncate text-[11px] font-medium text-content-primary'>{name}</span>
        <span className='ml-auto shrink-0 pl-2 font-mono text-[9.5px] tabular-nums text-content-tertiary'>
          {pending ? '···' : formatDuration(duration)}
        </span>
      </span>
      {/* lane: quarter gridlines + the bar itself */}
      <span className='relative h-4'>
        {TICKS.slice(1).map((tick) => (
          <span
            key={tick}
            aria-hidden='true'
            className='absolute inset-y-0 w-px bg-border-primary'
            style={{ left: `${tick * 100}%` }}
          />
        ))}
        <span
          className={cn(
            'absolute top-1/2 h-1.5 -translate-y-1/2 rounded-full',
            error
              ? 'bg-action-danger'
              : pending
                ? 'animate-pulse bg-action-warning'
                : toneBarClass[kind.tone],
          )}
          style={{ left: `${left}%`, width: `${width}%`, minWidth: 2 }}
        />
      </span>
    </div>
  )
}
