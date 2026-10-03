import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { ArrowUpRight } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { Badge, Card, CardDescription, CardHeader, CardTitle, EmptyState } from '../components/ui'
import { cn } from '../lib/utils'

/** Shared visual tokens for Pocketfuse surfaces, expressed with Tailwind. */
export const panelSurface =
  'rounded-[4px] border border-border-primary bg-surface-primary shadow-none ring-0'
export const panelSubtle = 'border-border-primary bg-surface-tertiary'
export const panelBrand = 'var(--content-brand)'

export function PageHeader({
  title,
  description,
  count,
  actions,
  eyebrow,
}: {
  title: string
  description?: string
  count?: string | number
  actions?: ReactNode
  eyebrow?: string
}) {
  return (
    <header className='flex flex-col gap-3 border-b border-border-primary bg-background-primary px-4 py-5 sm:flex-row sm:items-start sm:justify-between sm:px-6'>
      <div className='min-w-0'>
        <div className='flex flex-wrap items-center gap-2'>
          {eyebrow && (
            <span className='font-mono text-[10px] font-medium uppercase tracking-[0.12em] text-content-secondary'>
              {eyebrow}
            </span>
          )}
          <h1 className='truncate text-[22px] font-semibold leading-7 tracking-[-0.02em] text-content-primary'>
            {title}
          </h1>
          {count !== undefined && (
            <Badge
              tone='neutral'
              className='h-5 rounded-[3px] border-border-secondary bg-surface-secondary px-1.5 font-mono text-[10px] text-content-secondary'
            >
              {count}
            </Badge>
          )}
        </div>
        {description && (
          <p className='mt-1.5 max-w-2xl text-xs leading-5 text-content-secondary'>{description}</p>
        )}
      </div>
      {actions && <div className='flex shrink-0 flex-wrap items-center gap-2'>{actions}</div>}
    </header>
  )
}

export function MetricStrip({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        'grid grid-cols-2 overflow-hidden rounded-[4px] border border-border-primary bg-surface-primary sm:grid-cols-4',
        className,
      )}
    >
      {children}
    </div>
  )
}

export function MetricCell({
  label,
  value,
  detail,
  icon: Icon,
  className,
}: {
  label: string
  value: ReactNode
  detail?: string
  icon?: LucideIcon
  className?: string
}) {
  return (
    <div
      className={cn(
        'min-w-0 border-b border-border-primary px-4 py-3.5 sm:border-b-0 sm:border-r last:border-r-0 odd:sm:border-b-0 even:border-b-0',
        className,
      )}
    >
      <div className='flex items-center justify-between gap-2'>
        <span className='truncate text-[11px] font-medium text-content-secondary'>{label}</span>
        {Icon && <Icon className='size-3.5 text-content-brand' aria-hidden='true' />}
      </div>
      <p className='mt-1.5 truncate text-lg font-semibold tracking-[-0.02em] text-content-primary tabular-nums'>
        {value}
      </p>
      {detail && <p className='mt-0.5 truncate text-[10px] text-content-tertiary'>{detail}</p>}
    </div>
  )
}

export function Panel({
  title,
  description,
  action,
  children,
  className,
}: {
  title?: string
  description?: string
  action?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <Card className={cn(panelSurface, className)}>
      {(title || description || action) && (
        <CardHeader className='flex flex-row items-start justify-between gap-3 border-b border-border-primary px-4 py-3'>
          <div className='min-w-0'>
            {title && (
              <CardTitle className='text-sm font-semibold text-content-primary'>{title}</CardTitle>
            )}
            {description && (
              <CardDescription className='mt-0.5 text-[11px] leading-4 text-content-secondary'>
                {description}
              </CardDescription>
            )}
          </div>
          {action && <div className='shrink-0'>{action}</div>}
        </CardHeader>
      )}
      {children}
    </Card>
  )
}

export function EmptyPanel({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode
  title: string
  description: string
  action?: ReactNode
}) {
  return <EmptyState icon={icon} title={title} description={description} action={action} />
}

export function StatusChip({ status, className }: { status?: string; className?: string }) {
  const value = status?.toLowerCase()
  const tone =
    value === 'success' || value === 'completed' || value === 'ok'
      ? 'green'
      : value === 'error' || value === 'failed' || value === 'failure'
        ? 'red'
        : value === 'pending' || value === 'running'
          ? 'amber'
          : 'neutral'
  const label = status ? status.charAt(0).toUpperCase() + status.slice(1).toLowerCase() : 'Unknown'
  return (
    <Badge
      tone={tone}
      className={cn(
        'h-5 rounded-[3px] px-1.5 font-mono text-[10px] uppercase tracking-[0.02em]',
        className,
      )}
    >
      {label}
    </Badge>
  )
}

export function BackLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Link
      to={to}
      className='inline-flex items-center gap-1 text-xs font-medium text-content-secondary underline-offset-2 hover:text-content-brand hover:underline'
    >
      {children}
      <ArrowUpRight className='size-3' aria-hidden='true' />
    </Link>
  )
}
