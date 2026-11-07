import { ChevronDown, ChevronRight, ListTree, TriangleAlert } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import type { Trace, TurnStep } from '../../lib/api'
import {
  barLayout,
  displayName,
  flattenTree,
  observationHasError,
  observationIsPending,
  observationOffsetMs,
  observationTokens,
  type FlatNode,
  type TreeNode,
} from '../../lib/trace-tree'
import { cn, formatCost, formatDuration, formatTokens } from '../../lib/utils'
import { Button, Input } from '../ui'
import { kindOf, toneBarClass, toneChipClass } from './kinds'

// The trace's structure and its timing in one view: every row is a tree node on
// the left and a bar on a shared time axis on the right, so nothing needs a
// second view to compare.

const ROW_GRID = 'grid grid-cols-[minmax(0,1fr)_minmax(96px,30%)] gap-x-2'
const TICKS = [0, 0.25, 0.5, 0.75, 1]

function TreeRow({
  flat,
  selected,
  onSelect,
  collapsed,
  onToggle,
  originMs,
  windowMs,
  step,
  raw,
}: {
  flat: FlatNode
  selected: boolean
  onSelect: () => void
  collapsed: boolean
  onToggle: () => void
  originMs: number
  windowMs: number
  step?: TurnStep
  raw: boolean
}) {
  const { node, treeLines, isLastSibling } = flat
  const observation = node.observation
  const kind = kindOf(observation)
  const Icon = kind.icon
  const tokens = observationTokens(observation)
  const pending = observationIsPending(observation)
  const error = observationHasError(observation)
  const offset = observationOffsetMs(node, originMs)
  const hasChildren = node.children.length > 0
  const bar = barLayout(offset, pending ? windowMs - offset : node.durationMs, windowMs)
  const calls = step?.toolCalls ?? []
  return (
    <div
      role='treeitem'
      aria-selected={selected}
      aria-expanded={hasChildren ? !collapsed : undefined}
      onClick={onSelect}
      className={cn(
        ROW_GRID,
        'cursor-pointer rounded-[2px] pr-1.5',
        selected ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
      )}
    >
      <div className='flex min-w-0 items-stretch'>
        {treeLines.map((hasLaterSiblings, depth) => (
          <span key={depth} className='relative w-3.5 shrink-0'>
            {hasLaterSiblings && (
              <span className='absolute left-[7px] top-0 h-full w-px bg-border-primary' />
            )}
          </span>
        ))}
        {node.depth > 0 && (
          <span className='relative w-3.5 shrink-0'>
            <span
              className={cn(
                'absolute left-[7px] top-0 w-px bg-border-primary',
                isLastSibling ? 'h-1/2' : 'h-full',
              )}
            />
            <span className='absolute left-[7px] top-1/2 h-px w-1.5 bg-border-primary' />
          </span>
        )}
        <button
          type='button'
          aria-label={collapsed ? 'Expand' : 'Collapse'}
          onClick={(event) => {
            event.stopPropagation()
            onToggle()
          }}
          className={cn(
            'my-auto flex size-11 shrink-0 items-center justify-center rounded-[2px] text-content-tertiary hover:text-content-primary md:size-10 pointer-coarse:size-11',
            !hasChildren && 'invisible',
          )}
        >
          <ChevronRight className={cn('size-3 transition-transform', !collapsed && 'rotate-90')} />
        </button>
        <span
          title={kind.label}
          className={cn(
            'my-auto flex size-5 shrink-0 items-center justify-center rounded-[3px] border',
            toneChipClass[kind.tone],
          )}
        >
          <Icon className='size-3' />
        </span>
        <span className='ml-1.5 flex min-w-0 flex-1 flex-col justify-center py-1'>
          <span className='flex items-baseline gap-1.5'>
            <span
              className={cn(
                'truncate text-xs font-medium',
                error ? 'text-content-error' : 'text-content-primary',
                step?.side && 'text-content-secondary',
              )}
              title={observation.name}
            >
              {raw ? observation.name || displayName(observation) : displayName(observation)}
            </span>
            {step?.side && (
              <span className='shrink-0 text-[9.5px] text-content-tertiary'>side call</span>
            )}
          </span>
          <span className='flex items-center gap-2 text-[9.5px] text-content-tertiary'>
            <span className='shrink-0 font-mono tabular-nums'>
              {pending ? 'running' : formatDuration(observation.duration ?? node.durationMs)}
            </span>
            {tokens.total !== undefined && (
              <span className='shrink-0'>{formatTokens(tokens.total)} tok</span>
            )}
            {observation.cost !== undefined && (
              <span className='shrink-0'>{formatCost(observation.cost)}</span>
            )}
            {calls.map((call, index) => (
              <span
                key={index}
                className={cn(
                  'truncate font-mono',
                  call.error ? 'text-content-error' : 'text-content-warning',
                )}
              >
                {call.error && <TriangleAlert className='mr-0.5 inline size-2.5' />}
                {call.name}
              </span>
            ))}
          </span>
        </span>
      </div>
      <span className='relative my-auto h-4' title={`+${formatDuration(offset)}`}>
        {TICKS.slice(1, -1).map((tick) => (
          <span
            key={tick}
            aria-hidden='true'
            className='absolute inset-y-0 w-px bg-border-primary'
            style={{ left: `${tick * 100}%` }}
          />
        ))}
        <span
          className={cn(
            'absolute top-1/2 h-2 -translate-y-1/2 rounded-[1px]',
            error
              ? 'bg-action-danger'
              : pending
                ? 'motion-safe:animate-pulse bg-action-warning'
                : toneBarClass[kind.tone],
          )}
          style={{ ...bar, minWidth: 2 }}
        />
      </span>
    </div>
  )
}

export function TraceTreePanel({
  trace,
  roots,
  selectedId,
  onSelect,
  timeWindow,
  durationMs,
  raw,
  onToggleRaw,
  hasMerged,
}: {
  trace: Trace
  roots: TreeNode[]
  selectedId: string
  onSelect: (id: string) => void
  timeWindow: { originMs: number; endMs: number }
  durationMs?: number
  raw: boolean
  onToggleRaw: () => void
  hasMerged: boolean
}) {
  const [query, setQuery] = useState('')
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [errorsOnly, setErrorsOnly] = useState(false)
  const treeRef = useRef<HTMLDivElement>(null)
  const windowMs = Math.max(1, timeWindow.endMs - timeWindow.originMs)
  const steps = useMemo(
    () => new Map((trace.turn?.steps ?? []).map((step) => [step.observationId, step])),
    [trace.turn],
  )
  const hasErrors = useMemo(
    () => (trace.observations ?? []).some(observationHasError),
    [trace.observations],
  )

  const toggleCollapse = (id: string) =>
    setCollapsed((previous) => {
      const next = new Set(previous)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const rows = useMemo(() => {
    const flat = flattenTree(roots, collapsed)
    const needle = query.trim().toLowerCase()
    if (!needle && !errorsOnly) return flat
    const matches = (item: FlatNode) => {
      const observation = item.node.observation
      const tools = steps.get(observation.id)?.toolCalls?.map((call) => call.name) ?? []
      return (
        (!errorsOnly || observationHasError(observation)) &&
        (!needle ||
          [observation.name, observation.label, ...tools].some((name) =>
            name?.toLowerCase().includes(needle),
          ))
      )
    }
    // Keep the ancestors of a match so the tree stays readable.
    const visible = new Set<string>()
    const byId = new Map(flat.map((item) => [item.node.observation.id, item]))
    for (const item of flat) {
      if (!matches(item)) continue
      let current: FlatNode | undefined = item
      while (current) {
        visible.add(current.node.observation.id)
        const parentId: string | undefined = current.node.observation.parentObservationId
        current = parentId ? byId.get(parentId) : undefined
      }
    }
    return flat.filter((item) => visible.has(item.node.observation.id))
  }, [roots, collapsed, query, errorsOnly, steps])

  const collapseAll = () =>
    setCollapsed(
      new Set(
        rows
          .map((row) => row.node)
          .filter((node) => node.children.length > 0)
          .map((node) => node.observation.id),
      ),
    )

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='flex flex-wrap items-center gap-1.5 border-b border-border-primary px-2.5 py-2'>
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder='Find a span or tool'
          className='h-11 min-w-28 flex-1 rounded-[3px] border-border-primary bg-surface-tertiary text-xs shadow-none'
        />
        {hasErrors && (
          <button
            type='button'
            aria-pressed={errorsOnly}
            onClick={() => setErrorsOnly((value) => !value)}
            className={cn(
              'min-h-11 shrink-0 rounded-[3px] border px-2 py-1 font-mono text-xs uppercase',
              errorsOnly
                ? 'border-border-danger bg-surface-error text-content-error'
                : 'border-border-primary text-content-tertiary hover:text-content-secondary',
            )}
          >
            Errors
          </button>
        )}
        {(hasMerged || raw) && (
          <button
            type='button'
            aria-pressed={raw}
            onClick={onToggleRaw}
            title='Nested spans recording one provider call are folded into one row. Show them as recorded.'
            className={cn(
              'min-h-11 shrink-0 rounded-[3px] border px-2 py-1 font-mono text-xs uppercase',
              raw
                ? 'border-border-brand bg-interactive-secondary-hover text-content-brand'
                : 'border-border-primary text-content-tertiary hover:text-content-secondary',
            )}
          >
            Raw spans
          </button>
        )}
        <Button
          variant='ghost'
          size='icon-xs'
          onClick={() => setCollapsed(new Set())}
          aria-label='Expand all'
          title='Expand all'
          className='min-h-11 min-w-11 text-content-secondary'
        >
          <ChevronDown />
        </Button>
        <Button
          variant='ghost'
          size='icon-xs'
          onClick={collapseAll}
          aria-label='Collapse all'
          title='Collapse all'
          className='min-h-11 min-w-11 text-content-secondary'
        >
          <ChevronRight />
        </Button>
      </div>
      <div className={cn(ROW_GRID, 'shrink-0 border-b border-border-primary px-1.5 pb-1 pt-1.5')}>
        <span className='pl-1 font-mono text-[9.5px] font-medium uppercase tracking-[0.08em] text-content-tertiary'>
          Span
        </span>
        <span className='relative h-3.5'>
          {TICKS.filter((_, index) => index % 2 === 0).map((tick) => (
            <span
              key={tick}
              className='absolute top-0 font-mono text-[9.5px] tabular-nums text-content-tertiary'
              style={{
                left: `${tick * 100}%`,
                transform:
                  tick === 1 ? 'translateX(-100%)' : tick === 0 ? undefined : 'translateX(-50%)',
              }}
            >
              {formatDuration(tick * windowMs)}
            </span>
          ))}
        </span>
      </div>
      <div
        role='tree'
        ref={treeRef}
        tabIndex={0}
        className='min-h-0 flex-1 overflow-auto p-1.5 outline-none'
        onKeyDown={(event) => {
          const order = ['trace', ...rows.map((row) => row.node.observation.id)]
          const index = order.indexOf(selectedId)
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault()
            const step = event.key === 'ArrowDown' ? 1 : -1
            onSelect(order[Math.max(0, Math.min(order.length - 1, index + step))])
            requestAnimationFrame(() =>
              treeRef.current
                ?.querySelector(`[aria-selected="true"]`)
                ?.scrollIntoView({ block: 'nearest' }),
            )
          } else if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
            if (!rows.some((row) => row.node.observation.id === selectedId)) return
            event.preventDefault()
            if ((event.key === 'ArrowRight') === collapsed.has(selectedId)) {
              toggleCollapse(selectedId)
            }
          }
        }}
      >
        <div
          role='treeitem'
          aria-selected={selectedId === 'trace'}
          onClick={() => onSelect('trace')}
          className={cn(
            ROW_GRID,
            'min-h-11 cursor-pointer rounded-[2px] py-1 pr-1.5 md:min-h-10 pointer-coarse:min-h-11',
            selectedId === 'trace' ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
          )}
        >
          <span className='flex min-w-0 items-center gap-1.5 pl-1'>
            <ListTree className='size-3.5 shrink-0 text-content-brand' />
            <span className='min-w-0 flex-1 truncate text-xs font-semibold text-content-primary'>
              {trace.name || 'Trace'}
            </span>
            <span className='shrink-0 font-mono text-[9.5px] text-content-tertiary'>
              {formatDuration(durationMs)}
            </span>
          </span>
          <span className='relative my-auto h-4'>
            <span className='absolute inset-x-0 top-1/2 h-0.5 -translate-y-1/2 bg-border-secondary' />
          </span>
        </div>
        {rows.length === 0 ? (
          <p className='px-2 py-6 text-center text-[11px] text-content-tertiary'>
            {query || errorsOnly ? 'No matching spans' : 'No spans recorded'}
          </p>
        ) : (
          rows.map((flat) => (
            <TreeRow
              key={flat.node.observation.id}
              flat={flat}
              selected={selectedId === flat.node.observation.id}
              onSelect={() => onSelect(flat.node.observation.id)}
              collapsed={collapsed.has(flat.node.observation.id)}
              onToggle={() => toggleCollapse(flat.node.observation.id)}
              originMs={timeWindow.originMs}
              windowMs={windowMs}
              step={steps.get(flat.node.observation.id)}
              raw={raw}
            />
          ))
        )}
      </div>
    </div>
  )
}
