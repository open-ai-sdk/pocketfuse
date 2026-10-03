import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from '@tanstack/react-router'
import {
  Activity,
  AlertCircle,
  ArrowLeft,
  Bot,
  Check,
  ChevronDown,
  ChevronRight,
  CircleDot,
  Clock3,
  Copy,
  Database,
  FileText,
  GitBranch,
  Hash,
  MessageSquare,
  RefreshCw,
  Sparkles,
  Star,
  UserRound,
  Wrench,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { getTrace, type Observation, type Trace } from '../lib/api'
import { cn, formatCost, formatDate, formatDuration, formatTokens, prettyJson } from '../lib/utils'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  Separator,
  Skeleton,
} from '../components/ui'
import { MetricCell, MetricStrip, Panel, StatusChip, panelSurface } from './primitives'

function kindFor(observation: Observation) {
  const type = observation.type?.toLowerCase()
  if (type?.includes('generation') || type?.includes('llm'))
    return {
      label: 'Generation',
      icon: Sparkles,
      tone: 'violet' as const,
      iconClass: 'border-border-secondary bg-surface-accent text-content-accent',
    }
  if (type?.includes('span') || type?.includes('chain'))
    return {
      label: 'Span',
      icon: GitBranch,
      tone: 'blue' as const,
      iconClass: 'border-border-information bg-surface-information text-content-information',
    }
  if (type?.includes('event'))
    return {
      label: 'Event',
      icon: CircleDot,
      tone: 'amber' as const,
      iconClass: 'border-border-warning bg-surface-warning text-content-warning',
    }
  return {
    label: observation.type || 'Observation',
    icon: Wrench,
    tone: 'neutral' as const,
    iconClass: 'border-border-primary bg-surface-tertiary text-content-secondary',
  }
}

function TimelineItem({ observation, depth }: { observation: Observation; depth: number }) {
  const [open, setOpen] = useState(true)
  const kind = kindFor(observation)
  const Icon = kind.icon
  const [copied, setCopied] = useState<string | null>(null)
  const copy = async (value: unknown, name: string) => {
    try {
      await navigator.clipboard.writeText(prettyJson(value))
      setCopied(name)
      window.setTimeout(() => setCopied(null), 1300)
    } catch {
      /* clipboard can be unavailable in HTTP local mode */
    }
  }
  return (
    <div className='relative' style={{ marginLeft: `${Math.min(depth, 3) * 22}px` }}>
      <Separator
        orientation='vertical'
        className='absolute -left-4 top-5 bottom-0 bg-border-primary'
      />
      <Card
        className={cn(
          'relative mb-3 gap-0 rounded-[4px] border-border-primary bg-surface-primary p-0 shadow-none ring-0',
          open && 'border-border-secondary',
        )}
      >
        <CardHeader className='flex min-h-[56px] flex-row items-center gap-2.5 px-3.5 py-3'>
          <Button
            variant='ghost'
            size='icon-xs'
            onClick={() => setOpen((value) => !value)}
            aria-label={open ? 'Collapse observation' : 'Expand observation'}
            aria-expanded={open}
            className='text-content-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
          >
            {open ? <ChevronDown /> : <ChevronRight />}
          </Button>
          <div
            className={cn(
              'flex size-7 shrink-0 items-center justify-center rounded-[3px] border',
              kind.iconClass,
            )}
          >
            <Icon className='size-3.5' />
          </div>
          <div className='min-w-0 flex-1'>
            <div className='flex flex-wrap items-center gap-1.5'>
              <span className='truncate text-xs font-medium text-content-primary'>
                {observation.name || 'Untitled observation'}
              </span>
              <Badge tone={kind.tone} className='h-5 rounded-[3px] px-1.5 text-[10px]'>
                {kind.label}
              </Badge>
              {observation.level && (
                <Badge
                  tone={observation.level.toLowerCase() === 'error' ? 'red' : 'neutral'}
                  className='h-5 rounded-[3px] px-1.5 font-mono text-[10px] uppercase'
                >
                  {observation.level}
                </Badge>
              )}
            </div>
            <div className='mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-content-tertiary'>
              <span className='font-mono'>{observation.id.slice(0, 12)}</span>
              {observation.model && (
                <span className='flex items-center gap-1'>
                  <Bot className='size-3' />
                  {observation.model}
                </span>
              )}
            </div>
          </div>
          <div className='hidden shrink-0 text-right sm:block'>
            <p className='font-mono text-[11px] tabular-nums text-content-secondary'>
              {formatDuration(observation.duration)}
            </p>
            <p className='mt-1 text-[10px] text-content-tertiary'>
              {formatDate(observation.startTime ?? observation.timestamp)}
            </p>
          </div>
        </CardHeader>
        {open && (
          <CardContent className='border-t border-border-primary bg-surface-tertiary p-3.5'>
            <div className='grid gap-3 lg:grid-cols-2'>
              <DataBlock
                label='Input'
                value={observation.input}
                copyKey='input'
                copied={copied}
                onCopy={copy}
              />
              <DataBlock
                label='Output'
                value={observation.output}
                copyKey='output'
                copied={copied}
                onCopy={copy}
              />
            </div>
            {observation.metadata && Object.keys(observation.metadata).length > 0 && (
              <div className='mt-3'>
                <DataBlock
                  label='Metadata'
                  value={observation.metadata}
                  copyKey='metadata'
                  copied={copied}
                  onCopy={copy}
                />
              </div>
            )}
          </CardContent>
        )}
      </Card>
    </div>
  )
}

function DataBlock({
  label,
  value,
  copyKey,
  copied,
  onCopy,
}: {
  label: string
  value: unknown
  copyKey: string
  copied: string | null
  onCopy: (value: unknown, name: string) => void
}) {
  const content = prettyJson(value)
  return (
    <Card
      size='sm'
      className='min-w-0 gap-2 rounded-[4px] border-border-primary bg-surface-primary p-0 shadow-none ring-0'
    >
      <CardHeader className='flex flex-row items-center justify-between px-3 pt-2.5 pb-0'>
        <CardTitle className='font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
          {label}
        </CardTitle>
        {value !== undefined && value !== null && (
          <Button
            variant='ghost'
            size='xs'
            onClick={() => onCopy(value, copyKey)}
            aria-label={`Copy ${label.toLowerCase()}`}
            className='text-content-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
          >
            {copied === copyKey ? (
              <>
                <Check data-icon='inline-start' />
                Copied
              </>
            ) : (
              <>
                <Copy data-icon='inline-start' />
                Copy
              </>
            )}
          </Button>
        )}
      </CardHeader>
      <CardContent className='px-3 pb-3'>
        <pre className='max-h-64 min-h-12 overflow-auto whitespace-pre-wrap break-words rounded-[3px] border border-border-primary bg-surface-tertiary p-2.5 font-mono text-[11px] leading-5 text-content-secondary'>
          {content || <span className='text-content-tertiary'>No data</span>}
        </pre>
      </CardContent>
    </Card>
  )
}

function TraceOverview({ trace }: { trace: Trace }) {
  return (
    <MetricStrip className='grid-cols-2 sm:grid-cols-4'>
      <MetricCell label='Duration' value={formatDuration(trace.duration ?? trace.latency)} />
      <MetricCell
        label='Observations'
        value={trace.observationCount ?? trace.observations?.length ?? 0}
      />
      <MetricCell label='Tokens' value={formatTokens(trace.totalTokens)} />
      <MetricCell label='Cost' value={formatCost(trace.totalCost)} />
    </MetricStrip>
  )
}

function traceDepths(observations: Observation[]) {
  const byId = new Map(observations.map((observation) => [observation.id, observation]))
  const depths = new Map<string, number>()
  const resolve = (observation: Observation, seen = new Set<string>()): number => {
    if (depths.has(observation.id)) return depths.get(observation.id) ?? 0
    if (!observation.parentObservationId || seen.has(observation.id)) {
      depths.set(observation.id, 0)
      return 0
    }
    seen.add(observation.id)
    const parent = byId.get(observation.parentObservationId)
    const depth = parent ? resolve(parent, seen) + 1 : 0
    depths.set(observation.id, depth)
    return depth
  }
  observations.forEach((observation) => resolve(observation))
  return depths
}

export function TraceDetailPage() {
  const { traceId } = useParams({ from: '/traces/$traceId' })
  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => getTrace(traceId),
    enabled: Boolean(traceId),
  })
  const trace = traceQuery.data
  const observations = useMemo(
    () =>
      [...(trace?.observations ?? [])].sort(
        (a, b) =>
          new Date(a.startTime ?? a.timestamp ?? '').getTime() -
          new Date(b.startTime ?? b.timestamp ?? '').getTime(),
      ),
    [trace?.observations],
  )
  const depths = useMemo(() => traceDepths(observations), [observations])

  return (
    <div className='min-h-full bg-background-primary text-content-primary'>
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
          <>
            <div className='flex flex-col justify-between gap-3 sm:flex-row sm:items-start'>
              <div>
                <div className='flex flex-wrap items-center gap-2'>
                  <h1 className='max-w-2xl truncate text-xl font-semibold tracking-[-0.02em] text-content-primary'>
                    {trace.name || 'Untitled trace'}
                  </h1>
                  <StatusChip status={trace.status} />
                </div>
                <p className='mt-1.5 flex items-center gap-2 font-mono text-[10px] text-content-tertiary'>
                  <Hash className='size-3' />
                  {trace.id}
                </p>
              </div>
              <div className='flex gap-2'>
                <Button variant='outline' size='sm' className='rounded-[3px]'>
                  <Star data-icon='inline-start' />
                  Bookmark
                </Button>
                <Button variant='outline' size='sm' className='rounded-[3px]'>
                  <FileText data-icon='inline-start' />
                  Export
                </Button>
              </div>
            </div>
            <div className='mt-4'>
              <TraceOverview trace={trace} />
            </div>
          </>
        ) : null}
      </div>
      {trace && (
        <div className='grid gap-5 px-4 py-5 sm:px-6 xl:grid-cols-[minmax(0,1fr)_280px]'>
          <section>
            <div className='mb-3 flex items-center justify-between'>
              <div>
                <h2 className='text-sm font-semibold text-content-primary'>Trace timeline</h2>
                <p className='mt-0.5 text-[11px] text-content-secondary'>
                  {observations.length} observation{observations.length === 1 ? '' : 's'} recorded
                </p>
              </div>
              <Button
                variant='ghost'
                size='sm'
                onClick={() => void traceQuery.refetch()}
                className='text-content-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
              >
                <RefreshCw data-icon='inline-start' />
                Refresh
              </Button>
            </div>
            {observations.length === 0 ? (
              <Card className={cn(panelSurface, 'p-0')}>
                <EmptyState
                  icon={<Activity className='size-4' />}
                  title='No observations'
                  description='This trace has been created, but no observations are attached yet.'
                />
              </Card>
            ) : (
              <div className='relative ml-4 pl-4'>
                <Separator
                  orientation='vertical'
                  className='absolute inset-y-0 left-0 bg-border-primary'
                />
                {observations.map((observation) => (
                  <TimelineItem
                    key={observation.id}
                    observation={observation}
                    depth={depths.get(observation.id) ?? 0}
                  />
                ))}
              </div>
            )}
          </section>
          <TraceSidebar trace={trace} />
        </div>
      )}
    </div>
  )
}

function TraceSidebar({ trace }: { trace: Trace }) {
  const entries = [
    { label: 'Project', value: trace.project, icon: Database },
    { label: 'Session', value: trace.sessionId, icon: MessageSquare },
    { label: 'User', value: trace.userId, icon: UserRound },
    { label: 'Started', value: formatDate(trace.startTime ?? trace.timestamp), icon: Clock3 },
    { label: 'Release', value: trace.release ?? trace.version, icon: GitBranch },
  ]
  return (
    <aside className='flex flex-col gap-4'>
      <Panel title='Trace details'>
        <CardContent className='px-4 pb-3'>
          <div className='flex flex-col'>
            {entries.map(({ label, value, icon: Icon }, index) => (
              <div key={label}>
                <div className='flex items-start gap-2.5 py-2.5 first:pt-0 last:pb-0'>
                  <Icon className='mt-0.5 size-3.5 shrink-0 text-content-secondary' />
                  <div className='min-w-0'>
                    <p className='font-mono text-[10px] uppercase tracking-[0.08em] text-content-tertiary'>
                      {label}
                    </p>
                    <p className='mt-0.5 truncate text-xs text-content-primary'>{value || '—'}</p>
                  </div>
                </div>
                {index < entries.length - 1 && <Separator className='bg-border-primary' />}
              </div>
            ))}
          </div>
        </CardContent>
      </Panel>
      {(trace.input !== undefined || trace.output !== undefined) && (
        <Panel title='Trace input / output'>
          <CardContent className='flex flex-col gap-3 px-3 pb-3'>
            <DataBlock
              label='Input'
              value={trace.input}
              copyKey='trace-input'
              copied={null}
              onCopy={() => undefined}
            />
            <DataBlock
              label='Output'
              value={trace.output}
              copyKey='trace-output'
              copied={null}
              onCopy={() => undefined}
            />
          </CardContent>
        </Panel>
      )}
      {trace.tags && trace.tags.length > 0 && (
        <Panel title='Tags'>
          <CardContent className='flex flex-wrap gap-1.5 px-4 pb-3'>
            {trace.tags.map((tag) => (
              <Badge
                key={tag}
                tone='neutral'
                className='rounded-[3px] border-border-primary bg-surface-tertiary font-mono text-[10px] text-content-secondary'
              >
                {tag}
              </Badge>
            ))}
          </CardContent>
        </Panel>
      )}
    </aside>
  )
}
