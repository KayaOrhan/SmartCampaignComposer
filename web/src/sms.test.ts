import { expect, it } from 'vitest'
import { smsEstimate } from './sms'
it('accounts for GSM extensions, Unicode and multipart overhead', () => {
  expect(smsEstimate('£'.repeat(160))).toEqual({ encoding: 'GSM-7', segments: 1 })
  expect(smsEstimate('^'.repeat(81)).segments).toBe(2)
  expect(smsEstimate('ş'.repeat(71))).toEqual({ encoding: 'Unicode', segments: 2 })
  expect(smsEstimate('🙂'.repeat(36)).segments).toBe(2)
})
