import { useQuery } from '@tanstack/react-query'
import { Bot, ChevronRight, TriangleAlert, Wrench } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import type { Observation, Score, ToolCall, Trace, TurnStep } from '../../lib/api'
import { relatedTracesOptions } from '../../lib/queries'
import {
  displayName,
  observationDurationMs,
  observationHasError,
  observationIsPending,
  observationTokens,
} from '../../lib/trace-tree'
import {
  cn,
  formatCost,
  formatDate,
  formatDuration,
  formatTokens,
  formatUsage,
  prettyJson,
} from '../../lib/utils'
import { panelSurface, StatusChip } from '../../pages/primitives'
import { ExpandableText, IOPreview } from '../io-preview'
import { Badge } from '../ui'
import { kindOf, toneChipClass } from './kinds'
import { LinkedTraceList, correlationEntries, sessionLabel } from './linked-trace-list'
import { ScoreRows } from './score-rows'

// The detail of one span or one trace, shared by the trace page and the session
// side panel: what it is, how long it took and what it cost on top, then input
// and output, then everything else folded away.

export const sectionLabel =
  'font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'

function Fold({
  title,
  count,
  children,
  initiallyOpen = false,
}: {
  title: string
  count?: number
  children: ReactNode
  initiallyOpen?: boolean
}) {
  const [open, setOpen] = useState(initiallyOpen)
  return (
    <section className={cn(panelSurface, 'overflow-hidden')}>
      <button
        type='button'
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className='flex w-full items-center gap-1.5 px-3 py-2 text-left hover:bg-surface-tertiary'
      >
        <ChevronRight
          className={cn('size-3 text-content-tertiary transition-transform', open && 'rotate-90')}
        />
        <span className={sectionLabel}>{title}</span>
        {count !== undefined && (
          <span className='font-mono text-[10px] text-content-tertiary'>{count}</span>
        )}
      </button>
      {open && <div className='border-t border-border-primary p-3'>{children}</div>}
    </section>
  )
}

function Facts({ rows }: { rows: { label: string; value?: string; mono?: boolean }[] }) {
  return (
    <dl className='grid gap-x-6 gap-y-2 sm:grid-cols-2'>
      {rows
        .filter((row) => row.value)
        .map(({ label, value, mono }) => (
          <div key={label} className='min-w-0'>
            <dt className='font-mono text-[9.5px] uppercase tracking-[0.08em] text-content-tertiary'>
              {label}
            </dt>
            <dd
              title={value}
              className={cn(
                'truncate text-xs text-content-primary',
                mono && 'font-mono text-[11px]',
              )}
            >
              {value}
            </dd>
          </div>
        ))}
    </dl>
  )
}

function Json({ value }: { value: unknown }) {
  return (
    <pre className='max-h-72 overflow-auto rounded-[3px] border border-border-primary bg-surface-tertiary p-2.5 font-mono text-[10.5px] leading-4 text-content-secondary'>
      {prettyJson(value)}
    </pre>
  )
}

// Indents text holding JSON; a clipped or plain text stays as it is.
function readableJson(text: string) {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

function ToolCallCard({ call }: { call: ToolCall }) {
  return (
    <div
      className={cn(
        'rounded-[3px] border p-2.5',
        call.error ? 'border-border-danger bg-surface-error' : 'border-border-primary',
      )}
    >
      <div className='flex items-center gap-1.5 font-mono text-[11px] font-medium text-content-primary'>
        {call.error ? (
          <TriangleAlert className='size-3 text-content-error' />
        ) : (
          <Wrench className='size-3 text-content-warning' />
        )}
        {call.name}
        {call.durationMs !== undefined && (
          <span className='font-normal text-content-tertiary'>
            {formatDuration(call.durationMs)}
          </span>
        )}
      </div>
      {call.args && (
        <pre className='mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words font-mono text-[10.5px] leading-4 text-content-secondary'>
          {readableJson(call.args)}
        </pre>
      )}
      {(call.error || call.result) && (
        <pre
          className={cn(
            'mt-1.5 max-h-56 overflow-auto whitespace-pre-wrap break-words border-t border-border-primary pt-1.5 font-mono text-[10.5px] leading-4',
            call.error ? 'text-content-error' : 'text-content-secondary',
          )}
        >
          {readableJson(call.error || call.result || '')}
        </pre>
      )}
    </div>
  )
}

// Token breakdowns a provider reports beyond input and output.
function usageDetails(observation: Observation) {
  const usage = observation.usage ?? {}
  return [
    { key: 'cached_input', label: 'Cached input' },
    { key: 'cache_creation_input', label: 'Cache creation' },
    { key: 'reasoning', label: 'Reasoning' },
  ].flatMap(({ key, label }) => {
    const value = usage[key]
    return typeof value === 'number' && value > 0 ? [{ label, value: formatTokens(value) }] : []
  })
}

function ScoreSection({ scores }: { scores: Score[] }) {
  if (scores.length === 0) return null
  return (
    <section className={cn(panelSurface, 'p-3')}>
      <h4 className={cn(sectionLabel, 'mb-1')}>Scores</h4>
      <ScoreRows scores={scores} emptyHint='' />
    </section>
  )
}

export function ObservationDetail({
  observation,
  step,
  scores,
  onSelect,
}: {
  observation: Observation
  step?: TurnStep
  scores: Score[]
  onSelect?: (id: string) => void
}) {
  const kind = kindOf(observation)
  const Icon = kind.icon
  const tokens = observationTokens(observation)
  const pending = observationIsPending(observation)
  const duration = observation.duration ?? observationDurationMs(observation)
  const hasParameters = Object.keys(observation.modelParameters ?? {}).length > 0
  const hasMetadata = Object.keys(observation.metadata ?? {}).length > 0
  const failed = observationHasError(observation)
  return (
    <div className='flex flex-col gap-3'>
      <div className={cn(panelSurface, 'px-3.5 py-3')}>
        <div className='flex flex-wrap items-center gap-2'>
          <span
            className={cn(
              'flex size-6 items-center justify-center rounded-[3px] border',
              toneChipClass[kind.tone],
            )}
          >
            <Icon className='size-3.5' />
          </span>
          <h2 className='min-w-0 truncate text-sm font-semibold text-content-primary'>
            {displayName(observation)}
          </h2>
          <Badge tone={kind.tone} className='rounded-[3px] px-1.5 text-[10px]'>
            {kind.label}
          </Badge>
          {step?.side && (
            <span title='Outside the conversation: its system prompt differs from the rest of this turn.'>
              <Badge tone='neutral' className='rounded-[3px] px-1.5 text-[10px]'>
                Side call
              </Badge>
            </span>
          )}
          {failed && (
            <Badge tone='red' className='rounded-[3px] px-1.5 font-mono text-[10px] uppercase'>
              {observation.level}
            </Badge>
          )}
        </div>
        <div className='mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[10.5px] text-content-secondary'>
          <span className='font-mono'>{pending ? 'running' : formatDuration(duration)}</span>
          {tokens.total !== undefined && (
            <span>
              {formatUsage({
                input: tokens.input,
                output: tokens.output,
                cachedInput: step?.usage.cachedInput,
              }) ?? `${formatTokens(tokens.total)} tokens`}
            </span>
          )}
          {observation.cost !== undefined && <span>{formatCost(observation.cost)}</span>}
          {step?.finishReason && <span>finish: {step.finishReason}</span>}
          <span>{formatDate(observation.startTime ?? observation.timestamp)}</span>
          {observation.label !== observation.name && observation.name && (
            <span className='font-mono text-content-tertiary'>{observation.name}</span>
          )}
          {observation.mergedInto && onSelect && (
            <button
              type='button'
              onClick={() => onSelect(observation.mergedInto ?? '')}
              className='font-medium text-content-brand hover:underline'
            >
              Part of one model call
            </button>
          )}
        </div>
        {observation.mergedSpans && observation.mergedSpans.length > 0 && (
          <p
            className='mt-1.5 text-[10.5px] text-content-tertiary'
            title='Nested spans with the same model and token counts, and no payload of their own, are one provider call.'
          >
            One provider call: also recorded as a nested span, shown together here.
          </p>
        )}
        {observation.statusMessage ? (
          <p className='mt-2 whitespace-pre-wrap break-words rounded-[3px] border border-border-danger bg-surface-error px-2.5 py-1.5 text-[11px] text-content-error'>
            {observation.statusMessage}
          </p>
        ) : (
          failed && (
            <p className='mt-2 rounded-[3px] border border-border-danger bg-surface-error px-2.5 py-1.5 text-[11px] text-content-error'>
              The sender marked this span failed without saying why.
            </p>
          )
        )}
      </div>
      {step?.toolCalls && step.toolCalls.length > 0 && (
        <section className={cn(panelSurface, 'p-3')}>
          <h4 className={cn(sectionLabel, 'mb-2')}>Tool calls</h4>
          <div className='flex flex-col gap-2'>
            {step.toolCalls.map((call, index) => (
              <ToolCallCard key={call.id ?? index} call={call} />
            ))}
          </div>
        </section>
      )}
      {step?.toolCalls?.length && !step.text ? (
        <Fold title='Output as recorded'>
          <div className='-m-3'>
            <IOPreview
              label='Output'
              value={observation.output}
              className='rounded-none border-0'
            />
          </div>
        </Fold>
      ) : (
        <IOPreview label='Output' value={observation.output} />
      )}
      <IOPreview label='Input' value={observation.input} />
      <ScoreSection scores={scores} />
      <Fold title='Details'>
        <Facts
          rows={[
            { label: 'Span id', value: observation.id, mono: true },
            { label: 'Parent', value: observation.parentObservationId, mono: true },
            { label: 'Type', value: observation.type },
            { label: 'Model', value: observation.model },
            { label: 'Started', value: formatDate(observation.startTime) },
            { label: 'Ended', value: observation.endTime ? formatDate(observation.endTime) : '' },
            {
              label: 'Span closed at (sender)',
              value: observation.reportedEndTime ? formatDate(observation.reportedEndTime) : '',
            },
            ...usageDetails(observation),
          ]}
        />
      </Fold>
      {hasParameters && (
        <Fold title='Model parameters'>
          <Json value={observation.modelParameters} />
        </Fold>
      )}
      {hasMetadata && (
        <Fold title='Metadata'>
          <Json value={observation.metadata} />
        </Fold>
      )}
    </div>
  )
}

function AroundThisTrace({ trace }: { trace: Trace }) {
  const related = useQuery(relatedTracesOptions(trace.id))
  if (related.isLoading) {
    return <p className='text-[11px] text-content-tertiary'>Loading…</p>
  }
  if (related.isError) {
    return <p className='text-[11px] text-content-error'>Could not load related traces.</p>
  }
  const overlapping = (related.data?.overlapping ?? []).map((item) => ({
    trace: item,
    unlinked: true,
  }))
  return (
    <div className='-m-3'>
      <p className='border-b border-border-primary px-3 py-2 text-[10.5px] text-content-tertiary'>
        Overlapping in time only. Nothing recorded a link, so another thread may own these.
      </p>
      <LinkedTraceList items={overlapping} emptyText='Nothing else ran during this trace.' />
    </div>
  )
}

export function TraceDetail({
  trace,
  scores,
  totalTokens,
}: {
  trace: Trace
  scores: Score[]
  totalTokens?: number
}) {
  const turn = trace.turn
  const hasMetadata = Object.keys(trace.metadata ?? {}).length > 0
  const usage = formatUsage(turn?.usage)
  return (
    <div className='flex flex-col gap-3'>
      <div className={cn(panelSurface, 'px-3.5 py-3')}>
        <div className='flex flex-wrap items-center gap-2'>
          <h2 className='min-w-0 truncate text-sm font-semibold text-content-primary'>
            {trace.name || 'Untitled trace'}
          </h2>
          <StatusChip status={trace.status} />
          {turn?.agentId && (
            <span className='inline-flex items-center gap-1 text-[11px] text-content-secondary'>
              <Bot className='size-3' />
              {turn.agentId}
            </span>
          )}
        </div>
        <div className='mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[10.5px] text-content-secondary'>
          <span className='font-mono'>{formatDuration(trace.duration)}</span>
          {usage ? (
            <span>{usage}</span>
          ) : (
            totalTokens !== undefined && <span>{formatTokens(totalTokens)} tokens</span>
          )}
          {trace.totalCost !== undefined && <span>{formatCost(trace.totalCost)}</span>}
          <span>{formatDate(trace.startTime ?? trace.timestamp)}</span>
        </div>
        {turn && turn.errorCount && !turn.statusMessage ? (
          <p className='mt-2 rounded-[3px] border border-border-danger bg-surface-error px-2.5 py-1.5 text-[11px] text-content-error'>
            The sender marked a span failed without saying why.
          </p>
        ) : null}
        {turn?.statusMessage && (
          <p className='mt-2 whitespace-pre-wrap break-words rounded-[3px] border border-border-danger bg-surface-error px-2.5 py-1.5 text-[11px] text-content-error'>
            {turn.statusMessage}
          </p>
        )}
      </div>
      {trace.input !== undefined && trace.input !== null && (
        <IOPreview label='Input' value={trace.input} />
      )}
      {trace.output !== undefined && trace.output !== null && (
        <IOPreview label='Output' value={trace.output} />
      )}
      {turn?.userMessage && (
        <section className={cn(panelSurface, 'p-3')}>
          <h4 className={cn(sectionLabel, 'mb-1.5')}>Started by</h4>
          <ExpandableText value={turn.userMessage.text} />
        </section>
      )}
      {turn?.reply && (
        <section className={cn(panelSurface, 'p-3')}>
          <h4 className={cn(sectionLabel, 'mb-1.5')}>Replied</h4>
          <ExpandableText value={turn.reply.text} />
        </section>
      )}
      <ScoreSection scores={scores} />
      <Fold title='Details'>
        <Facts
          rows={[
            { label: 'Trace id', value: trace.id, mono: true },
            { label: 'Project', value: trace.project },
            { label: 'Session', value: trace.sessionId ? sessionLabel(trace.sessionId) : '' },
            { label: 'User', value: trace.userId },
            { label: 'Release', value: trace.release ?? trace.version },
            { label: 'Environment', value: trace.environment },
            ...correlationEntries(trace).map(([label, value]) => ({ label, value, mono: true })),
          ]}
        />
        {trace.tags && trace.tags.length > 0 && (
          <div className='mt-2 flex flex-wrap gap-1'>
            {trace.tags.map((tag) => (
              <Badge key={tag} tone='neutral' className='rounded-[3px] font-mono text-[9.5px]'>
                {tag}
              </Badge>
            ))}
          </div>
        )}
      </Fold>
      {hasMetadata && (
        <Fold title='Metadata'>
          <Json value={trace.metadata} />
        </Fold>
      )}
      <Fold title='Ran at the same time'>
        <AroundThisTrace trace={trace} />
      </Fold>
    </div>
  )
}
