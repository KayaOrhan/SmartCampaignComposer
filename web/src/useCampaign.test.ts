import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { setLocale } from './i18n'
import { api, ApiError } from './api'
import { useCampaign } from './useCampaign'
import type { Campaign, Preview, SavedMessages } from './types'

const campaign: Campaign = { id: 3, revision: 5, status: 'content_ready', channels: ['sms'], brief: 'Offer', segment: { version: 1, all: [{ field: 'city', op: 'in', value: ['istanbul'] }] }, created_at: '' }
const saved: SavedMessages = { campaign_id: 3, revision: 5, status: 'content_ready', messages: { sms: { body: 'Hello' } }, validation: { revision: 5, audience: { eligible_unique: 1, by_channel: {}, as_of: '' } } }
const preview: Preview = { revision: 5, eligible_unique: 1, approval_fingerprint: 'a'.repeat(64), by_channel: {}, as_of: '' }
function setup() {
  vi.spyOn(api, 'get').mockResolvedValue(campaign)
  vi.spyOn(api, 'messages').mockResolvedValue(saved)
  vi.spyOn(api, 'preview').mockResolvedValue(preview)
  let state!: ReturnType<typeof useCampaign>
  const wrapper = mount(defineComponent({ setup() { state = useCampaign(campaign); return () => null } }))
  return { state, wrapper }
}
afterEach(() => vi.restoreAllMocks())
it('requires saved content, fresh audience and explicit review, invalidated by editing', async () => {
  const { state, wrapper } = setup()
  await state.refresh()
  expect(state.canApprove.value).toBe(false)
  await state.preview()
  expect(state.canApprove.value).toBe(false)
  state.review.value = true
  expect(state.canApprove.value).toBe(true)
  state.edit({ sms: { body: 'Edited' } })
  expect(state.review.value).toBe(false)
  expect(state.audience.value).toBeNull()
  expect(state.canApprove.value).toBe(false)
  wrapper.unmount()
})
it('preserves stale edits but never retries them under a refreshed revision', async () => {
  const { state, wrapper } = setup()
  await state.refresh()
  state.edit({ sms: { body: 'My unsaved draft' } })
  const save = vi.spyOn(api, 'save').mockRejectedValue(new ApiError(412, 'campaign_revision_conflict'))
  vi.mocked(api.get).mockResolvedValue({ ...campaign, revision: 6 })
  vi.mocked(api.messages).mockResolvedValue({ ...saved, revision: 6 })
  await state.save()
  expect(save).toHaveBeenCalledTimes(1)
  expect(save.mock.calls[0]?.[1]).toBe(5)
  expect(state.messages.value.sms?.body).toBe('My unsaved draft')
  expect(state.campaign.value.revision).toBe(6)
  expect(state.canSave.value).toBe(false)
  expect(state.canApprove.value).toBe(false)
  await state.save()
  expect(save).toHaveBeenCalledTimes(1)
  wrapper.unmount()
})
it('requires renewed review after an approval fingerprint conflict', async () => {
  const { state, wrapper } = setup()
  await state.refresh(); await state.preview(); state.review.value = true
  const approve = vi.spyOn(api, 'approve').mockRejectedValue(new ApiError(409, 'approval_preview_changed'))
  await state.approve()
  expect(approve).toHaveBeenCalledTimes(1)
  expect(state.review.value).toBe(false)
  expect(state.audience.value).toBeNull()
  expect(state.canApprove.value).toBe(false)
  wrapper.unmount()
})
it('rejects incoherent campaign/template reads and empty audiences', async () => {
  const { state, wrapper } = setup()
  vi.mocked(api.messages).mockResolvedValue({ ...saved, revision: 6 })
  await state.refresh()
  expect(state.stale.value).toBe(true)
  expect(state.canSave.value).toBe(false)
  vi.mocked(api.messages).mockResolvedValue(saved)
  await state.refresh()
  vi.mocked(api.preview).mockResolvedValue({ ...preview, eligible_unique: 0, approval_fingerprint: null })
  await state.preview(); state.review.value = true
  expect(state.canApprove.value).toBe(false)
  wrapper.unmount()
})
it('cancels an active request on unmount', async () => {
  const { state, wrapper } = setup()
  let signal: AbortSignal | undefined
  vi.mocked(api.get).mockImplementation((_id, s) => { signal = s; return new Promise(() => {}) })
  void state.refresh(); await flushPromises(); wrapper.unmount()
  expect(signal?.aborted).toBe(true)
})

it('opens a fresh review without approving, and requires confirmation each time', async () => {
  const { state, wrapper } = setup()
  const approve = vi.spyOn(api, 'approve').mockResolvedValue({ campaign_id: 3, revision: 6, status: 'sending', counts: { total: 1, pending: 1, in_flight: 0, sent: 0, dead: 0, skipped: 0 }, by_channel: {}, approved_at: '', approved_revision: 5 })
  await state.refresh(); await state.openReview()
  expect(state.review.value).toBe(true)
  expect(state.canApprove.value).toBe(true)
  expect(approve).not.toHaveBeenCalled()
  state.review.value = false
  await state.approve()
  expect(approve).not.toHaveBeenCalled()
  await state.refresh(); await state.openReview()
  expect(api.preview).toHaveBeenCalledTimes(2)
  expect(approve).not.toHaveBeenCalled()
  await state.approve()
  expect(approve).toHaveBeenCalledWith(3, preview, expect.any(AbortSignal))
  expect(approve).toHaveBeenCalledTimes(1)
  wrapper.unmount()
})
it('does not delete a stale or immutable campaign and never silently retries deletion', async () => {
  const { state, wrapper } = setup()
  const remove = vi.spyOn(api, 'remove').mockRejectedValue(new ApiError(412, 'campaign_revision_conflict'))
  await state.refresh(); await state.remove()
  expect(state.deleted.value).toBe(false)
  expect(state.stale.value).toBe(true)
  await state.remove()
  expect(remove).toHaveBeenCalledTimes(1)
  await state.refresh()
  state.campaign.value = { ...campaign, status: 'sending' }
  await state.remove()
  expect(remove).toHaveBeenCalledTimes(1)
  wrapper.unmount()
})

it('updates an existing error on language switch without modifying authored templates', async () => {
  const { state, wrapper } = setup()
  await state.refresh()
  const authored = { sms: { body: 'Draft {{first_name}} — Aynen koru' } }
  state.edit(authored)
  vi.spyOn(api, 'save').mockRejectedValue(new ApiError(422, 'message_too_long', 'sms.body'))
  await state.save()
  expect(state.error.value).toContain('Personalized message exceeds')
  try {
    setLocale('tr')
    expect(state.error.value).toContain('Kişiselleştirilmiş mesaj')
    expect(state.messages.value).toEqual(authored)
    expect(state.campaign.value.brief).toBe(campaign.brief)
  } finally { setLocale('en'); wrapper.unmount() }
})
