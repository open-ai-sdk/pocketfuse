export type TraceStatus = 'success' | 'error' | 'pending' | 'unknown'

export type Trace = {
  id: string
  name?: string
  project?: string
  userId?: string
  sessionId?: string
  release?: string
  version?: string
  timestamp?: string
  startTime?: string
  endTime?: string
  duration?: number
  latency?: number
  status?: TraceStatus | string
  input?: unknown
  output?: unknown
  metadata?: Record<string, unknown>
  environment?: string
  observations?: Observation[]
  observationCount?: number
  totalCost?: number
  totalTokens?: number
  tags?: string[]
}

export type Observation = {
  id: string
  traceId?: string
  parentObservationId?: string
  name?: string
  type?: string
  startTime?: string
  endTime?: string
  timestamp?: string
  duration?: number
  input?: unknown
  output?: unknown
  metadata?: Record<string, unknown>
  usage?: Record<string, unknown>
  model?: string
  modelParameters?: Record<string, unknown>
  promptTokens?: number
  completionTokens?: number
  totalTokens?: number
  level?: string
  statusMessage?: string
  cost?: number
}

export type Paginated<T> = {
  data: T[]
  total: number
  page: number
  limit: number
}

export type Score = {
  id: string
  traceId?: string
  observationId?: string
  name?: string
  value?: number
  stringValue?: string
  dataType?: string
  source?: string
  comment?: string
  timestamp?: string
}

export type TraceFilters = {
  project?: string
  name?: string
  traceId?: string
  observationId?: string
  type?: string
  from?: string
  to?: string
  page?: number
  limit?: number
}

const API_BASE = import.meta.env.VITE_API_BASE ?? ''

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function stringValue(record: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value) return value
  }
  return undefined
}

function numberValue(record: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'number' && Number.isFinite(value)) return value
    if (typeof value === 'string' && value.trim() !== '' && Number.isFinite(Number(value)))
      return Number(value)
  }
  return undefined
}

function normalizeObservation(raw: unknown): Observation {
  const item = asRecord(raw)
  return {
    id: stringValue(item, 'id', 'observationId') ?? crypto.randomUUID(),
    traceId: stringValue(item, 'traceId', 'trace_id'),
    parentObservationId: stringValue(
      item,
      'parentObservationId',
      'parent_observation_id',
      'parentId',
    ),
    name: stringValue(item, 'name', 'observationName'),
    type: stringValue(item, 'type', 'observationType'),
    startTime: stringValue(item, 'startTime', 'start_time', 'timestamp'),
    endTime: stringValue(item, 'endTime', 'end_time'),
    timestamp: stringValue(item, 'timestamp', 'startTime', 'start_time'),
    duration: numberValue(item, 'duration', 'latency'),
    input: item.input,
    output: item.output,
    metadata: asRecord(item.metadata),
    usage: asRecord(item.usage ?? item.usageDetails ?? item.usage_details),
    model: stringValue(item, 'model', 'modelName'),
    promptTokens: numberValue(item, 'promptTokens', 'prompt_tokens'),
    completionTokens: numberValue(item, 'completionTokens', 'completion_tokens'),
    totalTokens: numberValue(item, 'totalTokens', 'total_tokens'),
    level: stringValue(item, 'level', 'status'),
    statusMessage: stringValue(item, 'statusMessage', 'status_message'),
    cost: numberValue(item, 'cost', 'totalCost', 'total_cost'),
  }
}

function normalizeTrace(raw: unknown): Trace {
  const item = asRecord(raw)
  const observations = Array.isArray(item.observations)
    ? item.observations.map(normalizeObservation)
    : undefined
  const started = stringValue(item, 'startTime', 'start_time', 'timestamp')
  const ended = stringValue(item, 'endTime', 'end_time')
  const duration =
    numberValue(item, 'duration', 'latency') ??
    (started && ended ? new Date(ended).getTime() - new Date(started).getTime() : undefined)

  return {
    id: stringValue(item, 'id', 'traceId') ?? crypto.randomUUID(),
    name: stringValue(item, 'name', 'traceName'),
    project: stringValue(item, 'project', 'projectName', 'projectId'),
    userId: stringValue(item, 'userId', 'user_id'),
    sessionId: stringValue(item, 'sessionId', 'session_id'),
    release: stringValue(item, 'release'),
    version: stringValue(item, 'version'),
    timestamp: stringValue(item, 'timestamp', 'startTime', 'start_time'),
    startTime: started,
    endTime: ended,
    duration,
    latency: numberValue(item, 'latency', 'duration') ?? duration,
    status: stringValue(item, 'status', 'level') as TraceStatus | undefined,
    input: item.input,
    output: item.output,
    metadata: asRecord(item.metadata),
    tags: Array.isArray(item.tags)
      ? item.tags.filter((tag): tag is string => typeof tag === 'string')
      : undefined,
    environment: stringValue(item, 'environment', 'env'),
    observations,
    observationCount:
      numberValue(item, 'observationCount', 'observation_count') ?? observations?.length,
    totalCost: numberValue(item, 'totalCost', 'total_cost', 'cost'),
    totalTokens: numberValue(item, 'totalTokens', 'total_tokens'),
  }
}

function normalizePage<T>(
  payload: unknown,
  normalize: (item: unknown) => T,
  fallbackPage: number,
  fallbackLimit: number,
): Paginated<T> {
  const record = asRecord(payload)
  const rawData = Array.isArray(payload)
    ? payload
    : Array.isArray(record.data)
      ? record.data
      : Array.isArray(record.traces)
        ? record.traces
        : []
  return {
    data: rawData.map(normalize),
    total: numberValue(record, 'total', 'count') ?? rawData.length,
    page: numberValue(record, 'page') ?? fallbackPage,
    limit: numberValue(record, 'limit', 'pageSize') ?? fallbackLimit,
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      Accept: 'application/json',
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  if (!response.ok) {
    let detail = ''
    try {
      const body = (await response.json()) as Record<string, unknown>
      detail = stringValue(body, 'error', 'message') ?? ''
    } catch {
      // The server may return an empty or non-JSON error body.
    }
    throw new Error(detail || `Request failed (${response.status})`)
  }
  return response.json() as Promise<T>
}

export async function getHealth() {
  return request<{ status?: string; version?: string }>('/api/health')
}

export async function getTraces(filters: TraceFilters = {}): Promise<Paginated<Trace>> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== '') params.set(key, String(value))
  }
  const query = params.toString()
  const page = filters.page ?? 1
  const limit = filters.limit ?? 25
  const payload = await request<unknown>(`/api/traces${query ? `?${query}` : ''}`)
  return normalizePage(payload, normalizeTrace, page, limit)
}

export async function getTrace(traceId: string): Promise<Trace> {
  const payload = await request<unknown>(`/api/traces/${encodeURIComponent(traceId)}`)
  const record = asRecord(payload)
  const trace = normalizeTrace(record.data ?? payload)
  if (!trace.observations?.length && Array.isArray(record.observations)) {
    trace.observations = record.observations.map(normalizeObservation)
    trace.observationCount = trace.observations.length
  }
  return trace
}

export async function getObservations(filters: TraceFilters = {}): Promise<Paginated<Observation>> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== '') params.set(key, String(value))
  }
  const query = params.toString()
  const page = filters.page ?? 1
  const limit = filters.limit ?? 25
  const payload = await request<unknown>(`/api/observations${query ? `?${query}` : ''}`)
  return normalizePage(payload, normalizeObservation, page, limit)
}

export async function getSessions(): Promise<Paginated<Record<string, unknown>>> {
  return request<Paginated<Record<string, unknown>>>('/api/sessions')
}

export async function getScores(filters: TraceFilters = {}): Promise<Paginated<Score>> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== '') params.set(key, String(value))
  }
  const query = params.toString()
  const payload = await request<unknown>(`/api/scores${query ? `?${query}` : ''}`)
  return normalizePage(payload, normalizeScore, filters.page ?? 1, filters.limit ?? 25)
}

function normalizeScore(raw: unknown): Score {
  const item = asRecord(raw)
  return {
    id: stringValue(item, 'id', 'scoreId') ?? crypto.randomUUID(),
    traceId: stringValue(item, 'traceId', 'trace_id'),
    observationId: stringValue(item, 'observationId', 'observation_id'),
    name: stringValue(item, 'name', 'scoreName'),
    value: numberValue(item, 'value'),
    stringValue: stringValue(item, 'stringValue', 'string_value'),
    dataType: stringValue(item, 'dataType', 'data_type'),
    source: stringValue(item, 'source'),
    comment: stringValue(item, 'comment'),
    timestamp: stringValue(item, 'timestamp', 'createdAt', 'created_at'),
  }
}
