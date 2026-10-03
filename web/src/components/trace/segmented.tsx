import type { KeyboardEvent } from 'react'
import { cn } from '../../lib/utils'

// Segmented control for the trace navigation panel: switches between
// tree / timeline / graph without leaving the page.
export function SegmentedControl<T extends string>({
  value,
  options,
  onChange,
  ariaLabel,
  className,
}: {
  value: T
  options: { value: T; label: string; disabled?: boolean; title?: string }[]
  onChange: (value: T) => void
  ariaLabel: string
  className?: string
}) {
  return (
    <div
      role='radiogroup'
      aria-label={ariaLabel}
      className={cn('flex rounded-[3px] border border-border-primary p-0.5', className)}
    >
      {options.map((option) => {
        const active = option.value === value
        return (
          <button
            key={option.value}
            type='button'
            role='radio'
            aria-checked={active}
            disabled={option.disabled}
            title={option.disabled ? option.title : option.label}
            onClick={() => !option.disabled && onChange(option.value)}
            className={cn(
              'flex-1 rounded-[2px] px-2 py-1 text-[11px] font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-interactive-primary',
              active
                ? 'bg-interactive-secondary-hover text-content-brand'
                : 'text-content-tertiary hover:text-content-secondary',
              option.disabled && 'cursor-not-allowed opacity-40 hover:text-content-tertiary',
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

export type TabBarTab<T extends string> = { id: T; label: string; count?: number }

// Underline tab bar for the detail panel (preview / attributes / scores /
// log view). Arrow keys move between tabs, matching tablist semantics.
export function TabBar<T extends string>({
  tabs,
  active,
  onChange,
  ariaLabel,
}: {
  tabs: TabBarTab<T>[]
  active: T
  onChange: (tab: T) => void
  ariaLabel: string
}) {
  const handleKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    const index = tabs.findIndex((tab) => tab.id === active)
    const next = tabs[(index + (event.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length]
    onChange(next.id)
    event.currentTarget.parentElement
      ?.querySelector<HTMLButtonElement>(`[data-tab-id="${next.id}"]`)
      ?.focus()
  }

  return (
    <div
      role='tablist'
      aria-label={ariaLabel}
      className='flex items-center gap-1 border-b border-border-primary px-3'
    >
      {tabs.map((tab) => {
        const isActive = tab.id === active
        return (
          <button
            key={tab.id}
            data-tab-id={tab.id}
            type='button'
            role='tab'
            aria-selected={isActive}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onChange(tab.id)}
            onKeyDown={handleKeyDown}
            className={cn(
              '-mb-px border-b-2 px-2.5 py-2 text-xs font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-interactive-primary',
              isActive
                ? 'border-border-brand text-content-primary'
                : 'border-transparent text-content-secondary hover:text-content-primary',
            )}
          >
            {tab.label}
            {tab.count !== undefined && tab.count > 0 && (
              <span className='ml-1.5 font-mono text-[9.5px] tabular-nums text-content-tertiary'>
                {tab.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
