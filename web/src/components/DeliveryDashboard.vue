<script setup lang="ts">
import { t, number, time, statusLabel, channelLabel } from '../i18n'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage } from '../api'
import type { Delivery, Channel } from '../types'
const labels = { pending: 'Pending', in_flight: 'In flight', sent: 'Sent', dead: 'Dead', skipped: 'Skipped' } as const
const props = defineProps<{ id: number }>()
const emit = defineEmits<{ status: [status: Delivery['status']] }>()
const delivery = ref<Delivery | null>(null), error = ref<unknown>(null), updated = ref<Date | null>(null), loading = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined, controller: AbortController | null = null, stopped = false
async function poll() {
  if (loading.value || stopped) return
  clearTimeout(timer); controller = new AbortController(); loading.value = true
  try {
    const result = await api.delivery(props.id, AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]))
    if (stopped) return
    delivery.value = result; error.value = null; updated.value = new Date(); emit('status', result.status)
  } catch (e) { if (!stopped) error.value = e }
  finally {
    loading.value = false
    if (!stopped && (error.value || !delivery.value || !['completed', 'cancelled'].includes(delivery.value.status))) timer = setTimeout(poll, error.value ? 5000 : 2000)
  }
}
onMounted(poll)
onBeforeUnmount(() => { stopped = true; clearTimeout(timer); controller?.abort() })
</script>
<template>
  <section class="panel"><div class="section-heading"><h2>{{ t(`Delivery dashboard`) }}</h2><button class="quiet" :disabled="loading" @click="poll">{{ t(`Refresh delivery`) }}</button></div><p class="muted">{{ t(`Mock delivery only. Updates every 2 seconds until completion.`) }}</p><p v-if="error" role="alert" class="notice error">{{ errorMessage(error) }} {{ t(`Displayed counts may be stale. Retrying in 5 seconds.`) }}</p><p v-if="!delivery && loading" role="status">{{ t(`Loading delivery progress...`) }}</p>
    <template v-if="delivery"><p v-if="delivery.status === 'completed'" role="status" class="notice success">{{ t(`Campaign completed. All jobs are terminal; completion may include failed or skipped jobs.`) }}</p><p v-else role="status">{{ delivery.status === 'sending' ? t('Delivery in progress') : statusLabel(delivery.status) }}</p><progress :value="delivery.counts.sent + delivery.counts.dead + delivery.counts.skipped" :max="delivery.counts.total || 1" :aria-label="t(`Terminal delivery progress`)"/><small>{{ t(`{done} / {total} jobs terminal`, { done: number(delivery.counts.sent + delivery.counts.dead + delivery.counts.skipped), total: number(delivery.counts.total) }) }}</small><div class="counts"><div v-for="(label, key) in labels" :key="key" class="count"><strong>{{ number(delivery.counts[key]) }}</strong><span>{{ t(label) }}</span></div></div><div class="table-scroll"><table><thead><tr><th>{{ t(`Channel`) }}</th><th>{{ t(`Pending`) }}</th><th>{{ t(`In flight`) }}</th><th>{{ t(`Sent`) }}</th><th>{{ t(`Dead`) }}</th><th>{{ t(`Skipped`) }}</th><th>{{ t(`Total`) }}</th></tr></thead><tbody><tr v-for="(counts, channel) in delivery.by_channel" :key="channel"><th>{{ channelLabel(channel as Channel) }}</th><td>{{ number(counts?.pending ?? 0) }}</td><td>{{ number(counts?.in_flight ?? 0) }}</td><td>{{ number(counts?.sent ?? 0) }}</td><td>{{ number(counts?.dead ?? 0) }}</td><td>{{ number(counts?.skipped ?? 0) }}</td><td>{{ number(counts?.total ?? 0) }}</td></tr></tbody></table></div><p><small>{{ t(`Last updated {time}. Sent means acknowledged by the mock provider. Dead means retries exhausted or a permanent failure; skipped means dispatch was prevented by a final safety check.`, { time: updated ? time(updated) : '—' }) }}</small></p></template>
  </section>
</template>
