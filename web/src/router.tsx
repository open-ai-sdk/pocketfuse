import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import { BarChart3, Boxes, FileJson2, Settings2, Star, UsersRound } from 'lucide-react'
import { AppShell } from './components/app-shell'
import { DashboardPage } from './pages/dashboard-page'
import { PlaceholderPage } from './pages/placeholder-page'
import { TraceDetailPage } from './pages/trace-detail-page'
import { TracesPage } from './pages/traces-page'

const rootRoute = createRootRoute({ component: AppShell })
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: DashboardPage,
})
const tracesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/traces',
  component: TracesPage,
})
const traceDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/traces/$traceId',
  component: TraceDetailPage,
})
const sessionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions',
  component: () => (
    <PlaceholderPage
      title='Sessions'
      description='Group traces by conversations and user journeys.'
      icon={UsersRound}
    />
  ),
})
const scoresRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/scores',
  component: () => (
    <PlaceholderPage
      title='Scores'
      description='Review evaluation scores attached to your traces.'
      icon={Star}
    />
  ),
})
const datasetsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/datasets',
  component: () => (
    <PlaceholderPage
      title='Datasets'
      description='Manage local evaluation datasets.'
      icon={Boxes}
    />
  ),
})
const promptsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/prompts',
  component: () => (
    <PlaceholderPage
      title='Prompts'
      description='Inspect and version prompts used by your agents.'
      icon={FileJson2}
    />
  ),
})
const modelsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/models',
  component: () => (
    <PlaceholderPage
      title='Models & costs'
      description='Understand model usage and spend over time.'
      icon={BarChart3}
    />
  ),
})
const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: () => (
    <PlaceholderPage
      title='Settings'
      description='Configure your local Duckscope workspace.'
      icon={Settings2}
    />
  ),
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  tracesRoute,
  traceDetailRoute,
  sessionsRoute,
  scoresRoute,
  datasetsRoute,
  promptsRoute,
  modelsRoute,
  settingsRoute,
])
export const router = createRouter({ routeTree, defaultPreload: 'intent' })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
