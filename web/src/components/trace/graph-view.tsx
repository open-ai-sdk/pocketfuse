import { useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { Maximize, Minus, Plus } from 'lucide-react'
import { cn, formatDuration } from '../../lib/utils'
import { observationIsPending, type TreeNode } from '../../lib/trace-tree'
import { Button } from '../ui'
import { kindFor, toneDotVar } from './kinds'

// Graph view: the observation tree laid out left→right by depth as an SVG
// DAG. Column = depth, row = DFS arrival order within that column, edges
// are cubic béziers. Clicking (or focusing + Enter) a node selects it,
// syncing with the tree and timeline.
//
// The canvas scrolls natively (wheel/scrollbar) and also pans by dragging
// with the mouse; zoom scales the SVG via width/height while the viewBox
// stays at layout size, so scrollbars track the zoomed extent. A drag that
// moves more than a few pixels suppresses the click that follows it —
// panning never selects the node it started on.
//
// Layout constants are sized so a node's label (~18 chars at 11px) and a
// short duration both fit without clipping.

const NODE_W = 168
const NODE_H = 30
const COL_GAP = 56
const ROW_GAP = 10
const PAD = 12
const MAX_NODES = 200
const ZOOM_STEP = 0.2
const ZOOM_MIN = 0.4
const ZOOM_MAX = 2

type LaidNode = { node: TreeNode; x: number; y: number }
type LaidEdge = { id: string; d: string; from: string; to: string }

function truncate(text: string, max: number) {
  return text.length > max ? `${text.slice(0, max - 1)}…` : text
}

export function GraphView({
  roots,
  selectedId,
  onSelect,
}: {
  roots: TreeNode[]
  selectedId: string
  onSelect: (id: string) => void
}) {
  const { nodes, edges, width, height, total } = useMemo(() => layout(roots), [roots])
  const [zoom, setZoom] = useState(1)
  const scrollRef = useRef<HTMLDivElement>(null)
  const panMoved = useRef(false)
  const pan = useRef({ active: false, x: 0, y: 0, left: 0, top: 0 })

  if (total === 0) {
    return (
      <p className='px-2 py-8 text-center text-[11px] text-content-tertiary'>
        Nothing to graph — no observations
      </p>
    )
  }
  if (total > MAX_NODES) {
    return (
      <p className='px-2 py-8 text-center text-[11px] text-content-tertiary'>
        Too many observations to graph ({total}). Use the tree or timeline view.
      </p>
    )
  }

  const clampZoom = (next: number) => Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, next))
  const applyZoom = (next: number) => {
    const element = scrollRef.current
    const clamped = clampZoom(next)
    if (!element) {
      setZoom(clamped)
      return
    }
    // Keep the viewport anchored at the same relative point of the canvas.
    const ratioX =
      element.scrollWidth > element.clientWidth
        ? element.scrollLeft / (element.scrollWidth - element.clientWidth)
        : 0
    const ratioY =
      element.scrollHeight > element.clientHeight
        ? element.scrollTop / (element.scrollHeight - element.clientHeight)
        : 0
    setZoom(clamped)
    requestAnimationFrame(() => {
      element.scrollLeft = ratioX * (element.scrollWidth - element.clientWidth)
      element.scrollTop = ratioY * (element.scrollHeight - element.clientHeight)
    })
  }
  const resetView = () => {
    setZoom(1)
    const element = scrollRef.current
    if (element) {
      element.scrollLeft = 0
      element.scrollTop = 0
    }
  }

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    // Mouse drags pan; touch keeps native scrolling.
    if (event.pointerType !== 'mouse' || event.button !== 0) return
    const element = scrollRef.current
    if (!element) return
    pan.current = {
      active: true,
      x: event.clientX,
      y: event.clientY,
      left: element.scrollLeft,
      top: element.scrollTop,
    }
    panMoved.current = false
  }
  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!pan.current.active) return
    const element = scrollRef.current
    if (!element) return
    const dx = event.clientX - pan.current.x
    const dy = event.clientY - pan.current.y
    if (!panMoved.current && Math.abs(dx) + Math.abs(dy) > 3) {
      // Capture only once the drag is real — capturing on pointer-down
      // would retarget the click that follows a plain click on a node,
      // breaking selection.
      panMoved.current = true
      event.currentTarget.setPointerCapture(event.pointerId)
    }
    if (!panMoved.current) return
    element.scrollLeft = pan.current.left - dx
    element.scrollTop = pan.current.top - dy
  }
  const handlePointerUp = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!pan.current.active) return
    pan.current.active = false
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='flex shrink-0 items-center justify-end gap-1 border-b border-border-primary px-2.5 py-1.5'>
        <Button
          variant='ghost'
          size='icon-xs'
          aria-label='Zoom out'
          title='Zoom out'
          disabled={zoom <= ZOOM_MIN}
          onClick={() => applyZoom(zoom - ZOOM_STEP)}
          className='text-content-secondary'
        >
          <Minus />
        </Button>
        <span
          className='min-w-10 text-center font-mono text-[9.5px] tabular-nums text-content-tertiary'
          aria-live='polite'
        >
          {Math.round(zoom * 100)}%
        </span>
        <Button
          variant='ghost'
          size='icon-xs'
          aria-label='Zoom in'
          title='Zoom in'
          disabled={zoom >= ZOOM_MAX}
          onClick={() => applyZoom(zoom + ZOOM_STEP)}
          className='text-content-secondary'
        >
          <Plus />
        </Button>
        <Button
          variant='ghost'
          size='icon-xs'
          aria-label='Reset zoom'
          title='Reset zoom and pan'
          onClick={resetView}
          className='text-content-secondary'
        >
          <Maximize />
        </Button>
      </div>
      <div
        ref={scrollRef}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onClickCapture={(event) => {
          // A pan that moved is not a click on the graph.
          if (panMoved.current) {
            event.stopPropagation()
            panMoved.current = false
          }
        }}
        className={cn(
          'min-h-0 flex-1 touch-pan-y select-none overflow-auto p-3',
          'cursor-grab active:cursor-grabbing',
        )}
      >
        <svg
          role='img'
          aria-label='Observation graph'
          width={Math.round(width * zoom)}
          height={Math.round(height * zoom)}
          viewBox={`0 0 ${width} ${height}`}
          className='block'
        >
          {edges.map((edge) => {
            const highlighted = edge.from === selectedId || edge.to === selectedId
            return (
              <path
                key={edge.id}
                d={edge.d}
                fill='none'
                stroke={highlighted ? 'var(--border-brand)' : 'var(--border-secondary)'}
                strokeWidth={highlighted ? 1.5 : 1}
                vector-effect='non-scaling-stroke'
              />
            )
          })}
          {nodes.map(({ node, x, y }) => (
            <GraphNode
              key={node.observation.id}
              node={node}
              x={x}
              y={y}
              selected={node.observation.id === selectedId}
              onSelect={() => onSelect(node.observation.id)}
            />
          ))}
        </svg>
      </div>
    </div>
  )
}

function GraphNode({
  node,
  x,
  y,
  selected,
  onSelect,
}: {
  node: TreeNode
  x: number
  y: number
  selected: boolean
  onSelect: () => void
}) {
  const observation = node.observation
  const kind = kindFor(observation.type)
  const pending = observationIsPending(observation)
  const error = observation.level?.toUpperCase() === 'ERROR'
  const name = observation.name || 'Untitled observation'
  const stroke = error
    ? 'var(--border-danger)'
    : selected
      ? 'var(--border-brand)'
      : 'var(--border-secondary)'
  const fill = selected ? 'var(--surface-brand)' : 'var(--surface-primary)'

  return (
    <g
      transform={`translate(${x} ${y})`}
      role='button'
      tabIndex={0}
      aria-label={`${kind.label} ${name}`}
      aria-pressed={selected}
      className='cursor-pointer outline-none focus-visible:text-content-brand'
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          event.stopPropagation()
          onSelect()
        }
      }}
    >
      <title>{`${kind.label} · ${name} · ${formatDuration(node.durationMs)}`}</title>
      <rect
        width={NODE_W}
        height={NODE_H}
        rx={3}
        fill={fill}
        stroke={stroke}
        strokeWidth={selected ? 1.5 : 1}
        strokeDasharray={pending ? '3 2' : undefined}
      />
      <circle cx={14} cy={NODE_H / 2} r={3} fill={toneDotVar[kind.tone]} />
      <text x={24} y={NODE_H / 2 + 3.5} fontSize={11} fill='var(--content-primary)'>
        {truncate(name, 18)}
      </text>
      <text
        x={NODE_W - 8}
        y={NODE_H / 2 + 3.5}
        textAnchor='end'
        fontSize={9}
        fontFamily='var(--font-mono)'
        fill='var(--content-tertiary)'
      >
        {pending ? '···' : formatDuration(node.durationMs)}
      </text>
    </g>
  )
}

// Layered layout: x = depth, y = arrival order within the depth column.
function layout(roots: TreeNode[]) {
  const nodes: LaidNode[] = []
  const edges: LaidEdge[] = []
  const perDepth = new Map<number, number>()
  const positions = new Map<string, { x: number; y: number }>()
  let total = 0
  let maxDepth = 0

  const walk = (node: TreeNode) => {
    total += 1
    maxDepth = Math.max(maxDepth, node.depth)
    const slot = perDepth.get(node.depth) ?? 0
    const x = PAD + node.depth * (NODE_W + COL_GAP)
    const y = PAD + slot * (NODE_H + ROW_GAP)
    perDepth.set(node.depth, slot + 1)
    nodes.push({ node, x, y })
    positions.set(node.observation.id, { x, y })
    for (const child of node.children) walk(child)
  }
  roots.forEach(walk)

  for (const { node } of nodes) {
    for (const child of node.children) {
      const from = positions.get(node.observation.id)
      const to = positions.get(child.observation.id)
      if (!from || !to) continue
      const x1 = from.x + NODE_W
      const y1 = from.y + NODE_H / 2
      const x2 = to.x
      const y2 = to.y + NODE_H / 2
      const bend = COL_GAP * 0.6
      edges.push({
        id: `${node.observation.id}->${child.observation.id}`,
        from: node.observation.id,
        to: child.observation.id,
        d: `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`,
      })
    }
  }

  const width = PAD * 2 + maxDepth * (NODE_W + COL_GAP) + NODE_W
  const height = PAD * 2 + Math.max(1, ...perDepth.values()) * (NODE_H + ROW_GAP) - ROW_GAP
  return { nodes, edges, width, height, total }
}
