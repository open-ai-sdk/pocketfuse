import { createFileRoute } from '@tanstack/react-router'
import { SessionDetailPage } from '../../pages/session-detail-page'
import { RouteError, TraceDetailSkeleton } from '../../pages/route-skeletons'
import { sessionStoryOptions } from '../../lib/queries'

// `?noise=show` includes routine polling in the conversation. `?trace=` and
// `?obs=` open a turn or a span in the side panel, so a link lands in place.
export const Route = createFileRoute('/sessions/$sessionId')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { noise?: 'show'; trace?: string; obs?: string } => ({
    noise: search.noise === 'show' ? 'show' : undefined,
    trace: typeof search.trace === 'string' && search.trace ? search.trace : undefined,
    obs: typeof search.obs === 'string' && search.obs ? search.obs : undefined,
  }),
  loaderDeps: ({ search }) => ({ noise: search.noise }),
  loader: ({ context, params, deps }) =>
    context.queryClient.ensureQueryData(
      sessionStoryOptions(params.sessionId, deps.noise !== 'show'),
    ),
  pendingComponent: TraceDetailSkeleton,
  errorComponent: RouteError,
  component: SessionDetailPage,
})
