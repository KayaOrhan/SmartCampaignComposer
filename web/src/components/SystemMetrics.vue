<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { checked, errorMessage } from '../api'
const expanded = ref(false), text = ref(''), error = ref(''), loading = ref(false)
let controller: AbortController | null = null
async function load() {
  if (loading.value) return
  loading.value = true; error.value = ''; controller = new AbortController()
  try { const response = await checked(await fetch('/api/metrics', { cache: 'no-store', signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]) })); text.value = await response.text() }
  catch (e) { error.value = errorMessage(e) }
  finally { loading.value = false }
}
function toggle() { expanded.value = !expanded.value; if (expanded.value) void load(); else controller?.abort() }
onBeforeUnmount(() => controller?.abort())
</script>
<template><section class="panel"><div class="section-heading"><h2>System metrics</h2><button class="quiet" :aria-expanded="expanded" @click="toggle">{{ expanded ? 'Hide metrics' : 'View metrics' }}</button></div><p class="muted">Aggregate queue and AI validation counters across all campaigns.</p><template v-if="expanded"><p v-if="error" class="notice error" role="alert">{{ error }}</p><p v-if="loading" role="status">Loading metrics...</p><pre v-if="text" class="metric-text">{{ text }}</pre><button :disabled="loading" @click="load">Refresh metrics</button></template></section></template>
