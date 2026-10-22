import { afterEach, expect, it, vi } from 'vitest'
import { generateMessages } from './stream'

const event = (kind: string, data: unknown) => `event: ${kind}\r\ndata: ${JSON.stringify(data)}\r\n\r\n`
const valid = { attempt: 2, base_revision: 7, messages: { sms: { body: 'Hello' } }, audience: { eligible_unique: 3, by_channel: {}, as_of: '' } }
function mockStream(text: string) {
  const bytes = new TextEncoder().encode(text)
  let offset = 0
  const cancel = vi.fn()
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(new ReadableStream<Uint8Array>({
    pull(controller) { if (offset === bytes.length) controller.close(); else controller.enqueue(bytes.slice(offset, ++offset)) }, cancel,
  }), { headers: { 'Content-Type': 'text/event-stream' } })))
  return cancel
}
afterEach(() => vi.unstubAllGlobals())
it('handles byte-split UTF-8 and CRLF while resetting a repaired attempt', async () => {
  mockStream(event('attempt', { attempt: 1 }) + event('delta', { attempt: 1, channel: 'sms', field: 'body', text: 'İstanbul' }) + event('attempt', { attempt: 2 }) + event('delta', { attempt: 2, channel: 'sms', field: 'body', text: 'Hello' }) + event('validated', valid))
  const emit = vi.fn()
  expect(await generateMessages(1, ['sms'], new AbortController().signal, emit)).toEqual(valid)
  expect(emit.mock.calls.map(call => call[0])).toEqual(['attempt', 'delta', 'attempt', 'delta'])
  expect(emit.mock.calls[1]?.[1].text).toBe('İstanbul')
})
it.each([
  event('attempt', { attempt: 1 }) + event('delta', { attempt: 1, channel: 'sms', field: 'body', text: 'Incomplete' }),
  event('attempt', { attempt: 2 }) + event('validated', { ...valid, messages: { email: { body: 'Wrong channel' } } }),
  event('attempt', { attempt: 2 }) + event('validated', valid) + event('error', { error: { code: 'failed' } }),
  event('attempt', { attempt: 2 }) + event('delta', { attempt: 1, channel: 'sms', field: 'body', text: 'stale' }),
  event('error', { error: { code: 'llm_output_invalid' } }),
])('rejects incomplete, malformed, or failed streams', async text => {
  mockStream(text)
  await expect(generateMessages(1, ['sms'], new AbortController().signal, vi.fn())).rejects.toThrow()
})
it('cancels the reader on abort', async () => {
  const cancel = mockStream(event('attempt', { attempt: 1 }))
  const controller = new AbortController(); controller.abort()
  await expect(generateMessages(1, ['sms'], controller.signal, vi.fn())).rejects.toThrow('Cancelled')
  expect(cancel).toHaveBeenCalled()
})
