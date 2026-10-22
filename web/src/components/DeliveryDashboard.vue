<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, errorMessage } from '../api'
import type { Delivery } from '../types'
const props = defineProps<{ id: number }>()
const emit = defineEmits<{ status: [status: Delivery['status']] }>()
const delivery = ref<Delivery | null>(null), error = ref(''), updated = ref(''), loading = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined, controller: AbortController | null = null, stopped = false
async function poll() {
  if (loading.value || stopped) return
  clearTimeout(timer); controller = new AbortController(); loading.value = true
  try {
    const result = await api.delivery(props.id, AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]))
    if (stopped) return
    delivery.value = result; error.value = ''; updated.value = new Date().toLocaleTimeString(); emit('status', result.status)
  } catch (e) { if (!stopped) error.value = errorMessage(e) }
  finally {
    loading.value = false
    if (!stopped && (error.value || !delivery.value || !['completed', 'cancelled'].includes(delivery.value.status))) timer = setTimeout(poll, error.value ? 5000 : 2000)
  }
}
onMounted(poll)
onBeforeUnmount(() => { stopped = true; clearTimeout(timer); controller?.abort() })
</script>
<template>
  <section class="panel"><div class="section-heading"><h2>Delivery dashboard</h2><button class="quiet" :disabled="loading" @click="poll">Refresh delivery</button></div><p class="muted">Mock delivery only. Updates every 2 seconds until completion.</p><p v-if="error" role="alert" class="notice error">{{ error }} Displayed counts may be stale. Retrying in 5 seconds.</p><p v-if="!delivery && loading" role="status">Loading delivery progress...</p>
    <template v-if="delivery"><p v-if="delivery.status === 'completed'" role="status" class="notice success">Campaign completed. All jobs are terminal; completion may include failed or skipped jobs.</p><p v-else role="status">{{ delivery.status === 'sending' ? 'Delivery in progress' : delivery.status }}</p><progress :value="delivery.counts.sent + delivery.counts.dead + delivery.counts.skipped" :max="delivery.counts.total || 1" aria-label="Terminal delivery progress"/><small>{{ delivery.counts.sent + delivery.counts.dead + delivery.counts.skipped }} / {{ delivery.counts.total }} jobs terminal</small><div class="counts"><div v-for="(label, key) in { pending: 'Pending', in_flight: 'In flight', sent: 'Sent', dead: 'Dead', skipped: 'Skipped' }" :key="key" class="count"><strong>{{ delivery.counts[key].toLocaleString() }}</strong><span>{{ label }}</span></div></div><div class="table-scroll"><table><thead><tr><th>Channel</th><th>Pending</th><th>In flight</th><th>Sent</th><th>Dead</th><th>Skipped</th><th>Total</th></tr></thead><tbody><tr v-for="(counts, channel) in delivery.by_channel" :key="channel"><th>{{ String(channel).toUpperCase() }}</th><td>{{ counts?.pending }}</td><td>{{ counts?.in_flight }}</td><td>{{ counts?.sent }}</td><td>{{ counts?.dead }}</td><td>{{ counts?.skipped }}</td><td>{{ counts?.total }}</td></tr></tbody></table></div><p><small>Last updated {{ updated }}. Sent means acknowledged by the mock provider. Dead means retries exhausted or a permanent failure; skipped means dispatch was prevented by a final safety check.</small></p></template>
  </section>
</template>
