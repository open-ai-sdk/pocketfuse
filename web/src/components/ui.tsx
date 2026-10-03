import type { ReactNode } from 'react'
import { cn } from '../lib/utils'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Badge as ShadcnBadge } from './ui/badge'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from './ui/card'
import { Alert, AlertAction, AlertDescription, AlertTitle } from './ui/alert'
import { Skeleton } from './ui/skeleton'
import { Separator } from './ui/separator'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from './ui/empty'

export {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
  Button,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Input,
  Separator,
  Skeleton,
}

export function Badge({
  className,
  tone = 'neutral',
  children,
}: {
  className?: string
  tone?: 'neutral' | 'green' | 'red' | 'amber' | 'blue' | 'violet'
  children: ReactNode
}) {
  const variant = tone === 'red' ? 'destructive' : tone === 'neutral' ? 'outline' : 'secondary'
  return (
    <ShadcnBadge
      variant={variant}
      className={cn(
        tone === 'green' && 'border-emerald-200 bg-emerald-50 text-emerald-700',
        tone === 'amber' && 'border-amber-200 bg-amber-50 text-amber-700',
        tone === 'blue' && 'border-sky-200 bg-sky-50 text-sky-700',
        tone === 'violet' && 'border-violet-200 bg-violet-50 text-violet-700',
        className,
      )}
    >
      {children}
    </ShadcnBadge>
  )
}

export function Divider({ className }: { className?: string }) {
  return <Separator className={className} />
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <Empty className='min-h-[280px] px-6 text-center'>
      <EmptyHeader>
        {icon && <EmptyMedia variant='icon'>{icon}</EmptyMedia>}
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  )
}
