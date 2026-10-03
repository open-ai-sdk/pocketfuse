import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { cn, prettyJson } from '../lib/utils'
import { Badge, Button } from './ui'

// IO preview: Formatted view detects ChatML
// message arrays (or {messages: [...]}) and renders chat bubbles; anything
// else renders as a key-value table. Raw is pretty-printed JSON. Data is
// already in-memory so parsing is synchronous and size-gated.

type ChatMessage = {
  role?: string
  name?: string
  content?: unknown
  toolCalls?: unknown
}

const MAX_RENDER_CHARS = 300_000

function messageContentText(content: unknown): string {
  if (typeof content === 'string') return content
  if (Array.isArray(content))
    return content
      .map((part) => {
        if (part !== null && typeof part === 'object' && !Array.isArray(part)) {
          const fields = part as Record<string, unknown>
          return typeof fields.text === 'string'
            ? fields.text
            : typeof fields.content === 'string'
              ? fields.content
              : JSON.stringify(part)
        }
        return typeof part === 'string' ? part : JSON.stringify(part)
      })
      .filter((part): part is string => typeof part === 'string')
      .join('\n')
  return content === undefined || content === null ? '' : JSON.stringify(content, null, 2)
}

// Detects ChatML-ish input: an array of {role|content} objects, or a single
// message, or {messages: [...]}.
function extractMessages(value: unknown): ChatMessage[] | null {
  const asMessage = (item: unknown): ChatMessage | null => {
    if (item === null || typeof item !== 'object' || Array.isArray(item)) return null
    const message = item as ChatMessage
    return typeof message.role === 'string' || message.content !== undefined ? message : null
  }

  let candidates: unknown[] | null = null
  if (Array.isArray(value)) {
    candidates = value
  } else if (value !== null && typeof value === 'object') {
    if ('messages' in value && Array.isArray(value.messages)) candidates = value.messages
    else if (asMessage(value)) candidates = [value]
  }
  if (!candidates || candidates.length === 0) return null
  const messages = candidates.map(asMessage)
  if (!messages.every((m): m is ChatMessage => m !== null) || !messages.some((m) => m.role))
    return null
  return messages
}

function ChatBubble({ message }: { message: ChatMessage }) {
  const role = (message.role ?? 'message').toLowerCase()
  const tone =
    role === 'assistant'
      ? 'violet'
      : role === 'system'
        ? 'amber'
        : role === 'tool'
          ? 'blue'
          : 'neutral'
  return (
    <div className='rounded-[3px] border border-border-primary bg-surface-primary px-3 py-2'>
      <div className='mb-1 flex items-center gap-1.5'>
        <Badge tone={tone} className='rounded-[3px] px-1 font-mono text-[9.5px] uppercase'>
          {message.name ?? message.role ?? 'message'}
        </Badge>
      </div>
      <p className='whitespace-pre-wrap break-words text-[11.5px] leading-5 text-content-primary'>
        {messageContentText(message.content) || (
          <span className='text-content-tertiary'>Empty content</span>
        )}
      </p>
      {message.toolCalls !== undefined && (
        <pre className='mt-1.5 overflow-auto rounded-[3px] border border-border-primary bg-surface-tertiary p-2 font-mono text-[10.5px] text-content-secondary'>
          {prettyJson(message.toolCalls)}
        </pre>
      )}
    </div>
  )
}

// Non-chat fallback: top-level object → key-value table (same as
// JsonInputOutputView); arrays/primitives → pretty JSON.
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
              <span className='whitespace-pre-wrap break-words text-[11.5px] text-content-primary'>
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
  const [copied, setCopied] = useState(false)
  const text = prettyJson(value)
  const oversized = text.length > MAX_RENDER_CHARS
  const messages = mode === 'formatted' && !oversized ? extractMessages(value) : null
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1300)
    } catch {
      /* clipboard unavailable in non-secure contexts */
    }
  }

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
                className={cn(
                  'rounded-[2px] px-1.5 py-0.5 text-[10px] capitalize',
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
        {value !== undefined && value !== null && (
          <Button
            variant='ghost'
            size='icon-xs'
            onClick={() => void copy()}
            aria-label={`Copy ${label.toLowerCase()}`}
            className='text-content-secondary hover:text-content-brand'
          >
            {copied ? <Check /> : <Copy />}
          </Button>
        )}
      </header>
      <div className='max-h-96 overflow-auto p-3'>
        {value === undefined || value === null ? (
          <p className='py-2 text-center text-[11px] text-content-tertiary'>No data</p>
        ) : oversized ? (
          <pre className='overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-5 text-content-secondary'>
            {text}
          </pre>
        ) : messages ? (
          <div className='flex flex-col gap-2'>
            {messages.map((message, index) => (
              <ChatBubble key={index} message={message} />
            ))}
          </div>
        ) : mode === 'formatted' ? (
          <JsonTable value={value} />
        ) : (
          <pre className='overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-5 text-content-secondary'>
            {text}
          </pre>
        )}
      </div>
    </section>
  )
}
