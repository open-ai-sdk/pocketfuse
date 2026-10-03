import type { Observation } from './api'

// Observation tree helpers: build a parent→children forest then
// flatten it for render
// with ancestor guide-line flags per depth.

export type TreeNode = {
  observation: Observation
  children: TreeNode[]
  depth: number
  startMs: number
  endMs?: number
  durationMs?: number
}

const toMs = (value?: string) => {
  if (!value) return undefined
  const ms = new Date(value).getTime()
  return Number.isFinite(ms) ? ms : undefined
}

export function observationStartMs(observation: Observation) {
  return toMs(observation.startTime) ?? toMs(observation.timestamp) ?? 0
}

export function observationEndMs(observation: Observation) {
  return (
    toMs(observation.endTime) ??
    (toMs(observation.startTime) !== undefined && observation.duration !== undefined
      ? toMs(observation.startTime)! + observation.duration
      : undefined)
  )
}

export function observationDurationMs(observation: Observation) {
  if (observation.duration !== undefined && Number.isFinite(observation.duration))
    return observation.duration
  const start = observationStartMs(observation)
  const end = observationEndMs(observation)
  return start && end !== undefined && end > start ? end - start : undefined
}

// Build a forest: dedupe by id (earliest start wins), children sorted by
// startTime, orphans (missing/self parent) become roots — same rules as
// treeBuilding rules.
export function buildObservationTree(observations: Observation[]): TreeNode[] {
  const nodes = new Map<string, TreeNode>()
  for (const observation of observations) {
    const existing = nodes.get(observation.id)
    const startMs = observationStartMs(observation)
    if (existing && existing.startMs <= startMs) continue
    nodes.set(observation.id, {
      observation,
      children: existing?.children ?? [],
      depth: 0,
      startMs,
      endMs: observationEndMs(observation),
      durationMs: observationDurationMs(observation),
    })
  }

  const roots: TreeNode[] = []
  for (const node of nodes.values()) {
    const parentId = node.observation.parentObservationId
    const parent = parentId ? nodes.get(parentId) : undefined
    if (parent && parent !== node) parent.children.push(node)
    else roots.push(node)
  }

  const sortAndDepth = (node: TreeNode, depth: number) => {
    node.depth = depth
    node.children.sort((a, b) => a.startMs - b.startMs)
    for (const child of node.children) sortAndDepth(child, depth + 1)
  }
  roots.sort((a, b) => a.startMs - b.startMs)
  for (const root of roots) sortAndDepth(root, 0)
  return roots
}

// FlatNode carries per-depth "ancestor has later sibling" flags which drive
// the ├/└ guide lines used to draw ancestor connectors.
export type FlatNode = {
  node: TreeNode
  /** treeLines[i] = ancestor at depth i still has siblings below this row. */
  treeLines: boolean[]
  isLastSibling: boolean
}

export function flattenTree(roots: TreeNode[], collapsed: ReadonlySet<string>): FlatNode[] {
  const rows: FlatNode[] = []
  const walk = (node: TreeNode, treeLines: boolean[], isLastSibling: boolean) => {
    rows.push({ node, treeLines, isLastSibling })
    if (collapsed.has(node.observation.id)) return
    const last = node.children.length - 1
    node.children.forEach((child, index) => {
      walk(child, [...treeLines, index !== last], index === last)
    })
  }
  const last = roots.length - 1
  roots.forEach((root, index) => walk(root, [], index === last))
  return rows
}

// Timeline window: min start across the tree, max end. Trace bounds are only
// a fallback when NO observation carries timing — trace.created_at is the
// ingest time and must never stretch the window.
export function traceWindow(
  roots: TreeNode[],
  traceStart?: string,
  traceEnd?: string,
): { originMs: number; endMs: number } {
  let min = Number.POSITIVE_INFINITY
  let max = 0
  const visit = (node: TreeNode) => {
    if (node.startMs && node.startMs < min) min = node.startMs
    const end = node.endMs ?? (node.durationMs ? node.startMs + node.durationMs : node.startMs)
    if (end > max) max = end
    node.children.forEach(visit)
  }
  roots.forEach(visit)
  if (Number.isFinite(min) && max > 0) {
    return { originMs: min, endMs: Math.max(max, min + 1) }
  }
  const fallbackStart = toMs(traceStart) ?? toMs(traceEnd) ?? Date.now()
  const fallbackEnd = toMs(traceEnd) ?? fallbackStart
  return { originMs: fallbackStart, endMs: Math.max(fallbackEnd, fallbackStart + 1) }
}
// Normalized token counts: explicit columns win, else read the raw `usage`
// blob (usage_details style), else sum input+output.
export function observationTokens(observation: Observation): {
  input?: number
  output?: number
  total?: number
} {
  const usage = observation.usage ?? {}
  const pick = (...keys: string[]) => {
    for (const key of keys) {
      const value = usage[key]
      if (typeof value === 'number' && Number.isFinite(value)) return value
    }
    return undefined
  }
  const input = observation.promptTokens ?? pick('input', 'promptTokens', 'prompt_tokens')
  const output =
    observation.completionTokens ?? pick('output', 'completionTokens', 'completion_tokens')
  const total =
    observation.totalTokens ??
    pick('total', 'totalTokens', 'total_tokens') ??
    (input !== undefined || output !== undefined ? (input ?? 0) + (output ?? 0) : undefined)
  return { input, output, total }
}

// An observation with a start but no end (and no duration) is still running:
// it renders as a pending row with an animated in-flight bar.
export function observationIsPending(observation: Observation): boolean {
  // EVENTs are instants, not spans: never treat them as pending.
  if ((observation.type ?? '').toUpperCase() === 'EVENT') return false
  return (
    observationStartMs(observation) > 0 &&
    observationEndMs(observation) === undefined &&
    observation.duration === undefined
  )
}

// Start offset inside the trace window, for the elapsed-since-trace-start
// label shown as '+T'.
export function observationOffsetMs(node: TreeNode, originMs: number): number {
  return Math.max(0, node.startMs - originMs)
}
