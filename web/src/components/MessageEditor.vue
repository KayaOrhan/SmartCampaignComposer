<script setup lang="ts">
import { t, number } from '../i18n'
import type { Channel, Messages } from '../types'
import { smsEstimate } from '../sms'
const props = defineProps<{ messages: Messages; channels: Channel[]; disabled: boolean }>()
const variablesHelp = () => t('Allowed variables: {{first_name}}, {{city}}. Saving checks worst-case personalized lengths against the current audience.')
const emit = defineEmits<{ change: [messages: Messages] }>()
function update(channel: Channel, field: 'body' | 'subject', event: Event) {
  const next: Messages = JSON.parse(JSON.stringify(props.messages)) as Messages
  const text = (event.target as HTMLTextAreaElement).value
  if (channel === 'sms') next.sms = { body: text }
  else { next.email ??= { body: '', subject: '' }; next.email[field] = text }
  emit('change', next)
}
</script>
<template>
  <div class="split editor">
    <div v-if="channels.includes('sms')"><label for="sms-body">{{ t(`SMS message`) }}</label><textarea id="sms-body" :value="messages.sms?.body ?? ''" :disabled="disabled" rows="6" @input="update('sms', 'body', $event)"/><small>{{ t(`{count} template characters. Maximum {limit} after personalization.`, { count: number([...(messages.sms?.body ?? '')].length), limit: number(160) }) }}</small></div>
    <div v-if="channels.includes('email')"><label for="email-subject">{{ t(`Email subject`) }}</label><textarea id="email-subject" :value="messages.email?.subject ?? ''" :disabled="disabled" rows="2" @input="update('email', 'subject', $event)"/><small>{{ t(`Maximum {limit} characters after personalization.`, { limit: number(80) }) }}</small><label for="email-body">{{ t(`Email body`) }}</label><textarea id="email-body" :value="messages.email?.body ?? ''" :disabled="disabled" rows="5" @input="update('email', 'body', $event)"/><small>{{ t(`Maximum {limit} characters after personalization.`, { limit: number(2000) }) }}</small></div>
  </div>
  <p class="muted"><small>{{ variablesHelp() }}</small></p>
  <p v-if="messages.sms" class="notice" :class="{ warning: smsEstimate(messages.sms.body).encoding === 'Unicode' }">{{ t(`Template estimate: {segments} SMS segment(s), {encoding} encoding. Personalization can change this estimate. Unicode uses 70 units per single segment; the 160-character content limit still applies.`, { segments: number(smsEstimate(messages.sms.body).segments), encoding: smsEstimate(messages.sms.body).encoding }) }}</p>
</template>
