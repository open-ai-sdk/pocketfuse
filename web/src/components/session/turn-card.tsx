import { Link } from '@tanstack/react-router'
import { ArrowUpRight, CornerDownRight, TriangleAlert, Unlink, Wrench } from 'lucide-react'
import { useState } from 'react'
import type { Preview, StoryActivity, ToolCall, Turn, TurnStep } from '../../lib/api'
import { barLayout, displayName } from '../../lib/trace-tree'
import { cn, formatClock, formatDuration, formatUsage } from '../../lib/utils'
import { panelSurface, StatusChip } from '../../pages/primitives'
import { Badge } from '../ui'
import { kindOf, toneBarClass, toneChipClass } from '../trace/kinds'

// One turn of a conversation read top to bottom: what started it, each step in
// place with its timing, then what came back. Clicking anything opens its detail
// beside the conversation, so reading never leaves the page.

const PROMPT_LABEL: Record<string, string> = {
  turn: 'User',
  wake_turn: 'Woken by',
  job: 'Job brief',
  run: 'Input',
}

function MessageBlock({
  label,
  preview,
  tone,
  onOpen,
}: {
  label: string
  preview: Preview
  tone: 'prompt' | 'reply'
  onOpen?: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  const long = preview.text.length > 420
  const clipped = preview.text.length < preview.totalChars
  return (
    <div
      className={cn(
        'rounded-[3px] border px-2.5 py-1.5',
        tone === 'reply'
          ? 'border-border-secondary bg-surface-accent'
          : 'border-border-primary bg-surface-tertiary',
      )}
    >
      <span className='font-mono text-[9.5px] font-medium uppercase tracking-[0.08em] text-content-tertiary'>
        {label}
      </span>
      <p className='mt-0.5 whitespace-pre-wrap break-words text-[12px] leading-5 text-content-primary'>
        {long && !expanded ? `${preview.text.slice(0, 420)}…` : preview.text}
      </p>
      {long && !expanded && (
        <button
          type='button'
          onClick={() => setExpanded(true)}
          className='mt-0.5 text-[10.5px] font-medium text-content-brand hover:underline'
        >
          Show more
        </button>
      )}
      {expanded && clipped && onOpen && (
        <button
          type='button'
          onClick={onOpen}
          className='mt-0.5 text-[10.5px] font-medium text-content-brand hover:underline'
        >
          Open the span for all {preview.totalChars.toLocaleString()} characters
        </button>
      )}
    </div>
  )
}

function ToolCallLine({ call }: { call: ToolCall }) {
  return (
    <span className='flex min-w-0 items-baseline gap-1.5 text-[11px] leading-4'>
      {call.error ? (
        <TriangleAlert className='size-3 shrink-0 translate-y-0.5 text-content-error' />
      ) : (
        <Wrench className='size-3 shrink-0 translate-y-0.5 text-content-warning' />
      )}
      <span className='shrink-0 font-mono font-medium text-content-primary'>{call.name}</span>
      {call.durationMs !== undefined && (
        <span className='shrink-0 font-mono text-[10px] text-content-tertiary'>
          {formatDuration(call.durationMs)}
        </span>
      )}
      <span
        className={cn(
          'min-w-0 truncate',
          call.error ? 'text-content-error' : 'text-content-tertiary',
        )}
        title={call.error || call.result}
      >
        {call.error || call.result || 'no result recorded'}
      </span>
    </span>
  )
}

function StepRow({
  step,
  turn,
  selected,
  onSelect,
}: {
  step: TurnStep
  turn: Turn
  selected: boolean
  onSelect: () => void
}) {
  const kind = kindOf(step)
  const Icon = kind.icon
  const failed = step.level?.toUpperCase() === 'ERROR' || step.level?.toUpperCase() === 'FATAL'
  const bar = barLayout(step.offsetMs, step.durationMs, turn.durationMs)
  return (
    <li>
      <button
        type='button'
        aria-pressed={selected}
        onClick={onSelect}
        className={cn(
          'flex w-full flex-col gap-0.5 rounded-[2px] px-1.5 py-1 text-left',
          selected ? 'bg-interactive-secondary-hover' : 'hover:bg-surface-tertiary',
        )}
      >
        <span className='flex min-w-0 items-center gap-1.5'>
          <span className='w-11 shrink-0 text-right font-mono text-[9.5px] tabular-nums text-content-tertiary'>
            +{formatDuration(step.offsetMs)}
          </span>
          <span
            title={kind.label}
            className={cn(
              'flex size-4 shrink-0 items-center justify-center rounded-[2px] border',
              toneChipClass[kind.tone],
            )}
          >
            <Icon className='size-2.5' />
          </span>
          <span
            title={step.name}
            className={cn(
              'min-w-0 truncate text-[11.5px] font-medium',
              failed ? 'text-content-error' : 'text-content-primary',
              step.side && 'text-content-secondary',
            )}
          >
            {displayName({ id: step.observationId, label: step.label, name: step.name })}
          </span>
          {step.side && (
            <span className='shrink-0 text-[9.5px] text-content-tertiary'>side call</span>
          )}
          <span className='ml-auto flex shrink-0 items-center gap-2 text-[10px] text-content-tertiary'>
            {formatUsage(step.usage) && <span>{formatUsage(step.usage)}</span>}
            <span className='w-12 text-right font-mono tabular-nums text-content-secondary'>
              {step.pending ? 'running' : formatDuration(step.durationMs)}
            </span>
          </span>
        </span>
        <span className='relative ml-[50px] block h-1 rounded-full bg-surface-tertiary'>
          <span
            className={cn(
              'absolute inset-y-0 rounded-full',
              failed ? 'bg-action-danger' : toneBarClass[kind.tone],
            )}
            style={bar}
          />
        </span>
        {step.statusMessage && (
          <span className='ml-[50px] truncate text-[11px] text-content-error'>
            {step.statusMessage}
          </span>
        )}
        {step.text && !step.side && (
          <span className='ml-[50px] line-clamp-2 text-[11px] leading-4 text-content-secondary'>
            {step.text.text}
          </span>
        )}
        {step.toolCalls?.map((call, index) => (
          <span key={call.id ?? index} className='ml-[50px] block min-w-0'>
            <ToolCallLine call={call} />
          </span>
        ))}
      </button>
    </li>
  )
}

export function TurnCard({
  turn,
  selectedTrace,
  selectedSpan,
  onSelect,
}: {
  turn: Turn
  selectedTrace?: string
  selectedSpan?: string
  onSelect: (traceId: string, observationId?: string) => void
}) {
  const kind = kindOf({ role: turn.kind })
  const Icon = kind.icon
  const open = selectedTrace === turn.traceId
  const usage = formatUsage(turn.usage)
  const lastCall = [...turn.steps]
    .reverse()
    .find((step) => step.role === 'model_call' && !step.side)
  return (
    <article
      id={`turn-${turn.traceId}`}
      className={cn(panelSurface, 'scroll-mt-3 overflow-hidden', open && 'border-border-brand')}
    >
      <header className='flex flex-wrap items-center gap-x-2.5 gap-y-1 border-b border-border-primary px-3 py-2'>
        <button
          type='button'
          onClick={() => onSelect(turn.traceId)}
          className='flex min-w-0 items-center gap-1.5 hover:text-content-brand'
        >
          <span
            className={cn(
              'flex size-5 shrink-0 items-center justify-center rounded-[3px] border',
              toneChipClass[kind.tone],
            )}
          >
            <Icon className='size-3' />
          </span>
          <span className='text-xs font-semibold'>{kind.label}</span>
          {turn.agentId && (
            <span className='truncate text-[11px] text-content-secondary'>{turn.agentId}</span>
          )}
        </button>
        <StatusChip status={turn.status} />
        <span className='font-mono text-[10.5px] tabular-nums text-content-tertiary'>
          {formatClock(turn.startTime)}
        </span>
        <span className='font-mono text-[11px] tabular-nums text-content-primary'>
          {formatDuration(turn.durationMs)}
        </span>
        <span className='text-[10.5px] text-content-secondary'>
          {turn.modelCalls} model call{turn.modelCalls === 1 ? '' : 's'} · {turn.toolCalls} tool
          {turn.toolCalls === 1 ? '' : 's'}
        </span>
        {usage && <span className='text-[10.5px] text-content-secondary'>{usage}</span>}
        {turn.failedTools ? (
          <span className='inline-flex items-center gap-1 text-[10.5px] font-medium text-content-error'>
            <TriangleAlert className='size-3' />
            {turn.failedTools} tool error{turn.failedTools === 1 ? '' : 's'}
          </span>
        ) : null}
        <Link
          to='/traces/$traceId'
          params={{ traceId: turn.traceId }}
          className='ml-auto inline-flex items-center gap-0.5 text-[10.5px] font-medium text-content-secondary hover:text-content-brand hover:underline'
        >
          Full trace
          <ArrowUpRight className='size-3' />
        </Link>
      </header>
      <div className='flex flex-col gap-2 p-2.5'>
        {turn.spawnedBy && (
          <a
            href={`#turn-${turn.spawnedBy.traceId}`}
            onClick={(event) => {
              event.preventDefault()
              const link = turn.spawnedBy
              if (link) onSelect(link.traceId, link.observationId)
            }}
            className='inline-flex items-center gap-1 text-[10.5px] text-content-secondary hover:text-content-brand'
          >
            <CornerDownRight className='size-3' />
            Started by the {turn.spawnedBy.tool} call above
          </a>
        )}
        {turn.overlaps && !turn.spawnedBy && (
          <span className='text-[10.5px] text-content-tertiary'>
            Started while the previous turn was still running.
          </span>
        )}
        {turn.errorCount ? (
          <p className='rounded-[3px] border border-border-danger bg-surface-error px-2.5 py-1.5 text-[11px] text-content-error'>
            {turn.statusMessage || 'The sender marked a span failed without saying why.'}
          </p>
        ) : null}
        {turn.userMessage && (
          <MessageBlock
            label={PROMPT_LABEL[turn.kind] ?? 'Input'}
            preview={turn.userMessage}
            tone='prompt'
            onOpen={() => {
              const first = turn.steps.find((step) => step.role === 'model_call' && !step.side)
              onSelect(turn.traceId, first?.observationId)
            }}
          />
        )}
        {turn.steps.length > 0 && (
          <ol className='flex flex-col'>
            {turn.steps.map((step) => (
              <StepRow
                key={step.observationId}
                step={step}
                turn={turn}
                selected={open && selectedSpan === step.observationId}
                onSelect={() => onSelect(turn.traceId, step.observationId)}
              />
            ))}
          </ol>
        )}
        {turn.reply && (
          <MessageBlock
            label='Assistant'
            preview={turn.reply}
            tone='reply'
            onOpen={lastCall ? () => onSelect(turn.traceId, lastCall.observationId) : undefined}
          />
        )}
      </div>
    </article>
  )
}

export function ActivityRow({
  activity,
  selectedTrace,
  onSelect,
}: {
  activity: StoryActivity
  selectedTrace?: string
  onSelect: (traceId: string) => void
}) {
  const [open, setOpen] = useState(false)
  const count = activity.requests.length
  const unlinked = activity.requests.filter((request) => request.unlinked).length
  return (
    <div className='px-1 text-[10.5px] text-content-tertiary'>
      <button
        type='button'
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className='flex items-center gap-1.5 hover:text-content-secondary'
      >
        <span className='font-mono tabular-nums'>{formatClock(activity.startTime)}</span>
        <span>
          {open ? 'Hide' : 'Show'} {count} request{count === 1 ? '' : 's'}
        </span>
        {activity.errorCount ? (
          <span className='font-medium text-content-error'>{activity.errorCount} failed</span>
        ) : null}
        {unlinked > 0 && <span>{unlinked} by time only</span>}
      </button>
      {open && (
        <ul className='mt-1 flex flex-col border-l border-border-primary pl-2'>
          {activity.requests.map((request) => (
            <li key={request.traceId}>
              <button
                type='button'
                onClick={() => onSelect(request.traceId)}
                className={cn(
                  'flex w-full items-center gap-2 rounded-[2px] px-1.5 py-0.5 text-left hover:bg-surface-tertiary',
                  selectedTrace === request.traceId && 'bg-interactive-secondary-hover',
                )}
              >
                <span className='font-mono tabular-nums'>{formatClock(request.startTime)}</span>
                <span
                  className={cn(
                    'min-w-0 truncate',
                    request.status === 'error' ? 'text-content-error' : 'text-content-secondary',
                  )}
                >
                  {request.name || request.traceId}
                </span>
                {request.unlinked && (
                  <span title='Ran at the same time, but no identifier connects it. Another thread may own it.'>
                    <Badge tone='amber' className='gap-1 rounded-[3px] px-1 text-[9.5px]'>
                      <Unlink className='size-2.5' />
                      Time match
                    </Badge>
                  </span>
                )}
                <span className='ml-auto font-mono tabular-nums'>
                  {formatDuration(request.durationMs)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
