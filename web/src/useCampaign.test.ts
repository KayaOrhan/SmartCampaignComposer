import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
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
