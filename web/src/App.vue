<script setup lang="ts">
import { ref } from 'vue'
import CampaignList from './components/CampaignList.vue'
import CampaignDetail from './components/CampaignDetail.vue'
import SystemMetrics from './components/SystemMetrics.vue'
import type { Campaign } from './types'
const selected = ref<Campaign | null>(null)
</script>
<template>
  <header class="app-header"><a href="#" @click.prevent="selected = null" class="brand"><span class="brand-mark">c</span>campaign<span>studio</span></a><span class="header-note">SMART CAMPAIGN COMPOSER</span><span class="environment"><i/> Local sandbox</span></header>
  <main><template v-if="!selected"><CampaignList @open="selected = $event"/><SystemMetrics/></template><CampaignDetail v-else :key="selected.id" :initial="selected" @back="selected = null"/></main>
  <footer>Built for thoughtful campaigns.<span>AI suggests. You decide.</span></footer>
</template>
