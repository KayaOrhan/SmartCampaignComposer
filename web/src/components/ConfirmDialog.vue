<script setup lang="ts">
import { t } from '../i18n'
import { onMounted, onBeforeUnmount, ref } from 'vue'
defineProps<{ title: string; confirmLabel: string; disabled?: boolean; busy?: boolean }>()
const emit = defineEmits<{ confirm: []; close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
let previousFocus: HTMLElement | null = null
onMounted(() => { previousFocus = document.activeElement as HTMLElement | null; dialog.value?.showModal() })
onBeforeUnmount(() => { dialog.value?.close(); previousFocus?.focus() })
</script>
<template>
  <dialog ref="dialog" aria-labelledby="confirmation-title" @cancel.prevent="!busy && emit('close')">
    <h2 id="confirmation-title">{{ title }}</h2>
    <slot/>
    <div class="actions"><button autofocus :disabled="busy" @click="emit('close')">{{ t(`Cancel`) }}</button><button class="primary" :disabled="disabled || busy" @click="emit('confirm')">{{ confirmLabel }}</button></div>
  </dialog>
</template>
