import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import {
  Activity,
  BarChart3,
  Boxes,
  ChevronDown,
  CircleHelp,
  FileJson2,
  Gauge,
  RefreshCw,
  Settings2,
  Star,
  UserRound,
  UsersRound,
} from 'lucide-react'
import { getHealth } from '../lib/api'
import { Badge, Button } from './ui'
import { Separator } from './ui/separator'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from './ui/sidebar'

const primaryNav = [
  { label: 'Traces', to: '/traces', icon: Activity },
  { label: 'Sessions', to: '/sessions', icon: UsersRound },
  { label: 'Scores', to: '/scores', icon: Star },
]

const workspaceNav = [
  { label: 'Dashboard', to: '/', icon: Gauge },
  { label: 'Datasets', to: '/datasets', icon: Boxes },
  { label: 'Prompts', to: '/prompts', icon: FileJson2 },
  { label: 'Models & costs', to: '/models', icon: BarChart3 },
]

function useActivePath() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  return (to: string) =>
    to === '/' ? pathname === '/' : pathname === to || pathname.startsWith(`${to}/`)
}

function AppSidebar() {
  const isActive = useActivePath()
  return (
    <Sidebar variant='inset' collapsible='icon'>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              render={<Link to='/' aria-label='Pocketfuse home' />}
              tooltip='pocketfuse'
              className='px-1.5'
            >
              <span className='flex size-6 shrink-0 items-center justify-center rounded-md bg-primary text-xs font-semibold text-primary-foreground shadow-sm'>
                P
              </span>
              <span className='truncate text-sm font-medium tracking-tight'>pocketfuse</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Observe</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {primaryNav.map((item) => (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    render={<Link to={item.to} />}
                    isActive={isActive(item.to)}
                    tooltip={item.label}
                  >
                    <item.icon strokeWidth={1.8} />
                    <span>{item.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {workspaceNav.map((item) => (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    render={<Link to={item.to} />}
                    isActive={isActive(item.to)}
                    tooltip={item.label}
                  >
                    <item.icon strokeWidth={1.8} />
                    <span>{item.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <div className='rounded-lg border border-sidebar-border bg-sidebar-accent/50 px-3 py-2 text-xs group-data-[collapsible=icon]:hidden'>
              <div className='flex items-center gap-2 font-medium text-sidebar-foreground'>
                <span className='size-1.5 rounded-full bg-content-success' />
                Local project
              </div>
              <p className='mt-1 pl-3.5 text-muted-foreground'>SQLite · no auth</p>
            </div>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton render={<Link to='/settings' />} tooltip='Settings'>
              <Settings2 strokeWidth={1.8} />
              <span>Settings</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton tooltip='Help & docs'>
              <CircleHelp strokeWidth={1.8} />
              <span>Help & docs</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}

function Topbar() {
  const queryClient = useQueryClient()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const pageName =
    pathname === '/'
      ? 'Overview'
      : (pathname.split('/')[1]?.replace(/^./, (char) => char.toUpperCase()) ?? 'Overview')
  const health = useQuery({
    queryKey: ['health'],
    queryFn: getHealth,
    staleTime: 30_000,
    retry: false,
  })

  return (
    <header className='flex h-12 shrink-0 items-center gap-2 border-b border-sidebar-border bg-background px-4 md:px-6'>
      <SidebarTrigger aria-label='Toggle sidebar' />
      <Separator orientation='vertical' className='data-vertical:h-4 data-vertical:self-center' />
      <div className='flex min-w-0 items-center gap-2 text-sm'>
        <span className='hidden text-muted-foreground sm:inline'>Workspace</span>
        <span className='hidden text-muted-foreground/60 sm:inline' aria-hidden='true'>
          /
        </span>
        <span className='truncate font-medium text-foreground'>{pageName}</span>
      </div>
      <div className='ml-auto flex items-center gap-2'>
        <Badge
          tone='neutral'
          className='hidden gap-2 border-border-primary bg-surface-secondary px-2 py-1 text-xs text-content-secondary sm:inline-flex'
        >
          <span
            className={
              health.isSuccess
                ? 'size-1.5 rounded-full bg-content-success'
                : health.isPending
                  ? 'size-1.5 rounded-full bg-content-warning'
                  : 'size-1.5 rounded-full bg-content-tertiary'
            }
          />
          {health.isSuccess ? 'Connected' : health.isPending ? 'Checking' : 'Local mode'}
        </Badge>
        <Button
          variant='ghost'
          size='icon'
          aria-label='Refresh data'
          onClick={() => void queryClient.invalidateQueries()}
          className='text-content-secondary hover:bg-interactive-tertiary-hover hover:text-content-primary'
        >
          <RefreshCw />
        </Button>
        <Separator orientation='vertical' className='data-vertical:h-4 data-vertical:self-center' />
        <Button
          variant='ghost'
          size='sm'
          aria-label='Open account menu'
          className='gap-1.5 px-1.5 text-xs font-medium text-content-primary hover:bg-interactive-tertiary-hover'
        >
          <span className='flex size-6 items-center justify-center rounded-full bg-surface-brand text-content-brand'>
            <UserRound className='size-3.5' strokeWidth={1.8} />
          </span>
          <ChevronDown className='text-content-tertiary' />
        </Button>
      </div>
    </header>
  )
}

export function AppShell() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  return (
    <SidebarProvider className='h-svh overflow-hidden bg-background-primary'>
      <AppSidebar />
      {/* Canvas vs card differ by only ~3 RGB points in this palette — the
          stock inset shadow alone leaves the card edge invisible, so add
          the same hairline the panels use. shadow-none stops the ambient
          stacking on the ring (it read ~20% darker than every panel
          border); overflow-hidden clips the header's square corners to
          the card's rounded ones. */}
      <SidebarInset className='min-h-0 overflow-hidden shadow-none ring-1 ring-border-primary'>
        <Topbar />
        {/* key by pathname so each page starts scrolled to top */}
        <div key={pathname} className='min-h-0 flex-1 overflow-y-auto'>
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
