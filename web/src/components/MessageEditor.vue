<script setup lang="ts">
import type { Channel, Messages } from '../types'
import { smsEstimate } from '../sms'
const props = defineProps<{ messages: Messages; channels: Channel[]; disabled: boolean }>()
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
    <div v-if="channels.includes('sms')"><label for="sms-body">SMS message</label><textarea id="sms-body" :value="messages.sms?.body ?? ''" :disabled="disabled" rows="6" @input="update('sms', 'body', $event)"/><small>{{ [...(messages.sms?.body ?? '')].length }} template characters. Maximum 160 after personalization.</small></div>
    <div v-if="channels.includes('email')"><label for="email-subject">Email subject</label><textarea id="email-subject" :value="messages.email?.subject ?? ''" :disabled="disabled" rows="2" @input="update('email', 'subject', $event)"/><small>Maximum 80 characters after personalization.</small><label for="email-body">Email body</label><textarea id="email-body" :value="messages.email?.body ?? ''" :disabled="disabled" rows="5" @input="update('email', 'body', $event)"/><small>Maximum 2,000 characters after personalization.</small></div>
  </div>
  <p class="muted"><small v-pre>Allowed variables: {{first_name}}, {{city}}. Saving checks worst-case personalized lengths against the current audience.</small></p>
  <p v-if="messages.sms" class="notice" :class="{ warning: smsEstimate(messages.sms.body).encoding === 'Unicode' }">Template estimate: {{ smsEstimate(messages.sms.body).segments }} SMS segment(s), {{ smsEstimate(messages.sms.body).encoding }} encoding. Personalization can change this estimate. Unicode uses 70 units per single segment; the 160-character content limit still applies.</p>
</template>
