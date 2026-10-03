import { createRootRouteWithContext } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { AppShell } from '../components/app-shell'

// The router context carries the QueryClient so route loaders can
// ensureQueryData() before the page renders (see lib/queries.ts).
export type RouterContext = { queryClient: QueryClient }

export const Route = createRootRouteWithContext<RouterContext>()({
  component: AppShell,
})
