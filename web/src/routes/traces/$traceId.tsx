import { createFileRoute } from '@tanstack/react-router'
import { TraceDetailPage } from '../../pages/trace-detail-page'

// `?obs=<id>` deep-links a specific observation (per-event
// deep-link); selecting a node in the tree updates it.
export const Route = createFileRoute('/traces/$traceId')({
  validateSearch: (search: Record<string, unknown>): { obs?: string } => ({
    obs: typeof search.obs === 'string' && search.obs ? search.obs : undefined,
  }),
  component: TraceDetailPage,
})
