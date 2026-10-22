import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import DeliveryDashboard from './DeliveryDashboard.vue'
import { api } from '../api'
import type { Delivery } from '../types'
const delivery: Delivery = { campaign_id: 1, revision: 4, status: 'sending', approved_at: '', approved_revision: 3, counts: { total: 3, pending: 2, in_flight: 1, sent: 0, dead: 0, skipped: 0 }, by_channel: {} }
afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })
it('polls until completion and reports dead/skipped separately', async () => {
  vi.useFakeTimers()
  const get = vi.spyOn(api, 'delivery').mockResolvedValueOnce(delivery).mockResolvedValue({ ...delivery, status: 'completed', counts: { total: 3, pending: 0, in_flight: 0, sent: 1, dead: 1, skipped: 1 } })
  const wrapper = mount(DeliveryDashboard, { props: { id: 1 } }); await flushPromises()
  expect(wrapper.text()).toContain('Delivery in progress')
  await vi.advanceTimersByTimeAsync(2000); await flushPromises()
  expect(wrapper.text()).toContain('Campaign completed')
  expect(wrapper.get('progress').attributes('value')).toBe('3')
  expect(wrapper.text()).toContain('Dead'); expect(wrapper.text()).toContain('Skipped')
  await vi.advanceTimersByTimeAsync(10000)
  expect(get).toHaveBeenCalledTimes(2)
  wrapper.unmount()
})
it('shows errors, retries, and cleans up polling on unmount', async () => {
  vi.useFakeTimers()
  const get = vi.spyOn(api, 'delivery').mockRejectedValueOnce(new Error('offline')).mockResolvedValue(delivery)
  const wrapper = mount(DeliveryDashboard, { props: { id: 1 } }); await flushPromises()
  expect(wrapper.get('[role=alert]').text()).toContain('stale')
  await vi.advanceTimersByTimeAsync(5000); await flushPromises()
  expect(wrapper.find('[role=alert]').exists()).toBe(false)
  wrapper.unmount(); await vi.advanceTimersByTimeAsync(10000)
  expect(get).toHaveBeenCalledTimes(2)
})
