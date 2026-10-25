<script setup lang="ts">
import { t, number, date, statusLabel, channelLabel } from '../i18n'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage } from '../api'
import type { Campaign, Channel } from '../types'

const emit = defineEmits<{ open: [campaign: Campaign] }>()
const campaigns = ref<Campaign[]>([]), next = ref<number | null>(null)
const briefInput = ref<HTMLTextAreaElement | null>(null)
const brief = ref(''), channels = ref<Channel[]>(['sms', 'email'])
const loading = ref(false), creating = ref(false), error = ref<unknown>(null), createError = ref<unknown>(null)
const controller = new AbortController()
async function load(more = false) {
  if (loading.value) return
  loading.value = true; error.value = null
  try { const page = await api.list(more ? next.value ?? undefined : undefined, AbortSignal.any([controller.signal, AbortSignal.timeout(15000)])); campaigns.value = more ? [...campaigns.value, ...page.campaigns] : page.campaigns; next.value = page.next_before }
  catch (e) { error.value = e }
  finally { loading.value = false }
}
async function create() {
  if (creating.value || !brief.value.trim() || [...brief.value.trim()].length > 4000 || !channels.value.length) return
  creating.value = true; createError.value = null
  try { const campaign = await api.create(brief.value, channels.value, AbortSignal.any([controller.signal, AbortSignal.timeout(15000)])); if (!controller.signal.aborted) emit('open', campaign) }
  catch (e) { createError.value = e }
  finally { creating.value = false }
}
onMounted(() => load())
onBeforeUnmount(() => controller.abort())
</script>

<template>
  <div class="page-heading"><div><p class="eyebrow">{{ t(`YOUR WORKSPACE`) }}</p><h1>{{ t(`Campaigns`) }}</h1><p class="muted">{{ t(`Turn an idea into a conversation worth having.`) }}</p></div><span class="pill">{{ t(`Mock delivery`) }}</span></div>
  <div class="workspace-grid">
    <section class="panel"><div class="section-heading"><h2>{{ t(`Your campaigns`) }}</h2><button class="quiet" :disabled="loading" @click="load()">{{ t(`Refresh list`) }}</button></div>
      <p v-if="error" role="alert" class="notice error">{{ errorMessage(error) }} <button @click="load()">{{ t(`Retry`) }}</button></p>
      <p v-if="loading" role="status" class="muted">{{ t(`Loading campaigns...`) }}</p>
      <div v-else-if="!campaigns.length && !error" class="empty"><button type="button" class="empty-symbol" :aria-label="t(`Start a new campaign`)" @click="briefInput?.focus()">+</button><h3>{{ t(`Your next campaign starts here`) }}</h3><p>{{ t(`Write a brief to create your first campaign.`) }}</p></div>
      <ul class="campaign-list"><li v-for="campaign in campaigns" :key="campaign.id"><button @click="emit('open', campaign)"><span class="campaign-top"><span class="campaign-number">{{ t(`CAMPAIGN {id}`, { id: number(campaign.id) }) }}</span><span class="status" :class="campaign.status">{{ statusLabel(campaign.status) }}</span></span><strong>{{ campaign.brief }}</strong><span class="campaign-bottom"><span>{{ campaign.channels.map(channelLabel).join(' + ') }}</span><span>{{ date(campaign.created_at) }} <span aria-hidden="true">&rarr;</span></span></span></button></li></ul>
      <button v-if="next" class="secondary" :disabled="loading" @click="load(true)">{{ t(`Load more`) }}</button>
    </section>
    <form novalidate class="panel create-panel" @submit.prevent="create"><p class="eyebrow">{{ t(`START SOMETHING NEW`) }}</p><h2>{{ t(`Create a campaign`) }}</h2><p class="muted">{{ t(`Describe who you want to reach and what you want to say.`) }}</p><label for="brief">{{ t(`Campaign brief`) }}</label><textarea ref="briefInput" id="brief" v-model="brief" rows="7" required :placeholder="t(`Offer a friendly 30% discount to Istanbul customers who added to their cart in the last 30 days but haven't purchased.`)"/><small>{{ t(`{count} / {limit} characters`, { count: number([...brief].length), limit: number(4000) }) }}</small><p v-if="[...brief.trim()].length > 4000 || !channels.length" role="alert" class="notice error">{{ t(!channels.length ? `Select at least one channel.` : `Campaign brief must not exceed 4,000 characters.`) }}</p><fieldset><legend>{{ t(`Channels`) }}</legend><label class="channel-option"><input v-model="channels" type="checkbox" value="sms">{{ t(`SMS`) }}<span>{{ t(`Short & direct`) }}</span></label><label class="channel-option"><input v-model="channels" type="checkbox" value="email">{{ t(`Email`) }}<span>{{ t(`Room to tell more`) }}</span></label></fieldset><p class="notice">{{ t(`Demo mode uses a deterministic mock AI. It returns the sample Istanbul audience; it does not interpret arbitrary briefs.`) }}</p><p v-if="createError" role="alert" class="notice error">{{ errorMessage(createError) }} {{ t(`Check the campaign list before retrying if the response was lost.`) }}</p><button class="primary full" :disabled="creating || !brief.trim() || [...brief.trim()].length > 4000 || !channels.length">{{ t(creating ? 'Creating...' : 'Create campaign') }} <span aria-hidden="true">&rarr;</span></button></form>
  </div>
</template>
