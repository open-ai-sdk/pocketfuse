import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight, MessagesSquare, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { SESSIONS_PAGE_SIZE, sessionsOptions } from '../lib/queries'
import { formatRelative } from '../lib/utils'
import { sessionLabel } from '../components/trace/linked-trace-list'
import { Button } from '../components/ui'
import { EmptyPanel, PageHeader, Panel } from './primitives'

const sessionExample = `curl http://127.0.0.1:7625/api/ingest \\
  -H 'Content-Type: application/json' \\
  -d '{"trace":{"id":"sample-session-trace","sessionId":"sample-session","projectId":"demo","name":"Synthetic session example","input":{"message":"Say hello"},"output":{"message":"Hello from the sample"}},"observations":[{"id":"sample-session-model","traceId":"sample-session-trace","type":"GENERATION","name":"Sample model call","model":"demo-model","input":{"message":"Say hello"},"output":{"message":"Hello from the sample"}}]}'`

export function SessionsPage() {
  const [page, setPage] = useState(1)
  const [copyStatus, setCopyStatus] = useState('')
  const sessions = useQuery(sessionsOptions(page))
  const rows = sessions.data?.data ?? []
  const total = sessions.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / SESSIONS_PAGE_SIZE))
  return (
    <div className='min-h-full text-content-primary'>
      <PageHeader
        eyebrow='Observe'
        title='Sessions'
        count={total.toLocaleString()}
        description='Conversations and threads: every trace that carries the same session or thread id.'
      />
      <div className='px-4 py-5 sm:px-6'>
        <Panel>
          {rows.length === 0 ? (
            <div>
              <EmptyPanel
                icon={<MessagesSquare className='size-4' />}
                title='No sessions yet'
                description='Traces join a session when their spans carry session.id, langfuse.session.id, thread_id or a {thread_id} HTTP route.'
              />
              <div className='mx-auto max-w-2xl space-y-3 px-4 pb-6 text-sm'>
                <p>
                  This synthetic example only writes data if you run it yourself. Use an isolated
                  instance; adjust the local port if needed. Re-running updates the same sample IDs.
                </p>
                <div className='flex flex-wrap gap-3'>
                  <a
                    className='inline-flex min-h-11 items-center font-medium text-content-brand underline'
                    href='https://open-ai-sdk.github.io/pocketfuse/guide/ingestion'
                    target='_blank'
                    rel='noreferrer'
                  >
                    View session setup
                    <span className='sr-only'> (opens public documentation in a new tab)</span>
                  </a>
                  <Button
                    className='min-h-11'
                    onClick={() => {
                      if (!navigator.clipboard) {
                        setCopyStatus('Clipboard unavailable. Select and copy the example below.')
                        return
                      }
                      void navigator.clipboard.writeText(sessionExample).then(
                        () => setCopyStatus('Synthetic example copied. No data has been sent.'),
                        () => setCopyStatus('Copy failed. Select and copy the example below.'),
                      )
                    }}
                  >
                    Copy synthetic example
                  </Button>
                </div>
                <label className='block font-medium' htmlFor='session-example'>
                  Synthetic ingest command
                </label>
                <textarea
                  id='session-example'
                  readOnly
                  value={sessionExample}
                  className='h-44 w-full rounded border border-border-secondary bg-surface-secondary p-3 font-mono text-xs'
                  onFocus={(event) => event.currentTarget.select()}
                />
                <p role='status' aria-live='polite' className='text-content-secondary'>
                  {copyStatus}
                </p>
              </div>
            </div>
          ) : (
            <ul className='divide-y divide-border-primary'>
              {rows.map((session) => (
                <li key={session.id}>
                  <Link
                    to='/sessions/$sessionId'
                    params={{ sessionId: session.id }}
                    className='flex flex-col items-stretch justify-between gap-3 px-4 py-3 hover:bg-surface-tertiary sm:flex-row sm:items-center'
                  >
                    <span className='min-w-0 flex-1'>
                      <span className='block break-words text-sm font-medium text-content-primary'>
                        {session.name || session.title || sessionLabel(session.id)}
                      </span>
                      <span className='block break-words text-xs text-content-tertiary'>
                        {sessionLabel(session.id)} · {session.projectId ?? 'Unknown project'}
                        {session.userId ? ` · user ${session.userId}` : ''}
                      </span>
                    </span>
                    <span className='flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary'>
                      {session.errorCount ? (
                        <span className='inline-flex items-center gap-1 font-medium text-content-error'>
                          <TriangleAlert className='size-3' />
                          {session.errorCount} failed
                        </span>
                      ) : null}
                      <span className='font-mono tabular-nums'>
                        {session.traceCount ?? 0} traces
                      </span>
                      <span>{formatRelative(session.lastUpdatedAt)}</span>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
          {totalPages > 1 && (
            <div className='flex items-center justify-end gap-1 border-t border-border-primary px-4 py-2.5'>
              <Button
                variant='ghost'
                size='icon-sm'
                disabled={page <= 1}
                onClick={() => setPage((value) => value - 1)}
                aria-label='Previous page'
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
                onClick={() => setPage((value) => value + 1)}
                aria-label='Next page'
              >
                <ChevronRight />
              </Button>
            </div>
          )}
        </Panel>
      </div>
    </div>
  )
}
