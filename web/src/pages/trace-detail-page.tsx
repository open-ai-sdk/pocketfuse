import { useQuery } from '@tanstack/react-query'
import { Link, useParams, useNavigate, useSearch } from '@tanstack/react-router'
import {
  AlertCircle,
  ArrowLeft,
  Bot,
  ChevronDown,
  ChevronRight,
  Clock3,
  Database,
  Download,
  GitBranch,
  Hash,
  Layers,
  ListTree,
  MessageSquare,
  RefreshCw,
  Star,
  UserRound,
} from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { getScores, getTrace, type Observation, type Score, type Trace } from '../lib/api'
import {
  buildObservationTree,
  flattenTree,
  observationDurationMs,
  observationTokens,
  observationIsPending,
  observationOffsetMs,
  traceWindow,
  type FlatNode,
  type TreeNode,
} from '../lib/trace-tree'
import {
  cn,
  formatCost,
  formatDate,
  formatDuration,
  formatRelative,
  formatTokens,
  prettyJson,
} from '../lib/utils'
import { IOPreview } from '../components/io-preview'
import { kindFor, toneChipClass } from '../components/trace/kinds'
import { SegmentedControl, TabBar, type TabBarTab } from '../components/trace/segmented'
import { TimelineView } from '../components/trace/timeline-view'
import { GraphView } from '../components/trace/graph-view'
import { TraceLogView } from '../components/trace/log-view'
import { ScoreRows } from '../components/trace/score-rows'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Input,
  Separator,
  Skeleton,
} from '../components/ui'
import { MetricCell, MetricStrip, StatusChip, panelSurface } from './primitives'

// Left-panel view modes for exploring a trace's structure.
type ViewMode = 'tree' | 'timeline' | 'graph'
// Right-panel detail sections, shared by the trace root and observations.
type DetailTab = 'preview' | 'attributes' | 'scores' | 'log'

// Score chips grouped by name.
function ScoreBadges({ scores, className }: { scores: Score[]; className?: string }) {
  const groups = useMemo(() => {
    const byName = new Map<string, Score[]>()
    for (const score of scores) {
      const name = score.name || 'score'
      byName.set(name, [...(byName.get(name) ?? []), score])
    }
    return [...byName.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [scores])
  if (groups.length === 0) return null
  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      {groups.map(([name, items]) => (
        <span
          key={name}
          title={items
            .map(
              (s) =>
                `${s.value !== undefined ? s.value.toFixed(2) : (s.stringValue ?? '—')}${s.comment ? ` — ${s.comment}` : ''}`,
            )
            .join('\n')}
        >
          <Badge
            tone='blue'
            className='gap-1 rounded-[3px] px-1.5 font-mono text-[10px] normal-case'
          >
            <Star className='size-2.5' />
            {name}:{' '}
            {items
              .map((s) => (s.value !== undefined ? s.value.toFixed(2) : (s.stringValue ?? '—')))
              .join(', ')}
          </Badge>
        </span>
      ))}
    </div>
  )
}

// One tree row: ├/└ guide lines + type icon + name + latency/token/cost
// metrics + a proportional duration bar. Flattened DFS: ancestor
// guide lines encode the ancestor chain, not pixel nesting.
function TreeRow({
  flat,
  selected,
  onSelect,
  collapsed,
  onToggle,
  window: timeWindow,
}: {
  flat: FlatNode
  selected: boolean
  onSelect: () => void
  collapsed: boolean
  onToggle: () => void
  window: { originMs: number; endMs: number }
}) {
  const { node, treeLines, isLastSibling } = flat
  const observation = node.observation
  const kind = kindFor(observation.type)
  const Icon = kind.icon
  const tokens = observationTokens(observation)
  const pending = observationIsPending(observation)
  const span = timeWindow.endMs - timeWindow.originMs
  const left = Math.max(0, ((node.startMs - timeWindow.originMs) / span) * 100)
  const width = pending
    ? Math.max(2, 100 - left) // in-flight: bar runs to the window's right edge
    : Math.max(1.5, node.durationMs !== undefined ? (node.durationMs / span) * 100 : 1.5)
  const hasChildren = node.children.length > 0

  return (
    <div
      role='treeitem'
      aria-selected={selected}
      aria-expanded={hasChildren ? !collapsed : undefined}
      className={cn(
        'group relative flex cursor-pointer items-stretch rounded-[2px] pr-2',
        selected ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
      )}
      onClick={onSelect}
    >
      {/* ancestor guide lines */}
      {treeLines.map((hasLaterSiblings, depth) => (
        <span key={depth} className='relative w-5 shrink-0'>
          {hasLaterSiblings && (
            <span className='absolute left-[11px] top-0 h-full w-px bg-border-primary' />
          )}
        </span>
      ))}
      {/* own connector stub */}
      {node.depth > 0 && (
        <span className='relative w-5 shrink-0'>
          <span
            className={cn(
              'absolute left-[11px] top-0 w-px bg-border-primary',
              isLastSibling ? 'h-1/2' : 'h-full',
            )}
          />
          <span className='absolute left-[11px] top-1/2 h-px w-2 bg-border-primary' />
        </span>
      )}
      {/* collapse chevron */}
      <button
        type='button'
        aria-label={collapsed ? 'Expand' : 'Collapse'}
        className={cn(
          'my-auto flex size-4 shrink-0 items-center justify-center rounded-[2px] text-content-tertiary hover:text-content-primary',
          !hasChildren && 'invisible',
        )}
        onClick={(event) => {
          event.stopPropagation()
          onToggle()
        }}
      >
        <ChevronRight className={cn('size-3 transition-transform', !collapsed && 'rotate-90')} />
      </button>
      <span
        className={cn(
          'my-1 flex size-5 shrink-0 items-center justify-center rounded-[3px] border',
          toneChipClass[kind.tone],
        )}
      >
        <Icon className='size-3' />
      </span>
      <span className='ml-1.5 flex min-w-0 flex-1 flex-col py-1'>
        <span className='flex items-baseline gap-1.5'>
          <span className='truncate text-xs font-medium text-content-primary'>
            {observation.name || 'Untitled observation'}
          </span>
          <span className='shrink-0 text-[9.5px] uppercase tracking-wide text-content-tertiary'>
            {kind.label}
          </span>
          {observation.level && observation.level.toUpperCase() !== 'DEFAULT' && (
            <span
              className={cn(
                'shrink-0 font-mono text-[9.5px] uppercase',
                observation.level.toUpperCase() === 'ERROR'
                  ? 'text-content-error'
                  : 'text-content-warning',
              )}
            >
              {observation.level}
            </span>
          )}
        </span>
        {/* duration bar — pending rows pulse to the window edge */}
        <span className='relative mt-1 h-1 w-full overflow-hidden rounded-full bg-surface-tertiary'>
          <span
            className={cn(
              'absolute inset-y-0 rounded-full',
              observation.level?.toUpperCase() === 'ERROR'
                ? 'bg-action-danger'
                : pending
                  ? 'animate-pulse bg-action-warning'
                  : 'bg-interactive-primary',
            )}
            style={{ left: `${left}%`, width: `${Math.min(width, 100 - left)}%` }}
          />
        </span>
        <span className='mt-1 flex items-center gap-2 text-[9.5px] text-content-tertiary'>
          <span className='font-mono tabular-nums'>
            +{formatDuration(observationOffsetMs(node, timeWindow.originMs))}
          </span>
          <span className='font-mono tabular-nums'>
            {pending ? 'running' : formatDuration(observation.duration ?? node.durationMs)}
          </span>
          {tokens.total !== undefined && <span>{formatTokens(tokens.total)} tok</span>}
          {observation.cost !== undefined && <span>{formatCost(observation.cost)}</span>}
          {observation.model && <span className='truncate'>{observation.model}</span>}
        </span>
      </span>
    </div>
  )
}
// Left navigation panel: toolbar (search, expand/collapse all) + flattened
// observation tree. Synthetic TRACE row on top selects the trace itself.
function TraceNav({
  trace,
  roots,
  selectedId,
  onSelect,
  timeWindow,
  durationMs,
}: {
  trace: Trace
  roots: TreeNode[]
  selectedId: string
  onSelect: (id: string) => void
  timeWindow: { originMs: number; endMs: number }
  durationMs?: number
}) {
  const [query, setQuery] = useState('')
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
  const [typeFilter, setTypeFilter] = useState<string | null>(null)
  const [errorsOnly, setErrorsOnly] = useState(false)
  const treeRef = useRef<HTMLDivElement>(null)

  const toggleCollapse = (id: string) =>
    setCollapsed((previous) => {
      const next = new Set(previous)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const types = useMemo(
    () =>
      [
        ...new Set(
          roots
            .flatMap(function collect(node: TreeNode): string[] {
              return [node.observation.type ?? '', ...node.children.flatMap(collect)]
            })
            .filter(Boolean),
        ),
      ].sort(),
    [roots],
  )

  const rows = useMemo(() => {
    const flat = flattenTree(roots, collapsed)
    const needle = query.trim().toLowerCase()
    const matches = (f: FlatNode) =>
      (!typeFilter || (f.node.observation.type ?? '').toUpperCase() === typeFilter) &&
      (!errorsOnly || (f.node.observation.level ?? '').toUpperCase() === 'ERROR') &&
      (!needle || (f.node.observation.name ?? '').toLowerCase().includes(needle))
    const visible = new Set<string>()
    const byId = new Map(flat.map((f) => [f.node.observation.id, f]))
    for (const f of flat) {
      if (!matches(f)) continue
      // keep ancestors of matches visible so the tree stays readable
      let current: FlatNode | undefined = f
      while (current) {
        visible.add(current.node.observation.id)
        const parentId: string | undefined = current.node.observation.parentObservationId
        current = parentId ? byId.get(parentId) : undefined
      }
    }
    return flat.filter((f) => visible.has(f.node.observation.id))
  }, [roots, collapsed, query, typeFilter, errorsOnly])
  const expandAll = () => setCollapsed(new Set())
  const collapseAll = () =>
    setCollapsed(
      new Set(
        rows
          .map((r) => r.node)
          .filter((n) => n.children.length > 0)
          .map((n) => n.observation.id),
      ),
    )

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='flex items-center gap-1.5 border-b border-border-primary px-2.5 py-2'>
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder='Find in this trace'
          className='h-7 rounded-[3px] border-border-primary bg-surface-tertiary text-[11px] shadow-none'
        />
        <Button
          variant='ghost'
          size='icon-xs'
          onClick={expandAll}
          aria-label='Expand all'
          title='Expand all'
          className='text-content-secondary'
        >
          <ChevronDown />
        </Button>
        <Button
          variant='ghost'
          size='icon-xs'
          onClick={collapseAll}
          aria-label='Collapse all'
          title='Collapse all'
          className='text-content-secondary'
        >
          <ChevronRight />
        </Button>
      </div>
      {(types.length > 1 || errorsOnly) && (
        <div className='flex flex-wrap items-center gap-1 border-b border-border-primary px-2.5 py-1.5'>
          {types.map((type) => {
            const active = typeFilter === type
            return (
              <button
                key={type}
                type='button'
                onClick={() => setTypeFilter(active ? null : type)}
                className={cn(
                  'rounded-[3px] border px-1.5 py-0.5 font-mono text-[9.5px] uppercase',
                  active
                    ? 'border-border-brand bg-interactive-secondary-hover text-content-brand'
                    : 'border-border-primary text-content-tertiary hover:text-content-secondary',
                )}
              >
                {type}
              </button>
            )
          })}
          <button
            type='button'
            onClick={() => setErrorsOnly((value) => !value)}
            className={cn(
              'rounded-[3px] border px-1.5 py-0.5 font-mono text-[9.5px] uppercase',
              errorsOnly
                ? 'border-border-danger bg-surface-error text-content-error'
                : 'border-border-primary text-content-tertiary hover:text-content-secondary',
            )}
          >
            Errors
          </button>
        </div>
      )}
      <div
        role='tree'
        ref={treeRef}
        tabIndex={0}
        className='min-h-0 flex-1 overflow-auto p-1.5 outline-none'
        onKeyDown={(event) => {
          // Keyboard navigation over the flattened rows.
          const order = ['trace', ...rows.map((r) => r.node.observation.id)]
          const index = order.indexOf(selectedId)
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault()
            const next =
              order[
                Math.max(
                  0,
                  Math.min(order.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)),
                )
              ]
            onSelect(next)
            treeRef.current
              ?.querySelector(`[aria-selected="true"]`)
              ?.scrollIntoView({ block: 'nearest' })
          } else if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
            const flat = rows.find((r) => r.node.observation.id === selectedId)
            if (!flat) return
            event.preventDefault()
            if (event.key === 'ArrowRight') {
              if (collapsed.has(selectedId)) toggleCollapse(selectedId)
            } else if (!collapsed.has(selectedId)) toggleCollapse(selectedId)
          }
        }}
      >
        {/* synthetic TRACE root row — selects the trace itself */}
        <div
          role='treeitem'
          aria-selected={selectedId === 'trace'}
          className={cn(
            'flex cursor-pointer items-center gap-1.5 rounded-[2px] px-1.5 py-1.5',
            selectedId === 'trace' ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
          )}
          onClick={() => onSelect('trace')}
        >
          <ListTree className='size-3.5 shrink-0 text-content-brand' />
          <span className='min-w-0 flex-1 truncate text-xs font-semibold text-content-primary'>
            {trace.name || 'Trace'}
          </span>
          <span className='shrink-0 font-mono text-[9.5px] text-content-tertiary'>
            {formatDuration(durationMs)}
          </span>
        </div>
        <Separator className='my-1 bg-border-primary' />
        {rows.length === 0 ? (
          <p className='px-2 py-6 text-center text-[11px] text-content-tertiary'>
            {query ? 'No matching observations' : 'No observations recorded'}
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
              window={timeWindow}
            />
          ))
        )}
      </div>
    </div>
  )
}

// Detail section shared by both detail views: chronological log of every
// observation in the trace.
function LogTab({
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
  return (
    <div className={cn(panelSurface, 'rounded-[4px] p-3')}>
      <TraceLogView
        observations={observations}
        originMs={originMs}
        selectedId={selectedId}
        onSelect={onSelect}
      />
    </div>
  )
}

// Right panel when the TRACE root is selected. Header card stays above the
// tabs; the tab body switches between preview / attributes / scores / log.
function TraceDetailView({
  trace,
  scores,
  tab,
  observations,
  originMs,
  selectedId,
  onSelect,
  totalTokens,
  durationMs,
}: {
  trace: Trace
  scores: Score[]
  tab: DetailTab
  observations: Observation[]
  originMs: number
  selectedId: string
  onSelect: (id: string) => void
  totalTokens?: number
  durationMs?: number
}) {
  const detailRows = [
    { label: 'Project', value: trace.project, icon: Database },
    { label: 'Session', value: trace.sessionId, icon: MessageSquare },
    { label: 'User', value: trace.userId, icon: UserRound },
    { label: 'Started', value: formatDate(trace.startTime ?? trace.timestamp), icon: Clock3 },
    { label: 'Release', value: trace.release ?? trace.version, icon: GitBranch },
    { label: 'Environment', value: trace.environment, icon: Layers },
  ]
  return (
    <div className='flex flex-col gap-3'>
      <div className={cn(panelSurface, 'rounded-[4px] px-4 py-3')}>
        <div className='flex flex-wrap items-center gap-2'>
          <h2 className='text-sm font-semibold text-content-primary'>
            {trace.name || 'Untitled trace'}
          </h2>
          <StatusChip status={trace.status} />
          <ScoreBadges scores={scores} />
        </div>
        <div className='mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[10.5px] text-content-secondary'>
          <span className='font-mono text-content-tertiary'>{trace.id}</span>
          <span>{formatRelative(trace.startTime ?? trace.timestamp)}</span>
          <span className='font-mono'>{formatDuration(durationMs)}</span>
          {totalTokens !== undefined && <span>{formatTokens(totalTokens)} tokens</span>}
          {trace.totalCost !== undefined && <span>{formatCost(trace.totalCost)}</span>}
        </div>
        {trace.tags && trace.tags.length > 0 && (
          <div className='mt-2 flex flex-wrap gap-1'>
            {trace.tags.map((tag) => (
              <Badge
                key={tag}
                tone='neutral'
                className='rounded-[3px] font-mono text-[9.5px] text-content-secondary'
              >
                {tag}
              </Badge>
            ))}
          </div>
        )}
      </div>
      {tab === 'preview' && (
        <div className='grid gap-3 lg:grid-cols-2'>
          <IOPreview label='Input' value={trace.input} />
          <IOPreview label='Output' value={trace.output} />
        </div>
      )}
      {tab === 'attributes' && (
        <div className={cn(panelSurface, 'rounded-[4px] p-3')}>
          <h4 className='mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
            Details
          </h4>
          <dl className='grid gap-x-6 gap-y-2 sm:grid-cols-2'>
            {detailRows.map(({ label, value, icon: Icon }) => (
              <div key={label} className='flex items-start gap-2'>
                <Icon className='mt-0.5 size-3.5 shrink-0 text-content-secondary' />
                <div className='min-w-0'>
                  <dt className='font-mono text-[9.5px] uppercase tracking-[0.08em] text-content-tertiary'>
                    {label}
                  </dt>
                  <dd className='truncate text-xs text-content-primary'>{value || '—'}</dd>
                </div>
              </div>
            ))}
          </dl>
          {trace.metadata && Object.keys(trace.metadata).length > 0 && (
            <pre className='mt-3 max-h-48 overflow-auto rounded-[3px] border border-border-primary bg-surface-tertiary p-2.5 font-mono text-[10.5px] text-content-secondary'>
              {prettyJson(trace.metadata)}
            </pre>
          )}
        </div>
      )}
      {tab === 'scores' && (
        <div className={cn(panelSurface, 'rounded-[4px] p-3')}>
          <h4 className='mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
            Scores
          </h4>
          <ScoreRows scores={scores} emptyHint='No scores on this trace.' />
        </div>
      )}
      {tab === 'log' && (
        <LogTab
          observations={observations}
          originMs={originMs}
          selectedId={selectedId}
          onSelect={onSelect}
        />
      )}
    </div>
  )
}

// Right panel when an observation is selected: header card (type, timing,
// model, status) above the shared tab body.
function ObservationDetailView({
  observation,
  scores,
  tab,
  observations,
  originMs,
  selectedId,
  onSelect,
}: {
  observation: Observation
  scores: Score[]
  tab: DetailTab
  observations: Observation[]
  originMs: number
  selectedId: string
  onSelect: (id: string) => void
}) {
  const kind = kindFor(observation.type)
  const Icon = kind.icon
  const tokens = observationTokens(observation)
  const pending = observationIsPending(observation)
  const hasParams =
    observation.modelParameters && Object.keys(observation.modelParameters).length > 0
  const hasMetadata = observation.metadata && Object.keys(observation.metadata).length > 0
  const detailRows: { label: string; value: string; mono?: boolean }[] = [
    { label: 'Trace', value: observation.traceId || '—', mono: true },
    { label: 'Parent', value: observation.parentObservationId || '—', mono: true },
    { label: 'Type', value: kind.label },
    { label: 'Level', value: observation.level || 'DEFAULT' },
    {
      label: 'Started',
      value: formatDate(observation.startTime ?? observation.timestamp),
    },
    {
      label: 'Ended',
      value: observation.endTime ? formatDate(observation.endTime) : pending ? 'running' : '—',
    },
    {
      label: 'Duration',
      value: pending
        ? 'running'
        : formatDuration(observation.duration ?? observationDurationMs(observation)),
    },
    { label: 'Model', value: observation.model || '—' },
    {
      label: 'Usage',
      value:
        tokens.total !== undefined
          ? `${formatTokens(tokens.input ?? 0)} in · ${formatTokens(tokens.output ?? 0)} out · ${formatTokens(tokens.total)} total`
          : '—',
    },
    { label: 'Cost', value: observation.cost !== undefined ? formatCost(observation.cost) : '—' },
  ]

  return (
    <div className='flex flex-col gap-3'>
      <div className={cn(panelSurface, 'rounded-[4px] px-4 py-3')}>
        <div className='flex flex-wrap items-center gap-2'>
          <span
            className={cn(
              'flex size-6 items-center justify-center rounded-[3px] border',
              toneChipClass[kind.tone],
            )}
          >
            <Icon className='size-3.5' />
          </span>
          <h2 className='text-sm font-semibold text-content-primary'>
            {observation.name || 'Untitled observation'}
          </h2>
          <Badge tone={kind.tone} className='rounded-[3px] px-1.5 text-[10px]'>
            {kind.label}
          </Badge>
          {observation.level && observation.level.toUpperCase() !== 'DEFAULT' && (
            <Badge
              tone={observation.level.toUpperCase() === 'ERROR' ? 'red' : 'amber'}
              className='rounded-[3px] px-1.5 font-mono text-[10px] uppercase'
            >
              {observation.level}
            </Badge>
          )}
          <ScoreBadges scores={scores} />
        </div>
        <div className='mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[10.5px] text-content-secondary'>
          <span className='font-mono text-content-tertiary'>{observation.id}</span>
          <span>{formatDate(observation.startTime ?? observation.timestamp)}</span>
          <span className='font-mono'>
            {pending
              ? 'running'
              : formatDuration(observation.duration ?? observationDurationMs(observation))}
          </span>
          {tokens.total !== undefined && (
            <span title={`input ${tokens.input ?? '—'} · output ${tokens.output ?? '—'}`}>
              {formatTokens(tokens.total)} tokens
            </span>
          )}
          {observation.cost !== undefined && <span>{formatCost(observation.cost)}</span>}
          {observation.model && (
            <span className='flex items-center gap-1'>
              <Bot className='size-3' />
              {observation.model}
            </span>
          )}
        </div>
        {observation.statusMessage && (
          <p className='mt-2 rounded-[3px] border border-border-danger bg-surface-error px-2.5 py-1.5 text-[11px] text-content-error'>
            {observation.statusMessage}
          </p>
        )}
      </div>
      {tab === 'preview' && (
        <div className='grid gap-3 lg:grid-cols-2'>
          <IOPreview label='Input' value={observation.input} />
          <IOPreview label='Output' value={observation.output} />
        </div>
      )}
      {tab === 'attributes' && (
        <div className={cn(panelSurface, 'rounded-[4px] p-3')}>
          <h4 className='mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
            Details
          </h4>
          <dl className='grid gap-x-6 gap-y-2 sm:grid-cols-2'>
            {detailRows.map(({ label, value, mono }) => (
              <div key={label} className='min-w-0'>
                <dt className='font-mono text-[9.5px] uppercase tracking-[0.08em] text-content-tertiary'>
                  {label}
                </dt>
                <dd
                  className={cn(
                    'truncate text-xs text-content-primary',
                    mono && 'font-mono text-[11px]',
                  )}
                  title={value}
                >
                  {value}
                </dd>
              </div>
            ))}
          </dl>
          {(hasParams || hasMetadata) && (
            <div className='mt-3 flex flex-col gap-2'>
              {hasParams && (
                <div>
                  <h4 className='mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
                    Model parameters
                  </h4>
                  <pre className='max-h-40 overflow-auto rounded-[3px] border border-border-primary bg-surface-tertiary p-2.5 font-mono text-[10.5px] text-content-secondary'>
                    {prettyJson(observation.modelParameters)}
                  </pre>
                </div>
              )}
              {hasMetadata && (
                <div>
                  <h4 className='mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
                    Metadata
                  </h4>
                  <pre className='max-h-40 overflow-auto rounded-[3px] border border-border-primary bg-surface-tertiary p-2.5 font-mono text-[10.5px] text-content-secondary'>
                    {prettyJson(observation.metadata)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </div>
      )}
      {tab === 'scores' && (
        <div className={cn(panelSurface, 'rounded-[4px] p-3')}>
          <h4 className='mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
            Scores
          </h4>
          <ScoreRows scores={scores} emptyHint='No scores on this observation.' />
        </div>
      )}
      {tab === 'log' && (
        <LogTab
          observations={observations}
          originMs={originMs}
          selectedId={selectedId}
          onSelect={onSelect}
        />
      )}
    </div>
  )
}

export function TraceDetailPage() {
  const { traceId } = useParams({ from: '/traces/$traceId' })
  const { obs, view: viewParam, tab: tabParam } = useSearch({ from: '/traces/$traceId' })
  const navigate = useNavigate()
  // URL is the single source of truth for selection (`?obs=`), view
  // (`?view=tree|timeline|graph`) and detail tab (`?tab=`) — per-view
  // deep links survive reloads and can be shared. Params are passed
  // explicitly; without them a search update navigates to
  // /traces/undefined.
  const view: ViewMode = viewParam ?? 'tree'
  const tab: DetailTab = tabParam ?? 'preview'
  const selectedId = obs ?? 'trace'
  const patchSearch = (patch: { obs?: string | null; view?: ViewMode; tab?: DetailTab }) => {
    void navigate({
      to: '/traces/$traceId',
      params: { traceId },
      search: {
        obs: 'obs' in patch ? patch.obs || undefined : (obs ?? undefined),
        view:
          'view' in patch
            ? patch.view === 'tree'
              ? undefined
              : patch.view
            : (viewParam ?? undefined),
        tab:
          'tab' in patch
            ? patch.tab === 'preview'
              ? undefined
              : patch.tab
            : (tabParam ?? undefined),
      },
      replace: true,
    })
  }
  const setSelectedId = (id: string) => patchSearch({ obs: id })
  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => getTrace(traceId),
    enabled: Boolean(traceId),
    // Poll while any observation is still running.
    refetchInterval: (query) =>
      query.state.data?.observations?.some(observationIsPending) ? 3000 : false,
  })
  const scoresQuery = useQuery({
    queryKey: ['scores', traceId],
    queryFn: () => getScores({ traceId, limit: 200 }),
    enabled: Boolean(traceId),
  })
  const trace = traceQuery.data
  const scores = scoresQuery.data?.data ?? []

  const roots = useMemo(
    () => buildObservationTree(trace?.observations ?? []),
    [trace?.observations],
  )
  const timeWindow = useMemo(
    () => traceWindow(roots, trace?.startTime ?? trace?.timestamp, trace?.endTime),
    [roots, trace?.startTime, trace?.timestamp, trace?.endTime],
  )
  const observations = trace?.observations ?? []
  const selected = useMemo(
    () =>
      selectedId === 'trace'
        ? undefined
        : observations.find((observation) => observation.id === selectedId),
    [selectedId, observations],
  )
  const totalTokens = useMemo(() => {
    const sum = observations.reduce(
      (acc, observation) => acc + (observationTokens(observation).total ?? 0),
      0,
    )
    return trace?.totalTokens ?? (sum > 0 ? sum : undefined)
  }, [observations, trace?.totalTokens])
  // Trace duration often isn't stored server-side — fall back to the
  // observation window.
  const traceDurationMs =
    trace?.duration ?? trace?.latency ?? timeWindow.endMs - timeWindow.originMs

  // A single observation has nothing to relate — the graph needs ≥2 nodes.
  const graphDisabled = observations.length < 2
  const activeView: ViewMode = view === 'graph' && graphDisabled ? 'tree' : view
  const detailScores = useMemo(
    () =>
      selected
        ? scores.filter((s) => s.observationId === selected.id)
        : scores.filter((s) => !s.observationId),
    [scores, selected],
  )
  const tabs: TabBarTab<DetailTab>[] = [
    { id: 'preview', label: 'Preview' },
    { id: 'attributes', label: 'Attributes' },
    { id: 'scores', label: 'Scores', count: detailScores.length },
    { id: 'log', label: 'Log view', count: observations.length },
  ]

  const exportTrace = () => {
    if (!trace) return
    const blob = new Blob([JSON.stringify({ ...trace, scores }, null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `trace-${trace.id}.json`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className='flex min-h-full flex-col bg-background-primary text-content-primary'>
      {/* ── header ─────────────────────────────────────────── */}
      <div className='border-b border-border-primary bg-background-primary px-4 py-4 sm:px-6'>
        <Link
          to='/traces'
          className='mb-3 inline-flex items-center gap-1 text-xs font-medium text-content-secondary hover:text-content-brand hover:underline'
        >
          <ArrowLeft className='size-3' />
          Back to traces
        </Link>
        {traceQuery.isLoading ? (
          <>
            <Skeleton className='h-7 w-60' />
            <Skeleton className='mt-2 h-3.5 w-44' />
          </>
        ) : traceQuery.isError ? (
          <Alert variant='destructive' className='max-w-2xl rounded-[4px]'>
            <AlertCircle />
            <AlertTitle>Could not load this trace</AlertTitle>
            <AlertDescription>
              {traceQuery.error instanceof Error
                ? traceQuery.error.message
                : 'The local API returned an error.'}
            </AlertDescription>
            <AlertAction>
              <Button variant='outline' size='sm' onClick={() => void traceQuery.refetch()}>
                <RefreshCw data-icon='inline-start' />
                Try again
              </Button>
            </AlertAction>
          </Alert>
        ) : trace ? (
          <div className='flex flex-col justify-between gap-3 sm:flex-row sm:items-start'>
            <div className='min-w-0'>
              <div className='flex flex-wrap items-center gap-2'>
                <h1 className='max-w-2xl truncate text-xl font-semibold tracking-[-0.02em] text-content-primary'>
                  {trace.name || 'Untitled trace'}
                </h1>
                <StatusChip status={trace.status} />
                <ScoreBadges scores={scores.filter((s) => !s.observationId)} />
              </div>
              <p className='mt-1.5 flex items-center gap-2 font-mono text-[10px] text-content-tertiary'>
                <Hash className='size-3' />
                {trace.id}
              </p>
            </div>
            <div className='flex gap-2'>
              <Button variant='outline' size='sm' className='rounded-[3px]' onClick={exportTrace}>
                <Download data-icon='inline-start' />
                Export
              </Button>
              <Button
                variant='ghost'
                size='sm'
                className='text-content-secondary'
                onClick={() => void traceQuery.refetch()}
              >
                <RefreshCw data-icon='inline-start' />
                Refresh
              </Button>
            </div>
          </div>
        ) : null}
        {trace && (
          <div className='mt-4'>
            <MetricStrip className='grid-cols-2 sm:grid-cols-4'>
              <MetricCell label='Duration' value={formatDuration(traceDurationMs)} />
              <MetricCell
                label='Observations'
                value={trace.observationCount ?? observations.length}
              />
              <MetricCell label='Tokens' value={formatTokens(totalTokens)} />
              <MetricCell label='Cost' value={formatCost(trace.totalCost)} />
            </MetricStrip>
          </div>
        )}
      </div>

      {/* ── two-pane workspace ── */}
      {trace && (
        <div className='grid min-h-0 flex-1 gap-0 xl:grid-cols-[380px_minmax(0,1fr)]'>
          <aside className='flex min-h-[320px] flex-col border-b border-border-primary bg-surface-primary xl:border-b-0 xl:border-r'>
            <div className='border-b border-border-primary px-2.5 py-2'>
              <SegmentedControl
                ariaLabel='Trace view'
                value={activeView}
                onChange={(next) => patchSearch({ view: next })}
                options={[
                  { value: 'tree', label: 'Tree' },
                  { value: 'timeline', label: 'Timeline' },
                  {
                    value: 'graph',
                    label: 'Graph',
                    disabled: graphDisabled,
                    title: 'Nothing to graph — this trace has a single node',
                  },
                ]}
              />
            </div>
            {activeView === 'tree' && (
              <TraceNav
                trace={trace}
                roots={roots}
                selectedId={selectedId}
                onSelect={setSelectedId}
                timeWindow={timeWindow}
                durationMs={traceDurationMs}
              />
            )}
            {activeView === 'timeline' && (
              <TimelineView
                roots={roots}
                timeWindow={timeWindow}
                selectedId={selectedId}
                onSelect={setSelectedId}
              />
            )}
            {activeView === 'graph' && (
              <GraphView roots={roots} selectedId={selectedId} onSelect={setSelectedId} />
            )}
          </aside>
          <main className='flex min-w-0 flex-col'>
            <TabBar
              ariaLabel='Detail sections'
              tabs={tabs}
              active={tab}
              onChange={(next) => patchSearch({ tab: next })}
            />
            <div className='min-h-0 flex-1 px-4 py-4 sm:px-5'>
              {selected ? (
                <ObservationDetailView
                  observation={selected}
                  scores={detailScores}
                  tab={tab}
                  observations={observations}
                  originMs={timeWindow.originMs}
                  selectedId={selectedId}
                  onSelect={setSelectedId}
                />
              ) : (
                <TraceDetailView
                  trace={trace}
                  scores={detailScores}
                  tab={tab}
                  observations={observations}
                  originMs={timeWindow.originMs}
                  selectedId={selectedId}
                  onSelect={setSelectedId}
                  totalTokens={totalTokens}
                  durationMs={traceDurationMs}
                />
              )}
            </div>
          </main>
        </div>
      )}
    </div>
  )
}
