<template>
  <main class="flex min-h-screen items-center justify-center bg-background px-6 py-16 text-foreground">
    <div class="w-full max-w-[360px]">
      <header class="mb-8 flex items-center gap-4 select-none">
        <BrandMark class="h-11 w-11 shrink-0" />
        <div class="min-w-0">
          <h1 class="text-xl font-semibold leading-tight tracking-tight">{{ title }}</h1>
          <p class="mt-1 text-sm text-muted-foreground text-balance">{{ subtitle }}</p>
        </div>
      </header>

      <slot />

      <p class="mt-8 text-sm text-muted-foreground">
        <slot name="footer" />
      </p>
    </div>
  </main>
</template>

<script setup lang="ts">
import BrandMark from '@/components/BrandMark.vue'

defineProps<{
  title: string
  subtitle: string
}>()
</script>
