import { queryOptions } from '@tanstack/react-query'
import { getScores, getTrace, getTraces, type TraceFilters } from './api'
import { observationIsPending } from './trace-tree'

// Shared queryOptions factories: route loaders ensureQueryData() with these
// exact options, pages read through the same key — one definition, no key
// drift between the loader and the component.
//
// Note: react-query drops `undefined` entries when hashing keys, so the
// initial page filters ({name: undefined, ...}) hash identically to
// TRACES_INITIAL_FILTERS.

export const TRACES_PAGE_SIZE = 25

/** The exact key the traces list builds on first render (no filters). */
export const TRACES_INITIAL_FILTERS: TraceFilters = { page: 1, limit: TRACES_PAGE_SIZE }

export function tracesOptions(filters: TraceFilters) {
  return queryOptions({
    queryKey: ['traces', filters],
    queryFn: () => getTraces(filters),
    placeholderData: (previous) => previous,
    // Empty result → keep watching for the first ingested trace.
    refetchInterval: (query) =>
      query.state.data?.total === 0 && !query.state.error ? 5000 : false,
  })
}

export function traceOptions(traceId: string) {
  return queryOptions({
    queryKey: ['trace', traceId],
    queryFn: () => getTrace(traceId),
    // Poll while any observation is still running, or while the trace has
    // no observations yet (the API omits `observations` then). Stops on
    // the first complete render; hidden tabs don't poll.
    refetchInterval: (query) => {
      const trace = query.state.data
      if (!trace) return false
      const observations = trace.observations ?? []
      return observations.length === 0 || observations.some(observationIsPending) ? 3000 : false
    },
  })
}

export function traceScoresOptions(traceId: string) {
  return queryOptions({
    queryKey: ['scores', traceId],
    queryFn: () => getScores({ traceId, limit: 200 }),
  })
}
