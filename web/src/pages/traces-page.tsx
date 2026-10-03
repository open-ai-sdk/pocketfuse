import { useQuery } from '@tanstack/react-query'
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from '@tanstack/react-table'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  Coins,
  Plus,
  RefreshCw,
  Search,
  SlidersHorizontal,
  TimerReset,
  Zap,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { type Trace, type TraceFilters, type TraceStatus } from '../lib/api'
import { TRACES_PAGE_SIZE, tracesOptions } from '../lib/queries'
import { cn, formatCost, formatDuration, formatRelative } from '../lib/utils'
import { Button, CardContent, Input, Skeleton } from '../components/ui'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../components/ui/table'
import { Field, FieldLabel } from '../components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../components/ui/select'
import { MetricCell, MetricStrip, PageHeader, Panel, StatusChip, EmptyPanel } from './primitives'

const pageSize = TRACES_PAGE_SIZE

function statusTone(status?: string): 'neutral' | 'green' | 'red' | 'amber' {
  const value = status?.toLowerCase()
  if (value === 'success' || value === 'completed' || value === 'ok') return 'green'
  if (value === 'error' || value === 'failed' || value === 'failure') return 'red'
  if (value === 'pending' || value === 'running') return 'amber'
  return 'neutral'
}

// Time-range presets → server-side `from` bound (traces are listed newest
// first, so a lower bound is all a range needs).
const TIME_RANGES = [
  { id: 'all', label: 'All time' },
  { id: '1h', label: 'Last hour', ms: 3_600_000 },
  { id: '24h', label: 'Last 24 hours', ms: 86_400_000 },
  { id: '7d', label: 'Last 7 days', ms: 604_800_000 },
  { id: '30d', label: 'Last 30 days', ms: 2_592_000_000 },
] as const

type TimeRangeId = (typeof TIME_RANGES)[number]['id']

function rangeFromMs(id: TimeRangeId): number | undefined {
  const range = TIME_RANGES.find((item) => item.id === id)
  return range && 'ms' in range ? Date.now() - range.ms : undefined
}

function TraceTable({ traces }: { traces: Trace[] }) {
  const navigate = useNavigate()
  const columns = useMemo<ColumnDef<Trace>[]>(
    () => [
      {
        id: 'name',
        header: 'Trace',
        accessorFn: (trace) => trace.name ?? trace.id,
        cell: ({ row }) => (
          <div className='min-w-0'>
            <Link
              to='/traces/$traceId'
              params={{ traceId: row.original.id }}
              className='block max-w-[320px] truncate text-xs font-medium text-content-primary hover:text-content-brand hover:underline'
            >
              {row.original.name || 'Untitled trace'}
            </Link>
            <span className='mt-0.5 block max-w-[320px] truncate font-mono text-[10px] text-content-tertiary'>
              {row.original.id}
            </span>
          </div>
        ),
      },
      {
        id: 'status',
        header: 'Status',
        accessorFn: (trace) => trace.status ?? 'unknown',
        cell: ({ getValue }) => <StatusChip status={String(getValue())} />,
      },
      {
        id: 'observations',
        header: 'Observations',
        accessorFn: (trace) => trace.observationCount ?? trace.observations?.length ?? 0,
        cell: ({ getValue }) => (
          <span className='font-mono text-xs tabular-nums text-content-secondary'>
            {String(getValue())}
          </span>
        ),
      },
      {
        id: 'latency',
        header: 'Latency',
        accessorFn: (trace) => trace.latency ?? trace.duration,
        cell: ({ getValue }) => (
          <span className='font-mono text-xs tabular-nums text-content-secondary'>
            {formatDuration(Number(getValue()) || undefined)}
          </span>
        ),
      },
      {
        id: 'cost',
        header: 'Cost',
        accessorFn: (trace) => trace.totalCost,
        cell: ({ getValue }) => (
          <span className='font-mono text-xs tabular-nums text-content-secondary'>
            {formatCost(Number(getValue()) || undefined)}
          </span>
        ),
      },
      {
        id: 'timestamp',
        header: 'Last activity',
        accessorFn: (trace) => trace.timestamp ?? trace.startTime,
        cell: ({ getValue }) => (
          <span className='whitespace-nowrap text-xs text-content-secondary'>
            {formatRelative(String(getValue() || ''))}
          </span>
        ),
      },
    ],
    [],
  )
  const table = useReactTable({ data: traces, columns, getCoreRowModel: getCoreRowModel() })
  return (
    <div className='overflow-x-auto'>
      <Table className='w-full min-w-[800px]'>
        <TableHeader>
          <TableRow className='border-b border-border-primary bg-surface-tertiary hover:bg-surface-tertiary'>
            {table.getHeaderGroups()[0].headers.map((header) => (
              <TableHead
                key={header.id}
                className='h-9 px-4 text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary first:pl-4'
              >
                {flexRender(header.column.columnDef.header, header.getContext())}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row) => (
            <TableRow
              key={row.id}
              onClick={() =>
                navigate({ to: '/traces/$traceId', params: { traceId: row.original.id } })
              }
              className='cursor-pointer border-b border-border-primary transition-colors hover:bg-surface-tertiary'
            >
              {row.getVisibleCells().map((cell) => (
                <TableCell key={cell.id} className='h-[58px] px-4 text-xs first:pl-4'>
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function TableLoading() {
  return (
    <div className='divide-y divide-border-primary'>
      {Array.from({ length: 7 }).map((_, index) => (
        <div key={index} className='flex h-[58px] items-center gap-5 px-4'>
          <Skeleton className='h-3.5 w-44' />
          <Skeleton className='h-5 w-16' />
          <Skeleton className='h-3.5 w-20' />
          <Skeleton className='h-3.5 w-14' />
          <Skeleton className='h-3.5 w-16' />
          <Skeleton className='ml-auto h-3.5 w-24' />
        </div>
      ))}
    </div>
  )
}

export function TracesPage() {
  const [search, setSearch] = useState('')
  const [project, setProject] = useState('')
  const [status, setStatus] = useState<TraceStatus | ''>('')
  const [timeRange, setTimeRange] = useState<TimeRangeId>('all')
  const [page, setPage] = useState(1)
  // The filter rail is always visible on large screens; on small screens
  // this toggle collapses it back into the toolbar button.
  const [filtersOpen, setFiltersOpen] = useState(false)
  const filters = useMemo<TraceFilters>(
    () => ({
      name: search || undefined,
      project: project || undefined,
      from:
        rangeFromMs(timeRange) !== undefined
          ? new Date(rangeFromMs(timeRange)!).toISOString()
          : undefined,
      page,
      limit: pageSize,
    }),
    [search, project, timeRange, page],
  )
  // The route loader ensures the initial (unfiltered) query; this read hits
  // the warm cache on first render and only fetches on filter changes.
  const tracesQuery = useQuery(tracesOptions(filters))

  const traces = tracesQuery.data?.data ?? []
  const total = tracesQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const averageLatency = traces.length
    ? traces.reduce((totalValue, trace) => totalValue + (trace.latency ?? trace.duration ?? 0), 0) /
      traces.length
    : 0
  const totalCost = traces.reduce((totalValue, trace) => totalValue + (trace.totalCost ?? 0), 0)
  const errors = traces.filter((trace) => statusTone(trace.status) === 'red').length
  const visibleTraces = status
    ? traces.filter((trace) => statusTone(trace.status) === statusTone(status))
    : traces
  const activeFilterCount =
    Number(Boolean(project)) + Number(Boolean(status)) + Number(timeRange !== 'all')
  const clearFilters = () => {
    setProject('')
    setStatus('')
    setTimeRange('all')
    setSearch('')
    setPage(1)
  }

  return (
    <div className='min-h-full text-content-primary'>
      <PageHeader
        eyebrow='Observe'
        title='Traces'
        count={total.toLocaleString()}
        description='Inspect every run received from your local AI applications.'
        actions={
          <>
            <Button
              variant='outline'
              size='sm'
              className='lg:hidden'
              onClick={() => setFiltersOpen((open) => !open)}
              aria-expanded={filtersOpen}
            >
              <SlidersHorizontal data-icon='inline-start' />
              Filters
              {activeFilterCount > 0 && (
                <span className='flex size-4 items-center justify-center rounded-full bg-interactive-primary text-[10px] text-white'>
                  {activeFilterCount}
                </span>
              )}
            </Button>
            <Button
              size='sm'
              className='bg-interactive-primary text-white hover:bg-interactive-primary-hover'
            >
              <Plus data-icon='inline-start' />
              New trace
            </Button>
          </>
        }
      />
      <div className='flex flex-col gap-5 px-4 py-5 sm:px-6'>
        <MetricStrip>
          <MetricCell
            icon={Activity}
            label='Total traces'
            value={tracesQuery.isLoading ? '—' : total.toLocaleString()}
            detail='Across this project'
          />
          <MetricCell
            icon={Zap}
            label='Error rate'
            value={traces.length ? `${Math.round((errors / traces.length) * 100)}%` : '—'}
            detail='Current page'
          />
          <MetricCell
            icon={TimerReset}
            label='Average latency'
            value={traces.length ? formatDuration(averageLatency) : '—'}
            detail='Current page'
          />
          <MetricCell
            icon={Coins}
            label='Total cost'
            value={traces.length ? formatCost(totalCost) : '—'}
            detail='Current page'
          />
        </MetricStrip>

        <div className='flex flex-col items-start gap-5 lg:flex-row'>
          <aside
            aria-label='Trace filters'
            className={cn('w-full shrink-0 lg:block lg:w-60', filtersOpen ? 'block' : 'hidden')}
          >
            <div className='rounded-[4px] border border-border-primary bg-surface-primary p-3 lg:sticky lg:top-4'>
              <div className='mb-3 flex items-center justify-between gap-2'>
                <h3 className='text-[11px] font-semibold text-content-primary'>Filters</h3>
                {activeFilterCount > 0 && (
                  <Button
                    variant='ghost'
                    size='xs'
                    onClick={clearFilters}
                    className='h-6 rounded-[3px] px-1.5 text-[11px] text-content-secondary hover:text-content-brand'
                  >
                    Clear all
                  </Button>
                )}
              </div>
              <div className='flex flex-col gap-3.5'>
                <Field className='gap-1.5'>
                  <FieldLabel
                    htmlFor='trace-status'
                    className='text-[11px] font-medium text-content-secondary'
                  >
                    Status
                  </FieldLabel>
                  <Select
                    value={status || 'all'}
                    onValueChange={(value) => {
                      setStatus(value === 'all' ? '' : ((value ?? '') as TraceStatus | ''))
                      setPage(1)
                    }}
                  >
                    <SelectTrigger
                      id='trace-status'
                      size='sm'
                      className='w-full rounded-[3px] border-border-secondary bg-background-primary text-xs'
                    >
                      <SelectValue placeholder='Any status' />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value='all'>Any status</SelectItem>
                        <SelectItem value='success'>Success</SelectItem>
                        <SelectItem value='error'>Error</SelectItem>
                        <SelectItem value='pending'>Pending</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field className='gap-1.5'>
                  <FieldLabel
                    htmlFor='trace-project'
                    className='text-[11px] font-medium text-content-secondary'
                  >
                    Project
                  </FieldLabel>
                  <Select
                    value={project || 'all'}
                    onValueChange={(value) => {
                      setProject(value === 'all' ? '' : (value ?? ''))
                      setPage(1)
                    }}
                  >
                    <SelectTrigger
                      id='trace-project'
                      size='sm'
                      className='w-full rounded-[3px] border-border-secondary bg-background-primary text-xs'
                    >
                      <SelectValue placeholder='All projects' />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value='all'>All projects</SelectItem>
                        <SelectItem value='default'>default</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field className='gap-1.5'>
                  <FieldLabel
                    htmlFor='trace-time-range'
                    className='text-[11px] font-medium text-content-secondary'
                  >
                    Time range
                  </FieldLabel>
                  <Select
                    value={timeRange}
                    onValueChange={(value) => {
                      setTimeRange((value ?? 'all') as TimeRangeId)
                      setPage(1)
                    }}
                  >
                    <SelectTrigger
                      id='trace-time-range'
                      size='sm'
                      className='w-full rounded-[3px] border-border-secondary bg-background-primary text-xs'
                    >
                      <SelectValue placeholder='All time' />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {TIME_RANGES.map((range) => (
                          <SelectItem key={range.id} value={range.id}>
                            {range.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
              </div>
            </div>
          </aside>

          {/* ── trace table ── */}
          <Panel
            title='Trace explorer'
            description='Filter, inspect, and open any recorded run.'
            className='w-full min-w-0 flex-1 self-stretch'
          >
            <CardContent className='flex flex-col gap-3 border-b border-border-primary bg-surface-primary px-4 py-3 sm:flex-row sm:items-center'>
              <div className='relative min-w-0 flex-1 sm:max-w-sm'>
                <Search
                  aria-hidden='true'
                  className='pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-content-tertiary'
                />
                <Input
                  aria-label='Search trace name or ID'
                  value={search}
                  onChange={(event) => {
                    setSearch(event.target.value)
                    setPage(1)
                  }}
                  placeholder='Search trace name or ID'
                  className='h-8 rounded-[3px] border-border-secondary bg-background-primary pl-8 text-xs shadow-none focus-visible:border-border-brand'
                />
              </div>
              <div className='flex items-center gap-2'>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  onClick={() => void tracesQuery.refetch()}
                  aria-label='Reload traces'
                  className='text-content-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
                >
                  <RefreshCw className={cn(tracesQuery.isFetching && 'animate-spin')} />
                </Button>
              </div>
            </CardContent>
            {tracesQuery.isLoading ? (
              <TableLoading />
            ) : tracesQuery.isError ? (
              <EmptyPanel
                icon={<AlertCircle className='size-4' />}
                title='Could not load traces'
                description={
                  tracesQuery.error instanceof Error
                    ? tracesQuery.error.message
                    : 'The local API is unavailable. Start Pocketfuse and try again.'
                }
                action={
                  <Button variant='outline' size='sm' onClick={() => void tracesQuery.refetch()}>
                    <RefreshCw data-icon='inline-start' />
                    Try again
                  </Button>
                }
              />
            ) : traces.length === 0 ? (
              <EmptyPanel
                icon={<Activity className='size-4' />}
                title='No traces yet'
                description='Start sending traces to the local Pocketfuse API and they will appear here.'
                action={
                  <Button variant='outline' size='sm'>
                    <Plus data-icon='inline-start' />
                    Create your first trace
                  </Button>
                }
              />
            ) : (
              <TraceTable traces={visibleTraces} />
            )}
            {!tracesQuery.isLoading && !tracesQuery.isError && traces.length > 0 && (
              <div className='flex items-center justify-between border-t border-border-primary px-4 py-2.5'>
                <span className='text-[11px] text-content-secondary'>
                  Showing{' '}
                  <span className='font-medium text-content-primary'>
                    {Math.min((page - 1) * pageSize + 1, total)}–{Math.min(page * pageSize, total)}
                  </span>{' '}
                  of <span className='font-medium text-content-primary'>{total}</span>
                </span>
                <div className='flex items-center gap-1'>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    disabled={page <= 1}
                    onClick={() => setPage((value) => Math.max(1, value - 1))}
                    aria-label='Previous page'
                    className='text-content-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
                  >
                    <ChevronLeft />
                  </Button>
                  <span className='min-w-14 text-center font-mono text-[10px] text-content-secondary'>
                    {page} / {totalPages}
                  </span>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    disabled={page >= totalPages}
                    onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
                    aria-label='Next page'
                    className='text-content-secondary hover:bg-interactive-secondary-hover hover:text-content-brand'
                  >
                    <ChevronRight />
                  </Button>
                </div>
              </div>
            )}
          </Panel>
        </div>
      </div>
    </div>
  )
}
