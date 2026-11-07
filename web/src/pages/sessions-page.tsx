import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight, MessagesSquare, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import { SESSIONS_PAGE_SIZE, sessionsOptions } from '../lib/queries'
import { formatRelative } from '../lib/utils'
import { sessionLabel } from '../components/trace/linked-trace-list'
import { Button } from '../components/ui'
import { EmptyPanel, PageHeader, Panel } from './primitives'

export function SessionsPage() {
  const [page, setPage] = useState(1)
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
            <EmptyPanel
              icon={<MessagesSquare className='size-4' />}
              title='No sessions yet'
              description='Traces join a session when their spans carry session.id, langfuse.session.id, thread_id or a {thread_id} HTTP route.'
            />
          ) : (
            <ul className='divide-y divide-border-primary'>
              {rows.map((session) => (
                <li key={session.id}>
                  <Link
                    to='/sessions/$sessionId'
                    params={{ sessionId: session.id }}
                    className='flex flex-wrap items-center justify-between gap-3 px-4 py-3 hover:bg-surface-tertiary'
                  >
                    <span className='min-w-0 flex-1'>
                      <span className='block truncate text-xs font-medium text-content-primary'>
                        {session.name || session.title || sessionLabel(session.id)}
                      </span>
                      <span className='block truncate text-[10.5px] text-content-tertiary'>
                        {sessionLabel(session.id)} · {session.projectId ?? 'Unknown project'}
                        {session.userId ? ` · user ${session.userId}` : ''}
                      </span>
                    </span>
                    <span className='flex shrink-0 items-center gap-4 text-[11px] text-content-secondary'>
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
