import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { AlertCircle, RefreshCw } from 'lucide-react'
import { cn } from '../lib/utils'
import { Skeleton } from '../components/ui'
import { panelSurface } from './primitives'

// Route-level pending/error components. pendingComponent replaces the page
// area (shell/topbar stay) while a route loader resolves, so its skeleton
// mirrors the page it stands in for. With defaultPendingMs = 250 the local
// API usually wins the race and no skeleton ever flashes.

/** Traces list: metric strip + filter rail ghost + table row ghosts. */
export function TracesSkeleton() {
  return (
    <div className='flex min-h-full flex-col gap-5 px-4 py-5 sm:px-6'>
      <div className='grid grid-cols-2 overflow-hidden rounded-[4px] border border-border-primary bg-surface-primary sm:grid-cols-4'>
        {Array.from({ length: 4 }).map((_, index) => (
          <div
            key={index}
            className='border-b border-border-primary px-4 py-3.5 sm:border-b-0 sm:border-r'
          >
            <Skeleton className='h-3 w-20' />
            <Skeleton className='mt-2 h-5 w-16' />
          </div>
        ))}
      </div>
      <div className='flex flex-col items-start gap-5 lg:flex-row'>
        <div className='hidden w-60 shrink-0 rounded-[4px] border border-border-primary bg-surface-primary p-3 lg:block'>
          <Skeleton className='h-3.5 w-12' />
          {Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className='mt-3.5 h-3 w-full' />
          ))}
        </div>
        <div className={cn(panelSurface, 'w-full min-w-0 flex-1 self-stretch')}>
          <div className='flex items-center justify-between border-b border-border-primary px-4 py-3'>
            <Skeleton className='h-4 w-28' />
            <Skeleton className='h-4 w-40' />
          </div>
          <div className='divide-y divide-border-primary'>
            {Array.from({ length: 7 }).map((_, index) => (
              <div key={index} className='flex h-[58px] items-center gap-5 px-4'>
                <Skeleton className='h-3.5 w-44' />
                <Skeleton className='h-5 w-16' />
                <Skeleton className='h-3.5 w-20' />
                <Skeleton className='h-3.5 w-14' />
                <Skeleton className='ml-auto h-3.5 w-24' />
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

/** Trace detail: header + metrics + tree column + detail pane ghosts. */
export function TraceDetailSkeleton() {
  return (
    <div className='flex min-h-full flex-col px-4 py-4 sm:px-6'>
      <Skeleton className='h-3 w-24' />
      <div className='mt-3 flex items-center gap-2'>
        <Skeleton className='h-7 w-64' />
        <Skeleton className='h-5 w-16' />
      </div>
      <Skeleton className='mt-2 h-3 w-52' />
      <div className='mt-4 grid grid-cols-2 overflow-hidden rounded-[4px] border border-border-primary bg-surface-primary sm:grid-cols-4'>
        {Array.from({ length: 4 }).map((_, index) => (
          <div
            key={index}
            className='border-b border-border-primary px-4 py-3.5 sm:border-b-0 sm:border-r'
          >
            <Skeleton className='h-3 w-16' />
            <Skeleton className='mt-2 h-5 w-20' />
          </div>
        ))}
      </div>
      <div className='mt-4 grid min-h-[420px] flex-1 gap-0 xl:grid-cols-[380px_minmax(0,1fr)]'>
        <div className='flex flex-col border-r border-border-primary'>
          <div className='flex gap-1 border-b border-border-primary px-2.5 py-2'>
            <Skeleton className='h-6 flex-1' />
          </div>
          <div className='flex flex-col gap-2 p-2'>
            {Array.from({ length: 5 }).map((_, index) => (
              <div key={index} className='flex items-center gap-2'>
                <Skeleton className='size-5 rounded-[3px]' />
                <Skeleton className='h-3 flex-1' style={{ maxWidth: `${90 - index * 12}%` }} />
                <Skeleton className='h-1.5 w-16 rounded-full' />
              </div>
            ))}
          </div>
        </div>
        <div className='flex flex-col px-4 py-4 sm:px-5'>
          <div className='flex gap-2 border-b border-border-primary pb-2'>
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className='h-3.5 w-16' />
            ))}
          </div>
          <div className='mt-4 grid gap-3 lg:grid-cols-2'>
            {Array.from({ length: 2 }).map((_, index) => (
              <div key={index} className='rounded-[4px] border border-border-primary p-3'>
                <Skeleton className='h-3 w-14' />
                {Array.from({ length: 3 }).map((_, row) => (
                  <Skeleton key={row} className='mt-3 h-3 w-full' />
                ))}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

/** Route-level loader failure (e.g. API down on direct navigation). */
export function RouteError({ error }: ErrorComponentProps) {
  const router = useRouter()
  return (
    <div className='flex min-h-full items-start justify-center px-4 py-16'>
      <div className={cn(panelSurface, 'w-full max-w-md rounded-[4px] px-4 py-4')}>
        <div className='flex items-start gap-2.5'>
          <AlertCircle className='mt-0.5 size-4 shrink-0 text-content-error' />
          <div className='min-w-0'>
            <p className='text-sm font-semibold text-content-primary'>Could not load this page</p>
            <p className='mt-1 break-words text-xs text-content-secondary'>
              {error instanceof Error
                ? error.message
                : String(error ?? 'The local API is unavailable.')}
            </p>
            <button
              type='button'
              onClick={() => void router.invalidate()}
              className='mt-3 inline-flex items-center gap-1.5 rounded-[3px] border border-border-secondary px-2.5 py-1.5 text-xs font-medium text-content-primary transition-colors hover:bg-surface-tertiary'
            >
              <RefreshCw className='size-3' />
              Try again
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
