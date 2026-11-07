import { describe, expect, it } from 'vite-plus/test'
import type { Observation } from './api'
import { barLayout, buildObservationTree, displayName, visibleObservations } from './trace-tree'

const spans: Observation[] = [
  { id: 'turn', name: 'assistant.turn', startTime: '2026-01-01T00:00:00Z' },
  {
    id: 'call',
    parentObservationId: 'turn',
    name: 'openai.responses.stream',
    label: 'gpt',
    mergedSpans: ['stream'],
    startTime: '2026-01-01T00:00:01Z',
  },
  {
    id: 'stream',
    parentObservationId: 'call',
    mergedInto: 'call',
    name: 'chat_streaming',
    startTime: '2026-01-01T00:00:01Z',
  },
  {
    id: 'inner',
    parentObservationId: 'stream',
    name: 'inner',
    startTime: '2026-01-01T00:00:02Z',
  },
]

describe('visibleObservations', () => {
  it('folds a nested provider call and keeps its children in the tree', () => {
    const roots = buildObservationTree(visibleObservations(spans, false))
    expect(roots.map((node) => node.observation.id)).toEqual(['turn'])
    const call = roots[0].children[0]
    expect(call.observation.id).toBe('call')
    expect(call.children.map((node) => node.observation.id)).toEqual(['inner'])
  })

  it('shows every span when asked', () => {
    expect(visibleObservations(spans, true)).toHaveLength(4)
  })
})

describe('displayName', () => {
  it('prefers what the span means over the instrumentation name', () => {
    expect(displayName(spans[1])).toBe('gpt')
    expect(displayName(spans[0])).toBe('assistant.turn')
    expect(displayName({ id: 'x' })).toBe('Untitled observation')
  })
})

describe('barLayout', () => {
  it('places a bar inside the window', () => {
    expect(barLayout(25, 50, 100)).toEqual({ left: '25%', width: '50%' })
  })

  it('keeps a tiny bar visible and never overruns the window', () => {
    expect(barLayout(0, 0, 1000).width).toBe('0.5%')
    expect(barLayout(90, 500, 100)).toEqual({ left: '90%', width: '10%' })
    expect(barLayout(500, 10, 100)).toEqual({ left: '100%', width: '0%' })
  })
})
