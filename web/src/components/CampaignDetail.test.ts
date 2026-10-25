import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import CampaignDetail from './CampaignDetail.vue'
import { api } from '../api'
import type { Campaign } from '../types'

const campaign: Campaign = { id: 3, revision: 5, status: 'content_ready', channels: ['sms', 'email'], brief: 'Synthetic offer', segment: { version: 1, all: [{ field: 'city', op: 'in', value: ['istanbul'] }] }, created_at: '' }
beforeEach(() => {
  vi.spyOn(HTMLDialogElement.prototype, 'showModal').mockImplementation(function(this: HTMLDialogElement) { this.open = true })
  vi.spyOn(HTMLDialogElement.prototype, 'close').mockImplementation(function(this: HTMLDialogElement) { this.open = false })
  vi.spyOn(api, 'get').mockResolvedValue(campaign)
  vi.spyOn(api, 'messages').mockResolvedValue({ campaign_id: 3, revision: 5, status: 'content_ready', messages: { sms: { body: 'Saved SMS' }, email: { subject: 'Saved subject', body: 'Saved email' } }, validation: { revision: 5, audience: { eligible_unique: 2, by_channel: {}, as_of: '' } } })
  vi.spyOn(api, 'preview').mockResolvedValue({ revision: 5, eligible_unique: 2, by_channel: { sms: { count: 1, max_first_name_runes: 3, max_city_runes: 8 }, email: { count: 2, max_first_name_runes: 3, max_city_runes: 8 } }, as_of: '', approval_fingerprint: 'a'.repeat(64) })
})
afterEach(() => { vi.restoreAllMocks(); document.body.innerHTML = '' })
const button = (wrapper: ReturnType<typeof mount>, name: string) => wrapper.findAll('button').find(b => b.text() === name)!
it('shows saved templates and current counts, cancels without approval, and confirms explicitly', async () => {
  const approve = vi.spyOn(api, 'approve').mockRejectedValue(new Error('offline'))
  const wrapper = mount(CampaignDetail, { props: { initial: campaign }, attachTo: document.body })
  await flushPromises()
  await button(wrapper, 'Review & approve').trigger('click'); await flushPromises()
  const dialog = wrapper.get('dialog')
  expect(dialog.text()).toContain('Saved SMS')
  expect(dialog.text()).toContain('Saved subject')
  expect(dialog.text()).toContain('Saved email')
  expect(dialog.text()).toContain('2Unique recipients')
  expect(dialog.text()).toContain('SMS eligible')
  expect(dialog.findAll('input[type=checkbox]')).toHaveLength(0)
  expect(approve).not.toHaveBeenCalled()
  await button(wrapper, 'Cancel').trigger('click')
  expect(wrapper.find('dialog').exists()).toBe(false)
  await button(wrapper, 'Review & approve').trigger('click'); await flushPromises()
  expect(api.preview).toHaveBeenCalledTimes(2)
  await button(wrapper, 'Confirm approval & start mock delivery').trigger('click'); await flushPromises()
  expect(approve).toHaveBeenCalledTimes(1)
  wrapper.unmount()
})
it('deletes only after confirmation and returns to the list after success', async () => {
  const remove = vi.spyOn(api, 'remove').mockResolvedValue()
  const wrapper = mount(CampaignDetail, { props: { initial: campaign } })
  await flushPromises()
  await button(wrapper, 'Delete draft').trigger('click')
  expect(remove).not.toHaveBeenCalled()
  await button(wrapper, 'Cancel').trigger('click')
  expect(remove).not.toHaveBeenCalled()
  await button(wrapper, 'Delete draft').trigger('click')
  await button(wrapper, 'Confirm deletion').trigger('click'); await flushPromises()
  expect(remove).toHaveBeenCalledWith(campaign, expect.any(AbortSignal))
  expect(wrapper.emitted('back')).toHaveLength(1)
  wrapper.unmount()
})
