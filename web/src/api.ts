import { t, errorKeys } from './i18n'
import type { Campaign, CampaignPage, Delivery, Messages, Preview, SavedMessages } from './types'

export class ApiError extends Error {
  constructor(public status: number, public code: string, public path = '$') { super(code) }
}
export const etag = (id: number, revision: number) => `"campaign-${id}-r${revision}"`
export async function checked(response: Response): Promise<Response> {
  if (response.ok) return response
  let code = 'request_failed', path = '$'
  try {
    const body = await response.json()
    if (typeof body.error?.code === 'string') code = body.error.code
    if (typeof body.error?.path === 'string') path = body.error.path
  } catch { /* Non-JSON proxy errors use the safe fallback. */ }
  throw new ApiError(response.status, code, path)
}
async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await checked(await fetch(`/api${path}`, { ...options, cache: 'no-store', signal: options.signal ?? AbortSignal.timeout(15000) }))
  return response.json() as Promise<T>
}
const json = (body: unknown, revision?: string): RequestInit => ({ method: 'POST', headers: { 'Content-Type': 'application/json', ...(revision ? { 'If-Match': revision } : {}) }, body: JSON.stringify(body) })
export const api = {
  list: (before?: number, signal?: AbortSignal) => request<CampaignPage>(`/campaigns${before ? `?before=${before}` : ''}`, { signal }),
  create: (brief: string, channels: string[], signal?: AbortSignal) => request<Campaign>('/campaigns', { ...json({ brief, channels }), signal }),
  get: (id: number, signal?: AbortSignal) => request<Campaign>(`/campaigns/${id}`, { signal }),
  messages: (id: number, signal?: AbortSignal) => request<SavedMessages>(`/campaigns/${id}/messages`, { signal }),
  generateSegment: (c: Campaign, signal?: AbortSignal) => request<Campaign>(`/campaigns/${c.id}/generate-segment`, { method: 'POST', headers: { 'If-Match': etag(c.id, c.revision) }, signal: signal ?? AbortSignal.timeout(35000) }),
  preview: (id: number, signal?: AbortSignal) => request<Preview>(`/campaigns/${id}/audience-preview`, { method: 'POST', signal }),
  save: (id: number, revision: number, messages: Messages, signal?: AbortSignal) => request<SavedMessages>(`/campaigns/${id}/messages`, { ...json({ messages }, etag(id, revision)), method: 'PUT', signal }),
  approve: (id: number, preview: Preview, signal?: AbortSignal) => request<Delivery>(`/campaigns/${id}/approve`, { ...json({ approval_fingerprint: preview.approval_fingerprint }, etag(id, preview.revision)), signal }),
  remove: async (c: Campaign, signal?: AbortSignal): Promise<void> => {
    await checked(await fetch(`/api/campaigns/${c.id}`, { method: 'DELETE', headers: { 'If-Match': etag(c.id, c.revision) }, cache: 'no-store', signal: signal ?? AbortSignal.timeout(15000) }))
  },
  delivery: (id: number, signal?: AbortSignal) => request<Delivery>(`/campaigns/${id}/delivery-status`, { signal }),
}
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 409 || error.status === 412) return t('The campaign or audience changed. Refresh, reconcile your changes, and review again before saving or approving.')
    if (error.status === 503) return t('The service is temporarily unavailable. Please retry.')
    if (error.code === 'llm_timeout') return t('The AI request timed out. Please retry.')
    if (error.code === 'llm_output_invalid') return t('The AI response did not pass validation. Please retry.')
    if (error.code === 'llm_refused') return t('The AI declined this request. Edit the brief and retry.')
    if (['llm_unavailable', 'llm_audit_unavailable', 'generation_failed'].includes(error.code)) return t('AI generation failed. Please retry.')
    const key = Object.hasOwn(errorKeys, error.code) ? errorKeys[error.code] : undefined
    return key ? t('Request failed: {message} ({path}).', { message: t(key), path: error.path }) : t('The request could not be completed. Refresh and retry.')
  }
  if (error instanceof Error && error.name === 'AbortError') return t('Operation cancelled. Refresh to check whether a save completed.')
  return t('Unable to reach the service. Check the connection and retry.')
}
