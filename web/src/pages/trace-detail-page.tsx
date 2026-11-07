import { useSuspenseQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { ChevronRight, Download, RefreshCw, TriangleAlert, UserRound } from 'lucide-react'
import { useMemo } from 'react'
import { GraphView } from '../components/trace/graph-view'
import { kindOf } from '../components/trace/kinds'
import { correlationEntries, sessionLabel } from '../components/trace/linked-trace-list'
import { TraceLogView } from '../components/trace/log-view'
import { SegmentedControl } from '../components/trace/segmented'
import { ObservationDetail, TraceDetail } from '../components/trace/span-detail'
import { TraceTreePanel } from '../components/trace/trace-tree-panel'
import { Badge, Button } from '../components/ui'
import { traceOptions, traceScoresOptions } from '../lib/queries'
import {
  buildObservationTree,
  observationHasError,
  observationTokens,
  traceWindow,
  visibleObservations,
} from '../lib/trace-tree'
import { formatCost, formatDate, formatDuration, formatTokens, formatUsage } from '../lib/utils'
import { StatusChip } from './primitives'

// The left pane explores structure; the right pane reads whatever is selected.
type ViewMode = 'tree' | 'graph' | 'log'

export function TraceDetailPage() {
  const { traceId } = useParams({ from: '/traces/$traceId' })
  const { obs, view: viewParam, raw } = useSearch({ from: '/traces/$traceId' })
  const navigate = useNavigate()
  const view: ViewMode = viewParam ?? 'tree'
  const selectedId = obs ?? 'trace'
  // The URL is the single source of truth, so a span can be linked and survives
  // a reload. Params are passed explicitly: without them a search update
  // navigates to /traces/undefined.
  const patchSearch = (patch: { obs?: string; view?: ViewMode; raw?: 1 | undefined }) => {
    const nextView = 'view' in patch ? patch.view : view
    void navigate({
      to: '/traces/$traceId',
      params: { traceId },
      search: {
        obs: 'obs' in patch ? patch.obs : obs,
        view: nextView === 'tree' ? undefined : nextView,
        raw: 'raw' in patch ? patch.raw : raw,
      },
      replace: true,
    })
  }
  const setSelectedId = (id: string) => patchSearch({ obs: id === 'trace' ? undefined : id })
  const traceQuery = useSuspenseQuery(traceOptions(traceId))
  const scoresQuery = useSuspenseQuery(traceScoresOptions(traceId))
  const trace = traceQuery.data
  const scores = scoresQuery.data?.data ?? []
  const observations = useMemo(() => trace.observations ?? [], [trace.observations])
  const shown = useMemo(() => visibleObservations(observations, raw === 1), [observations, raw])
  const roots = useMemo(() => buildObservationTree(shown), [shown])
  const timeWindow = useMemo(
    () => traceWindow(roots, trace.startTime ?? trace.timestamp, trace.endTime),
    [roots, trace.startTime, trace.timestamp, trace.endTime],
  )
  const selected = observations.find((observation) => observation.id === selectedId)
  const step = trace.turn?.steps.find(
    (item) => item.observationId === (selected?.mergedInto ?? selected?.id),
  )
  const selectedScores = scores.filter((score) =>
    selected ? score.observationId === selected.id : !score.observationId,
  )
  const totalTokens = useMemo(() => {
    const sum = shown.reduce(
      (total, observation) => total + (observationTokens(observation).total ?? 0),
      0,
    )
    return trace.totalTokens ?? (sum > 0 ? sum : undefined)
  }, [shown, trace.totalTokens])
  const durationMs = trace.duration ?? trace.latency ?? timeWindow.endMs - timeWindow.originMs
  const errorCount = observations.filter(observationHasError).length
  const hasMerged = observations.some((observation) => observation.mergedInto)
  const graphDisabled = shown.length < 2
  const activeView: ViewMode = view === 'graph' && graphDisabled ? 'tree' : view
  const kind = trace.turn ? kindOf({ role: trace.turn.kind }) : undefined
  const usage = formatUsage(trace.turn?.usage)

  const exportTrace = () => {
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
    <div className='flex h-full min-h-0 flex-col text-content-primary'>
      <header className='shrink-0 border-b border-border-primary px-4 py-2.5 sm:px-5'>
        <nav
          aria-label='Breadcrumb'
          className='flex min-w-0 items-center gap-1 text-[11px] text-content-secondary'
        >
          <Link to='/traces' className='font-medium hover:text-content-brand hover:underline'>
            Traces
          </Link>
          {trace.sessionId && (
            <>
              <ChevronRight className='size-3 shrink-0 text-content-tertiary' />
              <Link
                to='/sessions/$sessionId'
                params={{ sessionId: trace.sessionId }}
                search={{ trace: trace.id }}
                className='truncate font-medium text-content-brand hover:underline'
              >
                {sessionLabel(trace.sessionId)}
              </Link>
            </>
          )}
          <ChevronRight className='size-3 shrink-0 text-content-tertiary' />
          <span className='truncate font-mono text-[10px] text-content-tertiary'>{trace.id}</span>
        </nav>
        <div className='mt-1 flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5'>
          <div className='flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1'>
            {kind && (
              <Badge tone={kind.tone} className='rounded-[3px] px-1.5 text-[10px]'>
                {kind.label}
              </Badge>
            )}
            <h1 className='max-w-xl truncate text-base font-semibold tracking-[-0.01em] text-content-primary'>
              {trace.name || 'Untitled trace'}
            </h1>
            <StatusChip status={trace.status} />
            <span className='font-mono text-[11px] tabular-nums text-content-secondary'>
              {formatDuration(durationMs)}
            </span>
            <span className='text-[11px] text-content-secondary'>
              {shown.length} span{shown.length === 1 ? '' : 's'}
            </span>
            {(usage || totalTokens !== undefined) && (
              <span className='text-[11px] text-content-secondary'>
                {usage ?? `${formatTokens(totalTokens)} tokens`}
              </span>
            )}
            {trace.totalCost !== undefined && (
              <span className='text-[11px] text-content-secondary'>
                {formatCost(trace.totalCost)}
              </span>
            )}
            {errorCount > 0 && (
              <span className='inline-flex items-center gap-1 text-[11px] font-medium text-content-error'>
                <TriangleAlert className='size-3' />
                {errorCount} failed
              </span>
            )}
            <span className='text-[11px] text-content-tertiary'>
              {formatDate(trace.startTime ?? trace.timestamp)}
            </span>
            {trace.userId && (
              <span className='inline-flex items-center gap-1 text-[11px] text-content-secondary'>
                <UserRound className='size-3' />
                {trace.userId}
              </span>
            )}
            {correlationEntries(trace).map(([key, value]) => (
              <span
                key={key}
                className='font-mono text-[10px] text-content-tertiary'
                title={`${key}: ${value}`}
              >
                {key.replace('_id', '')} {value.slice(0, 8)}
              </span>
            ))}
          </div>
          <div className='flex shrink-0 gap-1'>
            <Button
              variant='ghost'
              size='sm'
              className='text-content-secondary'
              onClick={exportTrace}
            >
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
      </header>

      <div className='grid min-h-0 flex-1 xl:grid-cols-[minmax(520px,48%)_minmax(0,1fr)]'>
        <aside className='flex min-h-[320px] min-w-0 flex-col border-b border-border-primary bg-surface-primary xl:min-h-0 xl:border-b-0 xl:border-r'>
          <div className='border-b border-border-primary px-2.5 py-1.5'>
            <SegmentedControl
              ariaLabel='Trace view'
              value={activeView}
              onChange={(next) => patchSearch({ view: next })}
              options={[
                { value: 'tree', label: 'Tree' },
                {
                  value: 'graph',
                  label: 'Graph',
                  disabled: graphDisabled,
                  title: 'Nothing to graph: this trace has a single span',
                },
                { value: 'log', label: 'Log' },
              ]}
            />
          </div>
          {activeView === 'tree' && (
            <TraceTreePanel
              trace={trace}
              roots={roots}
              selectedId={selectedId}
              onSelect={setSelectedId}
              timeWindow={timeWindow}
              durationMs={durationMs}
              raw={raw === 1}
              onToggleRaw={() => patchSearch({ raw: raw === 1 ? undefined : 1 })}
              hasMerged={hasMerged}
            />
          )}
          {activeView === 'graph' && (
            <GraphView roots={roots} selectedId={selectedId} onSelect={setSelectedId} />
          )}
          {activeView === 'log' && (
            <div className='min-h-0 flex-1 overflow-auto p-1.5'>
              <TraceLogView
                observations={shown}
                originMs={timeWindow.originMs}
                selectedId={selectedId}
                onSelect={setSelectedId}
              />
            </div>
          )}
        </aside>
        <main className='min-h-0 min-w-0 overflow-auto px-4 py-3 sm:px-5'>
          {selected ? (
            <ObservationDetail
              key={selected.id}
              observation={selected}
              step={step}
              scores={selectedScores}
              onSelect={setSelectedId}
            />
          ) : (
            <TraceDetail trace={trace} scores={selectedScores} totalTokens={totalTokens} />
          )}
        </main>
      </div>
    </div>
  )
}
