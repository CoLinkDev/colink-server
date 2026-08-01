<template>
  <Teleport to="body">
    <transition name="fade">
      <div v-if="open" class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <!-- Backdrop -->
        <div @click="handleCancel" class="fixed inset-0 bg-background/80 backdrop-blur-sm transition-opacity"></div>
        
        <!-- Modal Content -->
        <div class="relative z-10 w-full max-w-md rounded-lg border border-border bg-card p-6 shadow-lg duration-200 animate-in fade-in-0 zoom-in-95">
          <div class="flex flex-col space-y-1.5 text-center sm:text-left">
            <h3 class="text-lg font-semibold leading-none tracking-tight">{{ title }}</h3>
            <p class="text-sm text-muted-foreground mt-1.5">{{ description }}</p>
          </div>
          
          <div v-if="$slots.default" class="py-4">
            <slot />
          </div>
          
          <div class="flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2 mt-6 gap-2">
            <button 
              v-if="!hideCancel"
              @click="handleCancel"
              class="inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring border border-input bg-background hover:bg-muted h-9 px-4 py-2"
            >
              {{ cancelText || $t('common.cancel') }}
            </button>
            <button 
              v-if="!hideConfirm"
              @click="handleConfirm"
              :class="[
                'inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring h-9 px-4 py-2 shadow-sm',
                variant === 'destructive' 
                  ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90' 
                  : 'bg-primary text-primary-foreground hover:bg-primary/90'
              ]"
            >
              {{ confirmText || $t('common.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </transition>
  </Teleport>
</template>

<script setup lang="ts">
interface Props {
  open: boolean
  title: string
  description: string
  confirmText?: string
  cancelText?: string
  variant?: 'default' | 'destructive'
  hideCancel?: boolean
  hideConfirm?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  confirmText: '',
  cancelText: '',
  variant: 'default',
  hideCancel: false,
  hideConfirm: false
})

const emit = defineEmits(['update:open', 'confirm', 'cancel'])

const handleCancel = () => {
  emit('update:open', false)
  emit('cancel')
}

const handleConfirm = () => {
  emit('update:open', false)
  emit('confirm')
}
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
