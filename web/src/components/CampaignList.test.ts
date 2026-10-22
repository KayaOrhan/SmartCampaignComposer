import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import CampaignList from './CampaignList.vue'
import { api } from '../api'
afterEach(() => vi.restoreAllMocks())
it('shows an empty list and sends the selected channels with the brief', async () => {
  vi.spyOn(api, 'list').mockResolvedValue({ campaigns: [], next_before: null })
  const create = vi.spyOn(api, 'create').mockResolvedValue({ id: 1, brief: 'A friendly offer', channels: ['sms'], status: 'draft', revision: 1, segment: null, created_at: '' })
  const wrapper = mount(CampaignList)
  await flushPromises()
  expect(wrapper.text()).toContain('Your next campaign starts here')
  await wrapper.get('textarea').setValue('A friendly offer')
  await wrapper.get('input[value=email]').setValue(false)
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(create).toHaveBeenCalledWith('A friendly offer', ['sms'], expect.any(AbortSignal))
  expect(wrapper.emitted('open')).toHaveLength(1)
})
it('offers retry when listing fails', async () => {
  vi.spyOn(api, 'list').mockRejectedValue(new Error('offline'))
  const wrapper = mount(CampaignList)
  await flushPromises()
  expect(wrapper.get('[role=alert]').text()).toContain('retry')
})
