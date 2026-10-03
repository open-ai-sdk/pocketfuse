import { createFileRoute } from '@tanstack/react-router'
import { TraceDetailPage } from '../../pages/trace-detail-page'
import { RouteError, TraceDetailSkeleton } from '../../pages/route-skeletons'
import { traceOptions, traceScoresOptions } from '../../lib/queries'

// `?obs=<id>` deep-links a specific observation; `?view=` and `?tab=`
// deep-link the left-panel view (tree/timeline/graph) and the right-panel
// detail tab. Defaults (tree / preview) stay out of the URL.
export const Route = createFileRoute('/traces/$traceId')({
  validateSearch: (
    search: Record<string, unknown>,
  ): {
    obs?: string
    view?: 'tree' | 'timeline' | 'graph'
    tab?: 'preview' | 'attributes' | 'scores' | 'log'
  } => ({
    obs: typeof search.obs === 'string' && search.obs ? search.obs : undefined,
    view: search.view === 'timeline' || search.view === 'graph' ? search.view : undefined,
    tab:
      search.tab === 'attributes' || search.tab === 'scores' || search.tab === 'log'
        ? search.tab
        : undefined,
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
