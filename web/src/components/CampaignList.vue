<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage } from '../api'
import type { Campaign, Channel } from '../types'

const emit = defineEmits<{ open: [campaign: Campaign] }>()
const campaigns = ref<Campaign[]>([]), next = ref<number | null>(null)
const brief = ref(''), channels = ref<Channel[]>(['sms', 'email'])
const loading = ref(false), creating = ref(false), error = ref(''), createError = ref('')
const controller = new AbortController()
async function load(more = false) {
  if (loading.value) return
  loading.value = true; error.value = ''
  try { const page = await api.list(more ? next.value ?? undefined : undefined, AbortSignal.any([controller.signal, AbortSignal.timeout(15000)])); campaigns.value = more ? [...campaigns.value, ...page.campaigns] : page.campaigns; next.value = page.next_before }
  catch (e) { error.value = errorMessage(e) }
  finally { loading.value = false }
}
async function create() {
  if (creating.value || !brief.value.trim() || [...brief.value.trim()].length > 4000 || !channels.value.length) return
  creating.value = true; createError.value = ''
  try { const campaign = await api.create(brief.value, channels.value, AbortSignal.any([controller.signal, AbortSignal.timeout(15000)])); if (!controller.signal.aborted) emit('open', campaign) }
  catch (e) { createError.value = errorMessage(e) + ' Check the campaign list before retrying if the response was lost.' }
  finally { creating.value = false }
}
onMounted(() => load())
onBeforeUnmount(() => controller.abort())
</script>

<template>
  <div class="page-heading"><div><p class="eyebrow">YOUR WORKSPACE</p><h1>Campaigns</h1><p class="muted">Turn an idea into a conversation worth having.</p></div><span class="pill">Mock delivery</span></div>
  <div class="workspace-grid">
    <section class="panel"><div class="section-heading"><h2>Your campaigns</h2><button class="quiet" :disabled="loading" @click="load()">Refresh list</button></div>
      <p v-if="error" role="alert" class="notice error">{{ error }} <button @click="load()">Retry</button></p>
      <p v-if="loading" role="status" class="muted">Loading campaigns...</p>
      <div v-else-if="!campaigns.length && !error" class="empty"><span class="empty-symbol">+</span><h3>Your next campaign starts here</h3><p>Write a brief to create your first campaign.</p></div>
      <ul class="campaign-list"><li v-for="campaign in campaigns" :key="campaign.id"><button @click="emit('open', campaign)"><span class="campaign-top"><span class="campaign-number">CAMPAIGN {{ campaign.id }}</span><span class="status" :class="campaign.status">{{ campaign.status.replaceAll('_', ' ') }}</span></span><strong>{{ campaign.brief }}</strong><span class="campaign-bottom"><span>{{ campaign.channels.join(' + ').toUpperCase() }}</span><span>{{ new Date(campaign.created_at).toLocaleDateString() }} <span aria-hidden="true">&rarr;</span></span></span></button></li></ul>
      <button v-if="next" class="secondary" :disabled="loading" @click="load(true)">Load more</button>
    </section>
    <form class="panel create-panel" @submit.prevent="create"><p class="eyebrow">START SOMETHING NEW</p><h2>Create a campaign</h2><p class="muted">Describe who you want to reach and what you want to say.</p><label for="brief">Campaign brief</label><textarea id="brief" v-model="brief" rows="7" required placeholder="Offer a friendly 30% discount to Istanbul customers who added to their cart in the last 30 days but haven't purchased."/><small>{{ [...brief].length }} / 4,000 characters</small><fieldset><legend>Channels</legend><label class="channel-option"><input v-model="channels" type="checkbox" value="sms"> SMS <span>Short &amp; direct</span></label><label class="channel-option"><input v-model="channels" type="checkbox" value="email"> Email <span>Room to tell more</span></label></fieldset><p class="notice">Demo mode uses a deterministic mock AI. It returns the sample Istanbul audience; it does not interpret arbitrary briefs.</p><p v-if="createError" role="alert" class="notice error">{{ createError }}</p><button class="primary full" :disabled="creating || !brief.trim() || [...brief.trim()].length > 4000 || !channels.length">{{ creating ? 'Creating...' : 'Create campaign' }} <span aria-hidden="true">&rarr;</span></button></form>
  </div>
</template>
