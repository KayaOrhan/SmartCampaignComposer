<script setup lang="ts">
import { t, number, statusLabel, channelLabel } from '../i18n'
import { onMounted, ref } from 'vue'
import type { Campaign } from '../types'
import { useCampaign } from '../useCampaign'
import ConfirmDialog from './ConfirmDialog.vue'
import AudienceSummary from './AudienceSummary.vue'
import MessageEditor from './MessageEditor.vue'
import DeliveryDashboard from './DeliveryDashboard.vue'
const props = defineProps<{ initial: Campaign }>()
const emit = defineEmits<{ back: [] }>()
const deleting = ref(false)
const { deleted, remove, openReview, canReview, campaign, saved, messages, audience, review, error, notice, busy, stale, dirty, editable, canSave, canApprove, generated, streamPreview, attempt, refresh, edit, segment, preview, generate, save, approve, cancel } = useCampaign(props.initial)
onMounted(refresh)
async function confirmDelete() {
  await remove()
  deleting.value = false
  if (deleted.value) emit('back')
}
</script>
<template>
  <button class="quiet back" @click="$emit('back')">&larr; {{ t(`All campaigns`) }}</button>
  <div class="page-heading"><div><p class="eyebrow">{{ t(`CAMPAIGN {id} / REVISION {revision}`, { id: number(campaign.id), revision: number(campaign.revision) }) }}</p><h1>{{ t(`Make it meaningful.`) }}</h1><p class="muted detail-brief">{{ campaign.brief }}</p></div><span class="status" :class="campaign.status">{{ statusLabel(campaign.status) }}</span></div>
  <div v-if="error" role="alert" class="notice error">{{ error }}<div class="actions"><button :disabled="!!busy" @click="refresh">{{ t(stale || dirty ? 'Discard local changes and reload' : 'Refresh campaign') }}</button></div></div>
  <p v-if="notice" role="status" class="notice">{{ t(notice) }}</p>
  <div v-if="busy" role="status" class="notice">{{ t(busy) }}... <button v-if="busy === 'Generating messages'" class="quiet" @click="cancel">{{ t(`Cancel generation`) }}</button></div>
  <section class="panel"><div class="section-heading"><h2><span class="step">1</span>{{ t(`Audience`) }}</h2><button v-if="editable" class="secondary" :disabled="!!busy || stale" @click="segment">{{ t(campaign.segment ? 'Regenerate segment' : 'Generate segment') }}</button></div><AudienceSummary :segment="campaign.segment" :audience="audience"/><button v-if="campaign.segment && editable" :disabled="!!busy || stale" @click="preview">{{ t(`Preview audience`) }}</button></section>
  <section class="panel"><div class="section-heading"><h2><span class="step">2</span>{{ t(`Messages`) }}</h2><button v-if="editable" class="secondary" :disabled="!!busy || !campaign.segment || stale" @click="generate">{{ t(`Generate messages`) }}</button></div>
    <template v-if="busy === 'Generating messages'"><p class="stream-label">{{ t(`Streaming preview · attempt {attempt} · not validated or saved`, { attempt: number(attempt) }) }}</p><div class="split"><div v-if="campaign.channels.includes('sms')"><h3>{{ t(`SMS preview`) }}</h3><p class="preview-copy">{{ streamPreview.sms?.body || t('Waiting for content...') }}</p></div><div v-if="campaign.channels.includes('email')"><h3>{{ t(`Email preview`) }}</h3><strong>{{ streamPreview.email?.subject }}</strong><p class="preview-copy">{{ streamPreview.email?.body || t('Waiting for content...') }}</p></div></div></template>
    <p v-if="generated" class="notice">{{ t(`Validated suggestion · not saved. Your edits will be validated again when saved.`) }}</p>
    <p v-else-if="dirty" class="notice warning">{{ t(`Unsaved changes. Save and review again before approval.`) }}</p>
    <p v-else-if="saved?.validation" class="notice">{{ t(`Saved and validated at revision {revision}.`, { revision: number(saved.validation.revision) }) }}</p>
    <p v-else class="notice">{{ t(`No currently validated saved messages. Generate content or revalidate retained text.`) }}</p>
    <MessageEditor :messages="messages" :channels="campaign.channels" :disabled="!!busy || !editable || stale || !campaign.segment" @change="edit"/>
    <div v-if="editable" class="actions"><button class="primary" :disabled="!canSave" @click="save">{{ t(`Save messages`) }}</button><button v-if="dirty || stale || generated" :disabled="!!busy" @click="refresh">{{ t(`Discard local changes and reload`) }}</button></div>
  </section>
  <section v-if="editable" class="panel"><div class="section-heading"><h2><span class="step">3</span>{{ t(`Review & approve`) }}</h2></div><p class="muted">{{ t(`Review the current audience and saved templates together before confirming mock delivery.`) }}</p><button class="primary" :disabled="!canReview" @click="openReview">{{ t(`Review & approve`) }}</button></section>
  <ConfirmDialog v-if="review" :title="t(`Review campaign approval`)" :confirm-label="t(`Confirm approval & start mock delivery`)" :disabled="!canApprove" :busy="!!busy" @close="review = false" @confirm="approve">
    <p>{{ t(`Campaign {id} · revision {revision} · {channels}`, { id: number(campaign.id), revision: number(campaign.revision), channels: campaign.channels.map(channelLabel).join(' + ') }) }}</p>
    <AudienceSummary :segment="campaign.segment" :audience="audience"/>
    <template v-for="channel in campaign.channels" :key="channel"><h3>{{ t(`{channel} saved template`, { channel: channelLabel(channel) }) }}</h3><strong v-if="channel === 'email'">{{ saved?.messages?.email?.subject }}</strong><p class="preview-copy">{{ saved?.messages?.[channel]?.body }}</p></template>
    <p class="notice">{{ t(`By confirming, you approve these saved templates for this audience and start mock delivery. Audience changes will require a new review.`) }}</p>
  </ConfirmDialog>
  <DeliveryDashboard v-if="!editable" :id="campaign.id" @status="campaign.status = $event"/>
  <section v-if="editable" class="panel"><h2>{{ t(`Delete draft`) }}</h2><p class="muted">{{ t(`Remove this unapproved campaign from the workspace. Stored content and audit records are retained.`) }}</p><button :disabled="!!busy || stale" @click="deleting = true">{{ t(`Delete draft`) }}</button></section>
  <ConfirmDialog v-if="deleting" :title="t(`Delete this draft campaign?`)" :confirm-label="t(`Confirm deletion`)" :busy="!!busy" @close="deleting = false" @confirm="confirmDelete"><p>{{ t(`Campaign {id}: {brief}`, { id: number(campaign.id), brief: campaign.brief }) }}</p><p>{{ t(`This removes the unapproved campaign from the workspace. It cannot be restored here. Stored content and audit records are retained; campaigns with approval or delivery history cannot be deleted.`) }}</p></ConfirmDialog>
</template>
