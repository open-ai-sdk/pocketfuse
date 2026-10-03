import { createFileRoute } from '@tanstack/react-router'
import { TracesPage } from '../../pages/traces-page'
import { RouteError, TracesSkeleton } from '../../pages/route-skeletons'
import { TRACES_INITIAL_FILTERS, tracesOptions } from '../../lib/queries'

// Warm the query cache with the initial (unfiltered) list before the page
// renders — filter changes after that refetch client-side. Hovering the nav
// link preloads this loader, so clicking in is usually instant.
export const Route = createFileRoute('/traces/')({
  loader: ({ context }) =>
    context.queryClient.ensureQueryData(tracesOptions(TRACES_INITIAL_FILTERS)),
  pendingComponent: TracesSkeleton,
  errorComponent: RouteError,
  component: TracesPage,
})
