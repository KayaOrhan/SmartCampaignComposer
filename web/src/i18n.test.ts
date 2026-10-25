import { afterEach, expect, it } from 'vitest'
import { ApiError, errorMessage } from './api'
import { channelLabel, date, dateTime, locale, number, setLocale, statusLabel, t, turkish } from './i18n'

afterEach(() => setLocale('en'))
it('defaults to English and has a complete Turkish catalog with matching placeholders', () => {
  expect(locale.value).toBe('en')
  for (const [english, translated] of Object.entries(turkish)) {
    expect(translated.trim(), english).not.toBe('')
    expect([...translated.matchAll(/\{\w+\}/g)].map(m => m[0]).sort(), english)
      .toEqual([...english.matchAll(/\{\w+\}/g)].map(m => m[0]).sort())
  }
  setLocale('tr'); expect(t('Campaigns')).toBe('Kampanyalar')
  expect(statusLabel('content_ready')).toBe('İçerik hazır')
  expect(channelLabel('email')).toBe('E-posta')
  setLocale('unsupported'); expect(locale.value).toBe('en')
})
it('formats dates and numbers explicitly for the selected locale', () => {
  const when = '2026-10-25T12:34:56Z'
  expect(number(12345)).toBe('12,345')
  expect(date(when)).toBe(new Intl.DateTimeFormat('en-US', { dateStyle: 'short' }).format(new Date(when)))
  setLocale('tr')
  expect(number(12345)).toBe('12.345')
  expect(dateTime(when)).toBe(new Intl.DateTimeFormat('tr-TR', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(when)))
  expect(date('')).toBe('—')
})
it('translates errors and never translates interpolated campaign data or template variables', () => {
  const brief = 'Draft {{first_name}} <b>Merhaba</b>'
  setLocale('tr')
  expect(t('Campaign {id}: {brief}', { id: 3, brief })).toBe(`Kampanya 3: ${brief}`)
  expect(t('Allowed variables: {{first_name}}, {{city}}. Saving checks worst-case personalized lengths against the current audience.')).toContain('{{first_name}}, {{city}}')
  expect(errorMessage(new ApiError(422, 'message_too_long', 'sms.body'))).toContain('Kişiselleştirilmiş mesaj kanal sınırını aşıyor')
  expect(errorMessage(new ApiError(412, 'campaign_revision_conflict'))).toContain('yeniden inceleyin')
  expect(errorMessage(new ApiError(502, 'llm_timeout'))).toContain('zaman aşımına')
  expect(errorMessage(new ApiError(422, 'untrusted_diagnostic'))).not.toContain('untrusted_diagnostic')
  expect(errorMessage(new ApiError(422, 'constructor'))).toBe(t('The request could not be completed. Refresh and retry.'))
})
