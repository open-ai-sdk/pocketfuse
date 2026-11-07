import { createFileRoute } from '@tanstack/react-router'
import { TraceDetailPage } from '../../pages/trace-detail-page'
import { RouteError, TraceDetailSkeleton } from '../../pages/route-skeletons'
import { traceOptions, traceScoresOptions } from '../../lib/queries'

// `?obs=<id>` deep-links a span; `?view=` picks the left pane (tree stays out of
// the URL); `?raw=1` shows every span instead of folding nested provider calls.
export const Route = createFileRoute('/traces/$traceId')({
  validateSearch: (
    search: Record<string, unknown>,
  ): {
    obs?: string
    view?: 'graph' | 'log'
    raw?: 1
  } => ({
    obs: typeof search.obs === 'string' && search.obs ? search.obs : undefined,
    view: search.view === 'graph' || search.view === 'log' ? search.view : undefined,
    raw: search.raw === 1 || search.raw === '1' ? 1 : undefined,
  }),
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.ensureQueryData(traceOptions(params.traceId)),
      context.queryClient.ensureQueryData(traceScoresOptions(params.traceId)),
    ]).then(() => undefined),
  pendingComponent: TraceDetailSkeleton,
  errorComponent: RouteError,
  component: TraceDetailPage,
})
