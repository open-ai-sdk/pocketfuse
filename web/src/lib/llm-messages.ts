// Reads model-call payloads for display: a prompt becomes messages whose parts
// say what they are (text, tool call, tool result), and an output becomes text
// plus tool calls. Anything else is left for the generic JSON view.

export type MessagePart =
  | { kind: 'text'; text: string }
  | { kind: 'tool_call'; name: string; args: unknown }
  | { kind: 'tool_result'; name: string; value: unknown; error?: string }

export type DisplayMessage = { role: string; name?: string; parts: MessagePart[] }

export type Conversation = { messages: DisplayMessage[]; tools: string[] }

const record = (value: unknown): Record<string, unknown> | undefined =>
  value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined

const text = (value: unknown) => (typeof value === 'string' ? value : undefined)

function toolResultValue(content: unknown): unknown {
  if (!Array.isArray(content)) return content
  const parts = content.map((part) => {
    const fields = record(part)
    return fields?.value ?? fields?.text ?? part
  })
  return parts.length === 1 ? parts[0] : parts
}

function parsePart(part: unknown): MessagePart {
  const fields = record(part)
  if (!fields) return { kind: 'text', text: text(part) ?? JSON.stringify(part) }
  const type = text(fields.type)
  if (type === 'toolcall' || type === 'tool_call' || type === 'tool_use') {
    const fn = record(fields.function)
    return {
      kind: 'tool_call',
      name: text(fn?.name) ?? text(fields.name) ?? 'tool',
      args: fn?.arguments ?? fields.args ?? fields.input,
    }
  }
  if (type === 'toolresult' || type === 'tool_result') {
    const value = toolResultValue(fields.content)
    return {
      kind: 'tool_result',
      name: text(fields.name) ?? 'tool',
      value,
      error: text(record(value)?.error),
    }
  }
  return { kind: 'text', text: text(fields.text) ?? text(fields.content) ?? JSON.stringify(part) }
}

function parseContent(content: unknown): MessagePart[] {
  if (content === undefined || content === null || content === '') return []
  if (typeof content === 'string') return [{ kind: 'text', text: content }]
  if (Array.isArray(content)) return content.map(parsePart)
  return [{ kind: 'text', text: JSON.stringify(content, null, 2) }]
}

function parseMessage(value: unknown): DisplayMessage | undefined {
  const fields = record(value)
  const role = text(fields?.role)
  if (!fields || !role) return undefined
  const parts = parseContent(fields.content)
  const calls = fields.tool_calls ?? fields.toolCalls
  if (Array.isArray(calls)) parts.push(...calls.map(parseToolCall))
  return { role, name: text(fields.name), parts }
}

function parseToolCall(value: unknown): MessagePart {
  const fields = record(value)
  const fn = record(fields?.function)
  return {
    kind: 'tool_call',
    name: text(fields?.name) ?? text(fn?.name) ?? 'tool',
    args: fields?.args ?? fn?.arguments ?? fields?.arguments,
  }
}

/** A prompt: a list of messages, or an envelope holding one. */
export function parseConversation(value: unknown): Conversation | undefined {
  const envelope = record(value)
  const list = Array.isArray(value) ? value : envelope?.messages
  if (!Array.isArray(list) || list.length === 0) return undefined
  const messages = list.map(parseMessage)
  if (!messages.every((item): item is DisplayMessage => item !== undefined)) return undefined
  const system = text(envelope?.system)
  if (system) messages.unshift({ role: 'system', parts: [{ kind: 'text', text: system }] })
  const tools = Array.isArray(envelope?.tools)
    ? envelope.tools.flatMap((tool) => {
        const name = text(record(tool)?.name)
        return name ? [name] : []
      })
    : []
  return { messages, tools }
}

/** What a model answered: text and tool calls, when the output has that shape. */
export function parseModelOutput(value: unknown): DisplayMessage | undefined {
  const fields = record(value)
  if (!fields || !('text' in fields || 'tool_calls' in fields || 'choice' in fields)) {
    return undefined
  }
  const parts: MessagePart[] = []
  const answer = text(fields.text)
  if (answer) parts.push({ kind: 'text', text: answer })
  if (Array.isArray(fields.tool_calls)) parts.push(...fields.tool_calls.map(parseToolCall))
  if (Array.isArray(fields.choice)) parts.push(...fields.choice.map(parsePart))
  return parts.length > 0 ? { role: 'assistant', parts } : undefined
}
