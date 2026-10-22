<script setup lang="ts">
import { onMounted } from 'vue'
import type { Campaign } from '../types'
import { useCampaign } from '../useCampaign'
import AudienceSummary from './AudienceSummary.vue'
import MessageEditor from './MessageEditor.vue'
import DeliveryDashboard from './DeliveryDashboard.vue'
const props = defineProps<{ initial: Campaign }>()
defineEmits<{ back: [] }>()
const { campaign, saved, messages, audience, review, error, notice, busy, stale, dirty, editable, canSave, canApprove, generated, streamPreview, attempt, refresh, edit, segment, preview, generate, save, approve, cancel } = useCampaign(props.initial)
onMounted(refresh)
</script>
<template>
  <button class="quiet back" @click="$emit('back')">&larr; All campaigns</button>
  <div class="page-heading"><div><p class="eyebrow">CAMPAIGN {{ campaign.id }} / REVISION {{ campaign.revision }}</p><h1>Make it meaningful.</h1><p class="muted detail-brief">{{ campaign.brief }}</p></div><span class="status" :class="campaign.status">{{ campaign.status.replaceAll('_', ' ') }}</span></div>
  <div v-if="error" role="alert" class="notice error">{{ error }}<div class="actions"><button :disabled="!!busy" @click="refresh">{{ stale || dirty ? 'Discard local changes and reload' : 'Refresh campaign' }}</button></div></div>
  <p v-if="notice" role="status" class="notice">{{ notice }}</p>
  <div v-if="busy" role="status" class="notice">{{ busy }}... <button v-if="busy === 'Generating messages'" class="quiet" @click="cancel">Cancel generation</button></div>
  <section class="panel"><div class="section-heading"><h2><span class="step">1</span>Audience</h2><button v-if="editable" class="secondary" :disabled="!!busy || stale" @click="segment">{{ campaign.segment ? 'Regenerate segment' : 'Generate segment' }}</button></div><AudienceSummary :segment="campaign.segment" :audience="audience"/><button v-if="campaign.segment && editable" :disabled="!!busy || stale" @click="preview">Preview audience</button></section>
  <section class="panel"><div class="section-heading"><h2><span class="step">2</span>Messages</h2><button v-if="editable" class="secondary" :disabled="!!busy || !campaign.segment || stale" @click="generate">Generate messages</button></div>
    <template v-if="busy === 'Generating messages'"><p class="stream-label">Streaming preview &middot; attempt {{ attempt }} &middot; not validated or saved</p><div class="split"><div v-if="campaign.channels.includes('sms')"><h3>SMS preview</h3><p class="preview-copy">{{ streamPreview.sms?.body || 'Waiting for content...' }}</p></div><div v-if="campaign.channels.includes('email')"><h3>Email preview</h3><strong>{{ streamPreview.email?.subject }}</strong><p class="preview-copy">{{ streamPreview.email?.body || 'Waiting for content...' }}</p></div></div></template>
    <p v-if="generated" class="notice">Validated suggestion &middot; not saved. Your edits will be validated again when saved.</p>
    <p v-else-if="dirty" class="notice warning">Unsaved changes. Save and review again before approval.</p>
    <p v-else-if="saved?.validation" class="notice">Saved and validated at revision {{ saved.validation.revision }}.</p>
    <p v-else class="notice">No currently validated saved messages. Generate content or revalidate retained text.</p>
    <MessageEditor :messages="messages" :channels="campaign.channels" :disabled="!!busy || !editable || stale || !campaign.segment" @change="edit"/>
    <div v-if="editable" class="actions"><button class="primary" :disabled="!canSave" @click="save">Save messages</button><button v-if="dirty || stale || generated" :disabled="!!busy" @click="refresh">Discard local changes and reload</button></div>
  </section>
  <section v-if="editable" class="panel"><div class="section-heading"><h2><span class="step">3</span>Review &amp; approve</h2><button :disabled="!!busy || stale || dirty || generated || campaign.status !== 'content_ready'" @click="preview">Review current audience</button></div><p class="muted">Review the segment, saved messages above, and a fresh audience snapshot. Approval freezes these messages and queues mock delivery.</p><AudienceSummary v-if="audience" :segment="campaign.segment" :audience="audience"/><label class="check-review"><input v-model="review" type="checkbox" :disabled="!!busy || stale || dirty || generated || !audience?.approval_fingerprint || campaign.status !== 'content_ready'">I have reviewed the saved messages and this audience, and approve mock delivery.</label><button class="primary" :disabled="!canApprove" @click="approve">Approve &amp; start mock delivery</button></section>
  <DeliveryDashboard v-else :id="campaign.id" @status="campaign.status = $event"/>
</template>
