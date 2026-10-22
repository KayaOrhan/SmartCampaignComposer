import { ApiError, checked } from './api'
import type { Candidate, Channel, Delta } from './types'

// POST SSE needs fetch rather than EventSource. A closed stream without a
// terminal validated event is a failure, never a partially successful result.
export async function generateMessages(id: number, channels: Channel[], signal: AbortSignal, emit: (kind: 'attempt' | 'delta', event: Delta) => void): Promise<Candidate> {
  const response = await checked(await fetch(`/api/campaigns/${id}/generate-messages`, { method: 'POST', signal, cache: 'no-store' }))
  if (!response.headers.get('Content-Type')?.startsWith('text/event-stream') || !response.body) throw new Error('Invalid stream')
  const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
  let buffer = '', total = 0, attempt = 0, result: Candidate | null = null
  function event(frame: string) {
    const lines = frame.split(/\r?\n/)
    const kind = lines.find(line => line.startsWith('event:'))?.slice(6).trim()
    const data = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
    if (!kind && !data) return
    if (result) throw new Error('Event after terminal validation')
    const value = JSON.parse(data)
    if (kind === 'attempt') {
      if (!Number.isSafeInteger(value.attempt) || value.attempt <= attempt) throw new Error('Invalid attempt')
      attempt = value.attempt
      emit('attempt', value)
    } else if (kind === 'delta') {
      if (!attempt || value.attempt !== attempt || !channels.includes(value.channel) || !['body', 'subject'].includes(value.field) || (value.channel === 'sms' && value.field !== 'body') || typeof value.text !== 'string') throw new Error('Invalid delta')
      emit('delta', value)
    } else if (kind === 'validated') {
      if (!attempt || value.attempt !== attempt || !Number.isSafeInteger(value.base_revision) || value.base_revision < 1 || !value.messages || !value.audience || !Number.isSafeInteger(value.audience.eligible_unique)) throw new Error('Invalid result')
      if (Object.keys(value.messages).length !== channels.length) throw new Error('Invalid channels')
      for (const channel of channels) {
        if (typeof value.messages[channel]?.body !== 'string' || (channel === 'email' && typeof value.messages.email.subject !== 'string')) throw new Error('Invalid message')
      }
      result = value as Candidate
    } else if (kind === 'error') {
      throw new ApiError(502, typeof value.error?.code === 'string' ? value.error.code : 'generation_failed', value.error?.path)
    } else throw new Error('Unknown stream event')
  }
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (signal.aborted) throw new DOMException('Cancelled', 'AbortError')
      if (value) total += value.byteLength
      if (total > 524288) throw new Error('Stream exceeds limit')
      buffer += decoder.decode(value, { stream: !done })
      let match: RegExpExecArray | null
      while ((match = /\r?\n\r?\n/.exec(buffer))) {
        if (match.index > 65536) throw new Error('Event exceeds limit')
        event(buffer.slice(0, match.index)); buffer = buffer.slice(match.index + match[0].length)
      }
      if (buffer.length > 65536) throw new Error('Event exceeds limit')
      if (done) break
    }
    if (!result || buffer.trim()) throw new Error('Incomplete stream')
    return result
  } finally {
    try { await reader.cancel() } finally { reader.releaseLock() }
  }
}
