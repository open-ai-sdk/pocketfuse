import { useQuery } from '@tanstack/react-query'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import {
  Activity,
  BarChart3,
  Boxes,
  ChevronDown,
  CircleHelp,
  FileJson2,
  Gauge,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  RefreshCw,
  Settings2,
  Star,
  UsersRound,
  X,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { getHealth } from '../lib/api'
import { cn } from '../lib/utils'
import { Badge, Button, Card } from './ui'

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

type NavIcon = typeof Activity

type NavItemProps = {
  label: string
  to: string
  icon: NavIcon
  collapsed?: boolean
  onNavigate?: () => void
}

function NavItem({ label, to, icon: Icon, collapsed, onNavigate }: NavItemProps) {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const isActive = to === '/' ? pathname === '/' : pathname === to || pathname.startsWith(`${to}/`)

  return (
    <Link
      to={to}
      onClick={onNavigate}
      title={collapsed ? label : undefined}
      className={cn(
        'group flex h-9 items-center gap-3 rounded-md px-2 text-sm font-medium text-content-secondary transition-colors hover:bg-surface-overlay-primary hover:text-content-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-interactive-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary',
        collapsed && 'justify-center px-0',
        isActive &&
          'bg-interactive-secondary-hover text-content-brand hover:bg-interactive-secondary-press hover:text-content-brand',
      )}
    >
      <Icon
        className={cn(
          'size-5 shrink-0',
          isActive
            ? 'text-content-brand'
            : 'text-content-tertiary group-hover:text-content-primary',
        )}
        strokeWidth={1.8}
      />
      {!collapsed && <span className='truncate'>{label}</span>}
    </Link>
  )
}

function Sidebar({
  collapsed,
  onCollapse,
  onNavigate,
}: {
  collapsed: boolean
  onCollapse: () => void
  onNavigate?: () => void
}) {
  return (
    <aside
      className={cn(
        'group/sidebar flex h-screen shrink-0 flex-col border-r border-border-primary bg-surface-primary text-content-primary transition-[width] duration-200 motion-reduce:transition-none',
        collapsed ? 'w-12' : 'w-64',
      )}
    >
      <div
        className={cn(
          'relative flex h-12 shrink-0 items-center border-b border-border-primary px-2',
          collapsed ? 'justify-center' : 'gap-2',
        )}
      >
        <Link
          to='/'
          onClick={onNavigate}
          className='flex min-w-0 items-center gap-2 overflow-hidden text-content-primary'
          aria-label='Pocketfuse home'
        >
          <span className='flex size-6 shrink-0 items-center justify-center rounded-md bg-interactive-primary text-xs font-semibold text-content-white shadow-sm'>
            D
          </span>
          {!collapsed && (
            <span className='truncate text-sm font-medium tracking-tight'>pocketfuse</span>
          )}
        </Link>
        <Button
          variant='ghost'
          size='icon-sm'
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          onClick={onCollapse}
          className={cn(
            'text-content-tertiary hover:bg-interactive-tertiary-hover hover:text-content-primary',
            collapsed
              ? 'absolute right-1 top-2 opacity-0 group-hover/sidebar:opacity-100'
              : 'ml-auto',
          )}
        >
          {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
        </Button>
      </div>

      <div className='px-2 pt-4'>
        {!collapsed && (
          <p className='mb-2 px-2 text-[10px] font-medium uppercase tracking-[0.12em] text-content-tertiary'>
            Observe
          </p>
        )}
        <nav className='flex flex-col gap-1' aria-label='Observe'>
          {primaryNav.map((item) => (
            <NavItem key={item.to} {...item} collapsed={collapsed} onNavigate={onNavigate} />
          ))}
        </nav>
      </div>

      <div className='mt-5 px-2'>
        {!collapsed && (
          <p className='mb-2 px-2 text-[10px] font-medium uppercase tracking-[0.12em] text-content-tertiary'>
            Workspace
          </p>
        )}
        <nav className='flex flex-col gap-1' aria-label='Workspace'>
          {workspaceNav.map((item) => (
            <NavItem key={item.to} {...item} collapsed={collapsed} onNavigate={onNavigate} />
          ))}
        </nav>
      </div>

      <div className='mt-auto px-2 pb-3'>
        {!collapsed && (
          <Card
            size='sm'
            className='mb-3 rounded-md border border-border-primary bg-surface-secondary p-3 text-content-primary ring-0'
          >
            <div className='flex items-center gap-2 text-xs font-medium'>
              <span className='size-1.5 rounded-full bg-content-success' />
              Local project
            </div>
            <p className='mt-1 pl-3.5 text-[11px] text-content-tertiary'>SQLite · no auth</p>
          </Card>
        )}
        <nav className='flex flex-col gap-1' aria-label='Application'>
          <NavItem
            label='Settings'
            to='/settings'
            icon={Settings2}
            collapsed={collapsed}
            onNavigate={onNavigate}
          />
          <Button
            variant='ghost'
            size='lg'
            title={collapsed ? 'Help' : undefined}
            className={cn(
              'group w-full justify-start text-sm font-medium text-content-secondary hover:bg-interactive-tertiary-hover hover:text-content-primary',
              collapsed && 'justify-center px-0',
            )}
          >
            <CircleHelp strokeWidth={1.8} />
            {!collapsed && <span>Help & docs</span>}
          </Button>
        </nav>
      </div>
    </aside>
  )
}

function Topbar({ onMenu, onRefresh }: { onMenu: () => void; onRefresh: () => void }) {
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
    <header
      className='sticky top-0 z-40 flex h-12 shrink-0 items-center justify-between border-b border-border-primary bg-surface-primary px-4 text-content-primary md:px-8'
      aria-label='Main navigation'
    >
      <div className='flex min-w-0 items-center gap-3'>
        <Button
          variant='ghost'
          size='icon'
          aria-label='Open navigation'
          onClick={onMenu}
          className='text-content-secondary hover:bg-interactive-tertiary-hover hover:text-content-primary lg:hidden'
        >
          <Menu />
        </Button>
        <div className='flex min-w-0 items-center gap-2 text-sm'>
          <span className='text-content-tertiary'>Workspace</span>
          <span className='text-content-tertiary' aria-hidden='true'>
            /
          </span>
          <span className='truncate font-medium text-content-primary'>{pageName}</span>
        </div>
      </div>
      <div className='flex items-center gap-2'>
        <Badge
          tone='neutral'
          className='hidden gap-2 border-border-primary bg-surface-secondary px-2 py-1 text-xs text-content-secondary sm:inline-flex'
        >
          <span
            className={cn(
              'size-1.5 rounded-full',
              health.isSuccess
                ? 'bg-content-success'
                : health.isPending
                  ? 'bg-content-warning'
                  : 'bg-content-tertiary',
            )}
          />
          {health.isSuccess ? 'Connected' : health.isPending ? 'Checking' : 'Local mode'}
        </Badge>
        <Button
          variant='ghost'
          size='icon'
          aria-label='Refresh data'
          onClick={onRefresh}
          className='text-content-secondary hover:bg-interactive-tertiary-hover hover:text-content-primary'
        >
          <RefreshCw />
        </Button>
        <div className='hidden h-5 w-px bg-border-primary sm:block' aria-hidden='true' />
        <Button
          variant='ghost'
          size='sm'
          aria-label='Open account menu'
          className='gap-1.5 px-1.5 text-xs font-medium text-content-primary hover:bg-interactive-tertiary-hover'
        >
          <span className='flex size-6 items-center justify-center rounded-full bg-surface-brand text-[11px] font-semibold text-content-brand'>
            L
          </span>
          <ChevronDown className='text-content-tertiary' />
        </Button>
      </div>
    </header>
  )
}

export function AppShell() {
  const [collapsed, setCollapsed] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const router = useRouterState({ select: (state) => state.location.pathname })
  const closeMobile = () => setMobileOpen(false)

  return (
    <div className='flex h-[100dvh] overflow-hidden bg-background-primary'>
      <div
        className={cn(
          'fixed inset-0 z-40 bg-content-black/30 transition-opacity motion-reduce:transition-none lg:hidden',
          mobileOpen ? 'visible opacity-100' : 'pointer-events-none invisible opacity-0',
        )}
        onClick={closeMobile}
        aria-hidden='true'
      />
      <div
        className={cn(
          'fixed inset-y-0 left-0 z-50 transition-transform motion-reduce:transition-none lg:relative lg:z-auto lg:block lg:translate-x-0',
          mobileOpen ? 'translate-x-0' : '-translate-x-full',
        )}
      >
        <Sidebar
          collapsed={collapsed}
          onCollapse={() => setCollapsed((value) => !value)}
          onNavigate={closeMobile}
        />
        <Button
          variant='ghost'
          size='icon-xs'
          aria-label='Close navigation'
          onClick={closeMobile}
          className='absolute right-2 top-2 text-content-secondary hover:bg-interactive-tertiary-hover hover:text-content-primary lg:hidden'
        >
          <X />
        </Button>
      </div>
      <div className='flex min-w-0 flex-1 flex-col'>
        <Topbar
          onMenu={() => setMobileOpen(true)}
          onRefresh={() => window.dispatchEvent(new Event('pocketfuse:refresh'))}
        />
        <main key={router} className='min-h-0 flex-1 overflow-y-auto bg-background-primary'>
          <Outlet />
        </main>
      </div>
    </div>
  )
}

export function SectionHeading({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow?: string
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <div className='flex flex-col gap-4 border-b border-border-primary px-4 py-6 sm:flex-row sm:items-end sm:justify-between md:px-8'>
      <div className='min-w-0'>
        {eyebrow && (
          <p className='mb-1.5 text-xs font-medium uppercase tracking-[0.1em] text-content-brand'>
            {eyebrow}
          </p>
        )}
        <h1 className='truncate text-xl font-medium tracking-tight text-content-primary sm:text-2xl'>
          {title}
        </h1>
        {description && <p className='mt-1.5 text-sm text-content-secondary'>{description}</p>}
      </div>
      {actions && <div className='flex shrink-0 items-center gap-2'>{actions}</div>}
    </div>
  )
}
