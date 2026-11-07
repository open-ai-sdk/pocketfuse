import { createFileRoute } from '@tanstack/react-router'
import { SessionsPage } from '../../pages/sessions-page'
import { RouteError, TracesSkeleton } from '../../pages/route-skeletons'
import { sessionsOptions } from '../../lib/queries'

export const Route = createFileRoute('/sessions/')({
  loader: ({ context }) => context.queryClient.ensureQueryData(sessionsOptions(1)),
  pendingComponent: TracesSkeleton,
  errorComponent: RouteError,
  component: SessionsPage,
})
