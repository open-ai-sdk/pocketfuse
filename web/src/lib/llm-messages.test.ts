import { describe, expect, it } from 'vite-plus/test'
import { parseConversation, parseModelOutput } from './llm-messages'

describe('parseConversation', () => {
  it('names the tools and separates calls from results', () => {
    const conversation = parseConversation({
      messages: [
        { role: 'system', content: 'Be brief' },
        { role: 'user', content: [{ type: 'text', text: 'Draft a listing' }] },
        {
          role: 'assistant',
          content: [{ type: 'toolcall', function: { name: 'spawn_jobs', arguments: { a: 1 } } }],
        },
        {
          role: 'user',
          content: [
            {
              type: 'toolresult',
              name: 'spawn_jobs',
              content: [{ type: 'json', value: { error: 'budget exceeded' } }],
            },
          ],
        },
      ],
      tools: [{ name: 'spawn_jobs' }, { name: 'get_job' }],
    })
    expect(conversation?.tools).toEqual(['spawn_jobs', 'get_job'])
    expect(conversation?.messages[1].parts).toEqual([{ kind: 'text', text: 'Draft a listing' }])
    expect(conversation?.messages[2].parts[0]).toEqual({
      kind: 'tool_call',
      name: 'spawn_jobs',
      args: { a: 1 },
    })
    expect(conversation?.messages[3].parts[0]).toMatchObject({
      kind: 'tool_result',
      name: 'spawn_jobs',
      error: 'budget exceeded',
    })
  })

  it('moves a top-level system prompt to the front', () => {
    const conversation = parseConversation({
      system: 'Write copy',
      messages: [{ role: 'user', content: 'x' }],
    })
    expect(conversation?.messages.map((message) => message.role)).toEqual(['system', 'user'])
  })

  it('declines anything that is not a list of messages', () => {
    expect(parseConversation({ provider: 'x', questions: {} })).toBeUndefined()
    expect(parseConversation([{ not: 'a message' }])).toBeUndefined()
    expect(parseConversation('text')).toBeUndefined()
    expect(parseConversation({ messages: [] })).toBeUndefined()
  })
})

describe('parseModelOutput', () => {
  it('reads text and tool calls', () => {
    const message = parseModelOutput({
      text: 'Started',
      tool_calls: [{ id: 'c', name: 'wait_for_jobs', args: { mode: 'all' } }],
      final: {},
    })
    expect(message?.parts).toEqual([
      { kind: 'text', text: 'Started' },
      { kind: 'tool_call', name: 'wait_for_jobs', args: { mode: 'all' } },
    ])
  })

  it('declines other shapes and empty answers', () => {
    expect(parseModelOutput({ status: 'success', answers: {} })).toBeUndefined()
    expect(parseModelOutput({ text: '', tool_calls: [] })).toBeUndefined()
  })
})
