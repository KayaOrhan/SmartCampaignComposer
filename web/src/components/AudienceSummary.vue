<script setup lang="ts">
import { t, number, dateTime, channelLabel } from '../i18n'
import type { Audience, Segment, Channel } from '../types'
defineProps<{ segment: Segment | null; audience: Audience | null }>()
function criterion(rule: Segment['all'][number]): string {
  if (rule.field === 'city' || rule.field === 'language') return t(rule.field === 'city' ? 'City is {values}' : 'Language is {values}', { values: Array.isArray(rule.value) ? rule.value.join(` ${t('or')} `) : rule.value })
  if (rule.field === 'cart_added') return t('Added to cart in the last {days} days', { days: number(Number(rule.value)) })
  if (rule.field === 'purchase') return t(rule.op === 'none_within_days' ? 'No purchase in the last {days} days' : 'Purchased in the last {days} days', { days: number(Number(rule.value)) })
  return t('Unsupported criterion. Refresh before continuing.')
}
</script>
<template>
  <ul v-if="segment" class="criteria"><li v-for="(rule, index) in segment.all" :key="index">{{ criterion(rule) }}</li><li>{{ t(`Consent required for each selected channel`) }}</li></ul>
  <p v-else class="muted">{{ t(`Generate a segment to define your audience.`) }}</p>
  <template v-if="audience"><div class="counts"><div class="count"><strong>{{ number(audience.eligible_unique) }}</strong><span>{{ t(`Unique recipients`) }}</span></div><div v-for="(bounds, channel) in audience.by_channel" :key="channel" class="count"><strong>{{ number(bounds?.count ?? 0) }}</strong><span>{{ t(`{channel} eligible`, { channel: channelLabel(channel as Channel) }) }}</span></div></div><small>{{ t(`Snapshot: {date}. A person may be eligible for both channels.`, { date: dateTime(audience.as_of) }) }}</small><p v-if="!audience.eligible_unique" class="notice warning">{{ t(`No eligible recipients. This campaign cannot be approved.`) }}</p></template>
</template>
