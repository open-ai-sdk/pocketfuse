import { Check, ChevronRight, Copy, TriangleAlert, Wrench } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  parseConversation,
  parseModelOutput,
  type DisplayMessage,
  type MessagePart,
} from '../lib/llm-messages'
import { cn, prettyJson } from '../lib/utils'
import { Badge, Button } from './ui'

// IO preview: a prompt reads as a conversation, latest messages first in view,
// with system prompts and earlier history one click away. A model's answer reads
// as text and tool calls. Anything else is a key-value table, and Raw is always
// the JSON as stored.

const MAX_RENDER_CHARS = 600_000
// How much of a long prompt shows before asking: the end is what changed.
const RECENT_MESSAGES = 6
const LONG_TEXT_CHARS = 1200

export function ExpandableText({
  value,
  collapsed = false,
}: {
  value: string
  collapsed?: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const long = value.length > LONG_TEXT_CHARS
  if (collapsed && !expanded) {
    return (
      <button
        type='button'
        onClick={() => setExpanded(true)}
        className='inline-flex min-h-11 items-center gap-1 text-xs text-content-tertiary hover:text-content-brand'
      >
        <ChevronRight className='size-3' />
        Show {value.length.toLocaleString()} characters
      </button>
    )
  }
  return (
    <div>
      <p className='whitespace-pre-wrap break-words text-sm leading-6 text-content-primary'>
        {long && !expanded ? `${value.slice(0, LONG_TEXT_CHARS)}…` : value}
      </p>
      {long && !expanded && (
        <button
          type='button'
          onClick={() => setExpanded(true)}
          className='mt-1 inline-flex min-h-11 items-center text-xs font-medium text-content-brand hover:underline'
        >
          Show all {value.length.toLocaleString()} characters
        </button>
      )}
    </div>
  )
}

function JsonBlock({ value }: { value: unknown }) {
  return (
    <pre className='overflow-auto whitespace-pre-wrap break-words rounded-[3px] border border-border-primary bg-surface-tertiary p-2 font-mono text-xs leading-5 text-content-secondary'>
      {prettyJson(value) || '—'}
    </pre>
  )
}

function Part({ part, collapsed }: { part: MessagePart; collapsed: boolean }) {
  if (part.kind === 'text') return <ExpandableText value={part.text} collapsed={collapsed} />
  if (part.kind === 'tool_call') {
    return (
      <div className='flex flex-col gap-1'>
        <span className='inline-flex items-center gap-1 font-mono text-[10.5px] font-medium text-content-warning'>
          <Wrench className='size-3' />
          {part.name}
        </span>
        <JsonBlock value={part.args} />
      </div>
    )
  }
  return (
    <div className='flex flex-col gap-1'>
      <span
        className={cn(
          'inline-flex items-center gap-1 font-mono text-[10.5px] font-medium',
          part.error ? 'text-content-error' : 'text-content-secondary',
        )}
      >
        {part.error ? <TriangleAlert className='size-3' /> : <Wrench className='size-3' />}
        {part.name} {part.error ? 'failed' : 'returned'}
      </span>
      <JsonBlock value={part.value} />
    </div>
  )
}

function roleTone(role: string) {
  if (role === 'assistant') return 'violet'
  if (role === 'system' || role === 'developer') return 'amber'
  if (role === 'tool') return 'blue'
  return 'neutral'
}

function Bubble({ message }: { message: DisplayMessage }) {
  const role = message.role.toLowerCase()
  const collapsed = role === 'system' || role === 'developer'
  const onlyResults =
    message.parts.length > 0 && message.parts.every((p) => p.kind === 'tool_result')
  return (
    <div className='rounded-[3px] border border-border-primary bg-surface-primary px-3 py-2'>
      <Badge
        tone={onlyResults ? 'blue' : roleTone(role)}
        className='mb-1 rounded-[3px] px-1 font-mono text-xs uppercase'
      >
        {onlyResults ? 'tool result' : (message.name ?? message.role)}
      </Badge>
      {message.parts.length === 0 ? (
        <p className='text-[11px] text-content-tertiary'>Empty content</p>
      ) : (
        <div className='flex flex-col gap-2'>
          {message.parts.map((part, index) => (
            <Part key={index} part={part} collapsed={collapsed} />
          ))}
        </div>
      )}
    </div>
  )
}

function ConversationView({ messages, tools }: { messages: DisplayMessage[]; tools: string[] }) {
  const [showAll, setShowAll] = useState(false)
  const [showTools, setShowTools] = useState(false)
  const hidden = showAll ? 0 : Math.max(0, messages.length - RECENT_MESSAGES)
  return (
    <div className='flex flex-col gap-2'>
      {tools.length > 0 && (
        <button
          type='button'
          onClick={() => setShowTools((value) => !value)}
          className='min-h-11 text-left text-xs text-content-tertiary hover:text-content-secondary'
        >
          {showTools
            ? tools.join(', ')
            : `${tools.length} tool${tools.length === 1 ? '' : 's'} offered to the model`}
        </button>
      )}
      {hidden > 0 && (
        <button
          type='button'
          onClick={() => setShowAll(true)}
          className='min-h-11 rounded-[3px] border border-dashed border-border-primary py-1.5 text-xs font-medium text-content-secondary hover:text-content-brand'
        >
          Show {hidden} earlier message{hidden === 1 ? '' : 's'}
        </button>
      )}
      {messages.slice(hidden).map((message, index) => (
        <Bubble key={hidden + index} message={message} />
      ))}
    </div>
  )
}

// Non-chat fallback: top-level object as a key-value table; the rest as JSON.
function JsonTable({ value }: { value: unknown }) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return (
      <pre className='overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-5 text-content-secondary'>
        {prettyJson(value)}
      </pre>
    )
  }
  const entries = Object.entries(value)
  if (entries.length === 0) return <p className='text-[11px] text-content-tertiary'>Empty object</p>
  return (
    <dl className='flex flex-col divide-y divide-border-primary'>
      {entries.map(([key, entry]) => (
        <div key={key} className='grid gap-1 py-2 first:pt-0 last:pb-0 sm:grid-cols-[140px_1fr]'>
          <dt className='truncate font-mono text-[10px] uppercase tracking-[0.06em] text-content-tertiary'>
            {key}
          </dt>
          <dd className='min-w-0'>
            {typeof entry === 'string' ||
            typeof entry === 'number' ||
            typeof entry === 'boolean' ? (
              <span className='whitespace-pre-wrap break-words text-sm text-content-primary'>
                {String(entry)}
              </span>
            ) : (
              <pre className='max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-[3px] bg-surface-tertiary p-2 font-mono text-[10.5px] text-content-secondary'>
                {prettyJson(entry)}
              </pre>
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}

export function IOPreview({
  label,
  value,
  className,
}: {
  label: string
  value: unknown
  className?: string
}) {
  const [mode, setMode] = useState<'formatted' | 'raw'>('formatted')
  const [copyStatus, setCopyStatus] = useState<'idle' | 'pending' | 'success' | 'error'>('idle')
  const copyAttempt = useRef(0)
  const copying = useRef(false)
  const copyTimer = useRef<number | undefined>(undefined)
  const text = useMemo(() => prettyJson(value), [value])
  const copied = copyStatus === 'success'

  useEffect(() => {
    setCopyStatus('idle')
    return () => {
      copyAttempt.current += 1
      copying.current = false
      window.clearTimeout(copyTimer.current)
      copyTimer.current = undefined
    }
  }, [text])
  const oversized = text.length > MAX_RENDER_CHARS
  const conversation = useMemo(
    () => (oversized ? undefined : parseConversation(value)),
    [oversized, value],
  )
  const answer = useMemo(
    () => (oversized ? undefined : parseModelOutput(value)),
    [oversized, value],
  )
  const copy = async () => {
    if (copying.current) return
    copying.current = true
    const attempt = ++copyAttempt.current
    window.clearTimeout(copyTimer.current)
    copyTimer.current = undefined
    setCopyStatus('pending')
    try {
      await navigator.clipboard.writeText(text)
      if (attempt !== copyAttempt.current) return
      setCopyStatus('success')
      copyTimer.current = window.setTimeout(() => {
        if (attempt !== copyAttempt.current) return
        copyTimer.current = undefined
        setCopyStatus('idle')
      }, 1300)
    } catch {
      if (attempt !== copyAttempt.current) return
      setCopyStatus('error')
    } finally {
      if (attempt === copyAttempt.current) copying.current = false
    }
  }
  const empty = value === undefined || value === null
  const raw = (
    <pre className='overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-5 text-content-secondary'>
      {text}
    </pre>
  )

  return (
    <section
      className={cn(
        'overflow-hidden rounded-[4px] border border-border-primary bg-surface-primary',
        className,
      )}
    >
      <header className='flex items-center justify-between border-b border-border-primary px-3 py-2'>
        <div className='flex items-center gap-2'>
          <h4 className='font-mono text-[10px] font-medium uppercase tracking-[0.08em] text-content-secondary'>
            {label}
          </h4>
          <div className='flex rounded-[3px] border border-border-primary p-0.5'>
            {(['formatted', 'raw'] as const).map((item) => (
              <button
                key={item}
                type='button'
                onClick={() => setMode(item)}
                aria-pressed={mode === item}
                className={cn(
                  'inline-flex min-h-11 items-center rounded-[2px] px-2.5 text-xs capitalize',
                  mode === item
                    ? 'bg-interactive-secondary-hover text-content-primary'
                    : 'text-content-tertiary hover:text-content-secondary',
                )}
              >
                {item}
              </button>
            ))}
          </div>
        </div>
        {!empty && (
          <Button
            variant='ghost'
            size='icon-xs'
            onClick={() => void copy()}
            aria-label={`Copy ${label.toLowerCase()}`}
            aria-disabled={copyStatus === 'pending'}
            className='text-content-secondary hover:text-content-brand aria-disabled:cursor-wait aria-disabled:opacity-50'
          >
            <span className='pointer-events-none relative inline-flex size-4 items-center justify-center'>
              <Copy
                aria-hidden='true'
                className={cn(
                  'transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(.2,0,0,1)] motion-reduce:transition-none',
                  copied ? 'scale-25 opacity-0 blur-[4px]' : 'scale-100 opacity-100 blur-0',
                )}
              />
              <Check
                aria-hidden='true'
                className={cn(
                  'absolute inset-0 m-auto transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(.2,0,0,1)] motion-reduce:transition-none',
                  copied ? 'scale-100 opacity-100 blur-0' : 'scale-25 opacity-0 blur-[4px]',
                )}
              />
            </span>
          </Button>
        )}
      </header>
      {!empty && (
        <p
          role='status'
          aria-live='polite'
          aria-atomic='true'
          className={cn(
            'min-h-5 px-3 text-xs leading-5',
            copyStatus === 'error' ? 'text-content-error' : 'text-content-secondary',
          )}
        >
          {copyStatus === 'pending'
            ? 'Copying…'
            : copied
              ? 'Copied to clipboard.'
              : copyStatus === 'error'
                ? 'Copy failed.'
                : ''}
        </p>
      )}
      {copyStatus === 'error' && !empty && (
        <label className='mx-3 mt-2 flex flex-col gap-1 text-xs text-content-secondary'>
          Select the raw JSON below and copy it manually.
          <textarea
            readOnly
            value={text}
            rows={6}
            aria-label={`${label} raw JSON for manual copying`}
            onFocus={(event) => event.currentTarget.select()}
            className='max-h-64 w-full resize-y rounded-[3px] border border-border-primary bg-surface-tertiary p-2 font-mono text-xs leading-5 text-content-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-content-brand'
          />
        </label>
      )}
      <div className='p-3'>
        {empty ? (
          <p className='py-2 text-center text-[11px] text-content-tertiary'>No data</p>
        ) : mode === 'raw' || oversized ? (
          raw
        ) : conversation ? (
          <ConversationView messages={conversation.messages} tools={conversation.tools} />
        ) : answer ? (
          <Bubble message={answer} />
        ) : (
          <JsonTable value={value} />
        )}
      </div>
    </section>
  )
}
