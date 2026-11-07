import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { ArrowUpRight, ChevronRight, TriangleAlert, X } from 'lucide-react'
import { useEffect } from 'react'
import { ActivityRow, TurnCard } from '../components/session/turn-card'
import { sessionLabel } from '../components/trace/linked-trace-list'
import { ObservationDetail, TraceDetail } from '../components/trace/span-detail'
import { Button } from '../components/ui'
import { sessionStoryOptions, traceOptions, traceScoresOptions } from '../lib/queries'
import { cn, formatCost, formatDate, formatDuration, formatUsage } from '../lib/utils'

// The side panel reads one span or turn without leaving the conversation.
function DetailPanel({
  traceId,
  observationId,
  onSelect,
  onClose,
}: {
  traceId: string
  observationId?: string
  onSelect: (id?: string) => void
  onClose: () => void
}) {
  const trace = useQuery(traceOptions(traceId))
  const scoreList = useQuery(traceScoresOptions(traceId))
  const scores = scoreList.data?.data ?? []
  const observation = observationId
    ? trace.data?.observations?.find((item) => item.id === observationId)
    : undefined
  const step = trace.data?.turn?.steps.find(
    (item) => item.observationId === (observation?.mergedInto ?? observation?.id),
  )
  return (
    <aside className='fixed inset-y-0 right-0 z-40 flex w-full max-w-2xl min-w-0 flex-col border-l border-border-primary bg-background-primary xl:static xl:z-auto xl:max-w-none'>
      <div className='flex shrink-0 items-center justify-between gap-2 border-b border-border-primary px-3 py-1.5'>
        <Link
          to='/traces/$traceId'
          params={{ traceId }}
          search={{ obs: observationId }}
          className='inline-flex items-center gap-0.5 text-[11px] font-medium text-content-brand hover:underline'
        >
          Open in full trace
          <ArrowUpRight className='size-3' />
        </Link>
        <Button
          variant='ghost'
          size='icon-xs'
          onClick={onClose}
          aria-label='Close panel'
          className='text-content-secondary'
        >
          <X />
        </Button>
      </div>
      <div className='min-h-0 flex-1 overflow-auto p-3'>
        {trace.isLoading ? (
          <p className='py-8 text-center text-[11px] text-content-tertiary'>Loading…</p>
        ) : trace.isError || !trace.data ? (
          <p className='py-8 text-center text-[11px] text-content-error'>Could not load this.</p>
        ) : observationId && !observation ? (
          <p className='py-8 text-center text-[11px] text-content-tertiary'>
            This span is not in the trace.
          </p>
        ) : observation ? (
          <ObservationDetail
            key={observation.id}
            observation={observation}
            step={step}
            scores={scores.filter((score) => score.observationId === observation.id)}
            onSelect={onSelect}
          />
        ) : (
          <TraceDetail
            trace={trace.data}
            scores={scores.filter((score) => !score.observationId)}
            totalTokens={trace.data.totalTokens}
          />
        )}
      </div>
    </aside>
  )
}

export function SessionDetailPage() {
  const { sessionId } = useParams({ from: '/sessions/$sessionId' })
  const {
    noise,
    trace: selectedTrace,
    obs: selectedSpan,
  } = useSearch({
    from: '/sessions/$sessionId',
  })
  const navigate = useNavigate()
  const hideNoise = noise !== 'show'
  const { data: story } = useSuspenseQuery(sessionStoryOptions(sessionId, hideNoise))
  const totals = story.totals
  const usage = formatUsage(totals.usage)

  const select = (traceId?: string, observationId?: string) =>
    void navigate({
      to: '/sessions/$sessionId',
      params: { sessionId },
      search: { noise, trace: traceId, obs: observationId },
      replace: true,
    })

  // A link from a trace lands on its turn, not the top of the conversation.
  useEffect(() => {
    if (selectedTrace) {
      document.getElementById(`turn-${selectedTrace}`)?.scrollIntoView({ block: 'nearest' })
    }
  }, [selectedTrace])

  return (
    <div className='flex h-full min-h-0 flex-col text-content-primary'>
      <header className='shrink-0 border-b border-border-primary px-4 py-2.5 sm:px-5'>
        <nav
          aria-label='Breadcrumb'
          className='flex items-center gap-1 text-[11px] text-content-secondary'
        >
          <Link to='/sessions' className='font-medium hover:text-content-brand hover:underline'>
            Sessions
          </Link>
          <ChevronRight className='size-3 text-content-tertiary' />
          <span className='truncate font-mono text-[10px] text-content-tertiary'>
            {sessionLabel(story.id)}
          </span>
        </nav>
        <div className='mt-1 flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5'>
          <h1 className='min-w-0 max-w-3xl truncate text-base font-semibold tracking-[-0.01em]'>
            {story.name || story.title || sessionLabel(story.id)}
          </h1>
          <Button
            variant='ghost'
            size='sm'
            className='shrink-0 text-content-secondary'
            onClick={() =>
              void navigate({
                to: '/sessions/$sessionId',
                params: { sessionId },
                search: {
                  noise: hideNoise ? 'show' : undefined,
                  trace: selectedTrace,
                  obs: selectedSpan,
                },
                replace: true,
              })
            }
          >
            {hideNoise ? `Show polling (${story.hiddenNoise})` : 'Hide polling'}
          </Button>
        </div>
        <div className='mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-content-secondary'>
          <span>
            {totals.turns} turn{totals.turns === 1 ? '' : 's'}
          </span>
          <span>{totals.modelCalls} model calls</span>
          <span>{totals.toolCalls} tool calls</span>
          <span className='font-mono tabular-nums'>{formatDuration(totals.durationMs)}</span>
          {usage && <span>{usage}</span>}
          {totals.cost !== undefined && <span>{formatCost(totals.cost)}</span>}
          {totals.errors > 0 && (
            <span className='inline-flex items-center gap-1 font-medium text-content-error'>
              <TriangleAlert className='size-3' />
              {totals.errors} failed span{totals.errors === 1 ? '' : 's'}
            </span>
          )}
          {totals.failedTools > 0 && (
            <span className='font-medium text-content-error'>
              {totals.failedTools} tool error{totals.failedTools === 1 ? '' : 's'}
            </span>
          )}
          <span className='text-content-tertiary'>{formatDate(totals.startTime)}</span>
          {story.userId && <span className='text-content-tertiary'>user {story.userId}</span>}
        </div>
      </header>
      <div
        className={cn(
          'grid min-h-0 flex-1',
          selectedTrace && 'xl:grid-cols-[minmax(0,1fr)_minmax(460px,44%)]',
        )}
      >
        <main className='min-h-0 min-w-0 overflow-auto px-4 py-3 sm:px-5'>
          <div className='mx-auto flex max-w-4xl flex-col gap-2.5'>
            {story.items.length === 0 && (
              <p className='py-10 text-center text-[11px] text-content-tertiary'>
                Nothing recorded in this session yet.
              </p>
            )}
            {story.items.map((item, index) =>
              item.turn ? (
                <TurnCard
                  key={item.turn.traceId}
                  turn={item.turn}
                  selectedTrace={selectedTrace}
                  selectedSpan={selectedSpan}
                  onSelect={select}
                />
              ) : item.activity ? (
                <ActivityRow
                  key={`activity-${index}`}
                  activity={item.activity}
                  selectedTrace={selectedTrace}
                  onSelect={(traceId) => select(traceId)}
                />
              ) : null,
            )}
            {story.omittedTraces ? (
              <p className='py-2 text-center text-[11px] text-content-tertiary'>
                The oldest {story.omittedTraces} traces of this session are not shown.
              </p>
            ) : null}
          </div>
        </main>
        {selectedTrace && (
          <DetailPanel
            traceId={selectedTrace}
            observationId={selectedSpan}
            onSelect={(id) => select(selectedTrace, id)}
            onClose={() => select()}
          />
        )}
      </div>
    </div>
  )
}
