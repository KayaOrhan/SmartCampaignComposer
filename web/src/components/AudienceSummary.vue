<script setup lang="ts">
import type { Audience, Segment } from '../types'
defineProps<{ segment: Segment | null; audience: Audience | null }>()
function criterion(rule: Segment['all'][number]): string {
  if (rule.field === 'city' || rule.field === 'language') return `${rule.field === 'city' ? 'City' : 'Language'} is ${Array.isArray(rule.value) ? rule.value.join(' or ') : rule.value}`
  if (rule.field === 'cart_added') return `Added to cart in the last ${rule.value} days`
  if (rule.field === 'purchase') return `${rule.op === 'none_within_days' ? 'No purchase' : 'Purchased'} in the last ${rule.value} days`
  return 'Unsupported criterion. Refresh before continuing.'
}
</script>
<template>
  <ul v-if="segment" class="criteria"><li v-for="(rule, index) in segment.all" :key="index">{{ criterion(rule) }}</li><li>Consent required for each selected channel</li></ul>
  <p v-else class="muted">Generate a segment to define your audience.</p>
  <template v-if="audience"><div class="counts"><div class="count"><strong>{{ audience.eligible_unique.toLocaleString() }}</strong><span>Unique recipients</span></div><div v-for="(bounds, channel) in audience.by_channel" :key="channel" class="count"><strong>{{ bounds?.count.toLocaleString() }}</strong><span>{{ String(channel).toUpperCase() }} eligible</span></div></div><small>Snapshot: {{ new Date(audience.as_of).toLocaleString() }}. A person may be eligible for both channels.</small><p v-if="!audience.eligible_unique" class="notice warning">No eligible recipients. This campaign cannot be approved.</p></template>
</template>
