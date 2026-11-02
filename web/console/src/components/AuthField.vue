<template>
  <div class="space-y-2">
    <label :for="id" class="block text-sm font-medium leading-none">{{ label }}</label>
    <input
      :id="id"
      v-model="model"
      v-bind="$attrs"
      class="flex h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm transition-[border-color,box-shadow] placeholder:text-muted-foreground/70 focus-visible:border-ring focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-ring/10 disabled:cursor-not-allowed disabled:opacity-50"
    >
  </div>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false })

defineProps<{
  id: string
  label: string
}>()

const model = defineModel<string>({ required: true })
</script>
