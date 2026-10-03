import {
  Bot,
  CircleDot,
  Database,
  GitBranch,
  Layers,
  Rows3,
  Sparkles,
  Wrench,
  type LucideIcon,
} from 'lucide-react'

// Observation type → label/icon/tone map, shared by the tree, timeline,
// graph and log views so one observation reads the same in every view.
export type KindTone = 'violet' | 'blue' | 'amber' | 'green' | 'neutral'

export type ObservationKind = { label: string; icon: LucideIcon; tone: KindTone }

export const KIND_BY_TYPE: Record<string, ObservationKind> = {
  GENERATION: { label: 'Generation', icon: Sparkles, tone: 'violet' },
  SPAN: { label: 'Span', icon: GitBranch, tone: 'blue' },
  EVENT: { label: 'Event', icon: CircleDot, tone: 'green' },
  AGENT: { label: 'Agent', icon: Bot, tone: 'violet' },
  TOOL: { label: 'Tool', icon: Wrench, tone: 'amber' },
  CHAIN: { label: 'Chain', icon: Layers, tone: 'blue' },
  RETRIEVER: { label: 'Retriever', icon: Database, tone: 'blue' },
  EMBEDDING: { label: 'Embedding', icon: Rows3, tone: 'neutral' },
}

export function kindFor(type?: string): ObservationKind {
  return (
    KIND_BY_TYPE[type?.toUpperCase() ?? ''] ?? {
      label: type || 'Observation',
      icon: Wrench,
      tone: 'neutral' as const,
    }
  )
}

// Icon chip (border + tinted surface + tone text) used by tree rows and
// detail headers.
export const toneChipClass: Record<KindTone, string> = {
  violet: 'border-border-secondary bg-surface-accent text-content-accent',
  blue: 'border-border-information bg-surface-information text-content-information',
  green: 'border-border-success bg-surface-success text-content-success',
  amber: 'border-border-warning bg-surface-warning text-content-warning',
  neutral: 'border-border-primary bg-surface-tertiary text-content-secondary',
}

// Solid bar fill for timeline lanes, keyed by the same tone.
export const toneBarClass: Record<KindTone, string> = {
  violet: 'bg-content-accent',
  blue: 'bg-action-info',
  green: 'bg-action-success',
  amber: 'bg-action-warning',
  neutral: 'bg-content-tertiary',
}

// SVG fill for graph dots (SVG needs a resolvable value, not a class).
export const toneDotVar: Record<KindTone, string> = {
  violet: 'var(--content-accent)',
  blue: 'var(--action-info)',
  green: 'var(--action-success)',
  amber: 'var(--action-warning)',
  neutral: 'var(--content-tertiary)',
}
