import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowRight,
  BarChart3,
  CheckCircle2,
  Clock3,
  Database,
  Gauge,
  Sparkles,
  TerminalSquare,
  Zap,
} from 'lucide-react'
import { getTraces } from '../lib/api'
import { formatCost, formatDuration, formatRelative } from '../lib/utils'
import { Badge, Button, CardContent, CardFooter, Skeleton } from '../components/ui'
import { Panel, MetricCell, MetricStrip, PageHeader, StatusChip, EmptyPanel } from './primitives'

export function DashboardPage() {
  const tracesQuery = useQuery({
    queryKey: ['dashboard-traces'],
    queryFn: () => getTraces({ page: 1, limit: 8 }),
    staleTime: 15_000,
  })
  const traces = tracesQuery.data?.data ?? []
  const totalCost = traces.reduce((sum, trace) => sum + (trace.totalCost ?? 0), 0)
  const avgLatency = traces.length
    ? traces.reduce((sum, trace) => sum + (trace.latency ?? trace.duration ?? 0), 0) / traces.length
    : 0

  return (
    <div className='min-h-full bg-background-primary text-content-primary'>
      <PageHeader
        eyebrow='Workspace'
        title='Overview'
        description='A focused view of activity in this local SQLite project.'
        actions={
          <Button
            render={<Link to='/traces' />}
            size='sm'
            className='bg-interactive-primary text-white hover:bg-interactive-primary-hover'
          >
            Explore traces
            <ArrowRight data-icon='inline-end' />
          </Button>
        }
      />
      <div className='flex flex-col gap-5 px-4 py-5 sm:px-6'>
        <MetricStrip>
          <MetricCell
            icon={Activity}
            label='Traces'
            value={tracesQuery.isLoading ? '—' : (tracesQuery.data?.total ?? 0).toLocaleString()}
            detail='All recorded runs'
          />
          <MetricCell
            icon={Zap}
            label='Average latency'
            value={traces.length ? formatDuration(avgLatency) : '—'}
            detail='Recent traces'
          />
          <MetricCell
            icon={BarChart3}
            label='Cost'
            value={traces.length ? formatCost(totalCost) : '—'}
            detail='Current page'
          />
          <MetricCell
            icon={Database}
            label='Storage'
            value='SQLite'
            detail='Embedded and portable'
          />
        </MetricStrip>

        <div className='grid gap-5 xl:grid-cols-[minmax(0,1fr)_300px]'>
          <Panel
            title='Recent traces'
            description='The latest runs received by the local collector'
            action={
              <Link to='/traces' className='text-xs font-medium text-content-brand hover:underline'>
                See all
              </Link>
            }
          >
            {tracesQuery.isLoading ? (
              <CardContent className='flex flex-col gap-4 px-4 py-4'>
                {[1, 2, 3, 4].map((item) => (
                  <div className='flex items-center gap-3' key={item}>
                    <Skeleton className='size-7 rounded-[3px]' />
                    <div className='flex min-w-0 flex-1 flex-col gap-1.5'>
                      <Skeleton className='h-3.5 w-44' />
                      <Skeleton className='h-2.5 w-28' />
                    </div>
                    <Skeleton className='h-3 w-16' />
                  </div>
                ))}
              </CardContent>
            ) : traces.length === 0 ? (
              <EmptyPanel
                icon={<Activity className='size-4' />}
                title='Your workspace is quiet'
                description='Send a trace to the local API to see activity here.'
                action={
                  <Button render={<Link to='/traces' />} variant='outline' size='sm'>
                    Open traces
                  </Button>
                }
              />
            ) : (
              <div className='divide-y divide-border-primary'>
                {traces.slice(0, 6).map((trace) => (
                  <Link
                    key={trace.id}
                    to='/traces/$traceId'
                    params={{ traceId: trace.id }}
                    className='flex items-center gap-3 px-4 py-3 transition-colors hover:bg-surface-tertiary'
                  >
                    <span className='flex size-7 shrink-0 items-center justify-center rounded-[3px] border border-border-secondary bg-surface-secondary text-content-brand'>
                      <Sparkles className='size-3.5' />
                    </span>
                    <div className='min-w-0 flex-1'>
                      <p className='truncate text-xs font-medium text-content-primary'>
                        {trace.name || 'Untitled trace'}
                      </p>
                      <p className='mt-0.5 truncate font-mono text-[10px] text-content-tertiary'>
                        {trace.id}
                      </p>
                    </div>
                    <div className='hidden shrink-0 items-center gap-5 text-right sm:flex'>
                      <div>
                        <p className='text-xs tabular-nums text-content-secondary'>
                          {formatDuration(trace.duration ?? trace.latency)}
                        </p>
                        <p className='mt-0.5 text-[10px] text-content-tertiary'>latency</p>
                      </div>
                      <div className='min-w-16'>
                        <p className='text-xs text-content-secondary'>
                          {formatRelative(trace.timestamp ?? trace.startTime)}
                        </p>
                      </div>
                    </div>
                    <StatusChip status={trace.status} />
                  </Link>
                ))}
              </div>
            )}
          </Panel>

          <Panel title='Get started' description='Connect an agent in a few steps'>
            <CardContent className='flex flex-col gap-1 px-3 py-3'>
              <StartStep
                number='01'
                icon={TerminalSquare}
                title='Start Pocketfuse'
                description='Run the single binary locally.'
              />
              <StartStep
                number='02'
                icon={Gauge}
                title='Send a trace'
                description='Point your SDK at /api/ingest.'
              />
              <StartStep
                number='03'
                icon={Clock3}
                title='Inspect the timeline'
                description='Review spans and generations.'
              />
            </CardContent>
            <CardFooter className='border-t border-border-primary bg-transparent p-3'>
              <Link
                to='/traces'
                className='flex w-full items-center justify-between rounded-[3px] border border-border-primary bg-surface-tertiary px-3 py-2.5 text-xs font-medium text-content-secondary transition-colors hover:border-border-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
              >
                Open trace explorer
                <ArrowRight className='size-3.5' />
              </Link>
            </CardFooter>
          </Panel>
        </div>

        <div className='flex items-center gap-2 text-[11px] text-content-tertiary'>
          <CheckCircle2 className='size-3.5 text-content-success' />
          Local collector ready. Data stays in the configured SQLite file.
        </div>
      </div>
    </div>
  )
}

function StartStep({
  number,
  icon: Icon,
  title,
  description,
}: {
  number: string
  icon: typeof Activity
  title: string
  description: string
}) {
  return (
    <div className='flex gap-3 rounded-[3px] px-2 py-2.5 transition-colors hover:bg-surface-tertiary'>
      <Badge
        tone='neutral'
        className='mt-0.5 size-6 shrink-0 rounded-[3px] border-border-primary bg-surface-tertiary px-0 font-mono text-[10px] text-content-tertiary'
      >
        {number}
      </Badge>
      <div className='min-w-0'>
        <p className='flex items-center gap-1.5 text-xs font-medium text-content-primary'>
          <Icon className='size-3.5 text-content-brand' />
          {title}
        </p>
        <p className='mt-1 text-[11px] leading-4 text-content-secondary'>{description}</p>
      </div>
    </div>
  )
}
