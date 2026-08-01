<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div class="space-y-1">
        <h3 class="text-xl font-semibold tracking-tight">{{ $t('dashboard.title') }}</h3>
        <p class="text-muted-foreground text-sm">{{ $t('dashboard.subtitle') }}</p>
      </div>
      <button @click="fetchDevices" class="p-2 rounded-md hover:bg-muted text-muted-foreground transition-colors" :title="$t('common.refresh')">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
      </button>
    </div>

    <div v-if="loading && devices.length === 0" class="flex justify-center py-12">
      <div class="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
    </div>
    
    <div v-else-if="devices.length === 0" class="flex flex-col items-center justify-center py-16 border border-dashed border-border rounded-lg bg-background">
      <Monitor class="w-10 h-10 text-muted-foreground mb-4 opacity-40" stroke-width="1.5" />
      <h4 class="text-base font-medium">{{ $t('dashboard.noDevices') }}</h4>
      <p class="text-sm text-muted-foreground text-center max-w-sm mt-1">{{ $t('dashboard.noDevicesHint') }}</p>
    </div>

    <div v-else class="w-full overflow-x-auto border border-border rounded-lg bg-card shadow-sm">
      <table class="w-full text-sm text-left border-collapse">
        <thead class="bg-muted/50 text-muted-foreground text-xs uppercase tracking-wider border-b border-border">
          <tr>
            <th class="px-6 py-4 font-medium">{{ $t('devices.name') }}</th>
            <th class="px-6 py-4 font-medium">{{ $t('devices.platform') }}</th>
            <th class="px-6 py-4 font-medium">{{ $t('devices.status') }}</th>
            <th class="px-6 py-4 font-medium hidden md:table-cell">{{ $t('devices.lastSeen') }}</th>
            <th class="px-6 py-4 font-medium text-right">{{ $t('devices.actions') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/50">
          <tr v-for="device in devices" :key="device.id || device.deviceId" class="hover:bg-muted/30 transition-colors">
            <!-- Name -->
            <td class="px-6 py-4 align-middle">
              <div class="flex items-center gap-3">
                <div class="w-9 h-9 rounded-md bg-muted flex items-center justify-center text-foreground shrink-0 border border-border/40">
                  <Smartphone v-if="device.type === 'android' || device.type === 'ios'" class="w-4 h-4" />
                  <Monitor v-else class="w-4 h-4" />
                </div>
                <div>
                  <h4 class="font-medium text-sm text-foreground truncate max-w-[200px]" :title="device.name || $t('dashboard.unnamedDevice')">
                    {{ device.name || $t('dashboard.unnamedDevice') }}
                  </h4>
                  <span class="md:hidden block text-xs text-muted-foreground mt-0.5">
                    {{ formatDate(device.last_seen || device.lastSeen) }}
                  </span>
                </div>
              </div>
            </td>

            <!-- Platform -->
            <td class="px-6 py-4 align-middle capitalize text-muted-foreground">
              {{ device.type || $t('dashboard.unknownPlatform') }}
            </td>

            <!-- Status -->
            <td class="px-6 py-4 align-middle">
              <span v-if="device.online" class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-500 border border-emerald-500/20">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                {{ $t('dashboard.online') }}
              </span>
              <span v-else class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-muted text-muted-foreground border border-border">
                <span class="w-1.5 h-1.5 rounded-full bg-muted-foreground/40"></span>
                {{ $t('dashboard.offline') }}
              </span>
            </td>

            <!-- Last Seen -->
            <td class="px-6 py-4 align-middle text-muted-foreground hidden md:table-cell">
              {{ formatDate(device.last_seen || device.lastSeen) }}
            </td>

            <!-- Actions -->
            <td class="px-6 py-4 align-middle text-right">
              <div class="inline-flex items-center gap-1">
                <button
                  @click="triggerPush(device)"
                  class="inline-flex items-center justify-center p-2 rounded-md text-muted-foreground hover:text-primary hover:bg-primary/10 transition-colors"
                  :title="$t('dashboard.sendPush')"
                >
                  <Bell class="w-4 h-4" />
                </button>
                <button
                  @click="triggerRemoveDevice(device.id || device.deviceId)"
                  class="inline-flex items-center justify-center p-2 rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                  :title="$t('common.remove')"
                >
                  <Trash2 class="w-4 h-4" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Confirm Remove Device Dialog -->
    <Dialog 
      v-model:open="confirmDialogOpen"
      :title="$t('dashboard.removeDeviceConfirm')"
      description=""
      :confirmText="$t('common.remove')"
      variant="destructive"
      @confirm="confirmRemoveDevice"
    />

    <Dialog
      v-model:open="pushDialogOpen"
      :title="$t('dashboard.sendPush')"
      :description="pushTarget ? $t('dashboard.pushTo', { device: pushTarget.name }) : ''"
      :confirmText="$t('dashboard.sendPush')"
      @confirm="sendPush"
    >
      <div class="space-y-4">
        <label class="block space-y-1.5">
          <span class="text-sm font-medium">{{ $t('dashboard.pushTitle') }}</span>
          <input
            v-model="pushForm.title"
            type="text"
            maxlength="200"
            :placeholder="$t('dashboard.pushTitlePlaceholder')"
            class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
          />
        </label>
        <label class="block space-y-1.5">
          <span class="text-sm font-medium">{{ $t('dashboard.pushBody') }}</span>
          <textarea
            v-model="pushForm.body"
            rows="4"
            maxlength="4000"
            :placeholder="$t('dashboard.pushBodyPlaceholder')"
            class="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
          ></textarea>
        </label>
      </div>
    </Dialog>

    <!-- Alert Message Dialog -->
    <Dialog 
      v-model:open="alertDialog.open"
      :title="alertDialog.title"
      :description="alertDialog.description"
      :confirmText="$t('common.ok')"
      hideCancel
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Bell, Monitor, Smartphone, Trash2, RefreshCw } from 'lucide-vue-next'
import request from '@/utils/request'
import Dialog from '@/components/Dialog.vue'

const { t } = useI18n()
const devices = ref<any[]>([])
const loading = ref(false)

const confirmDialogOpen = ref(false)
const deviceToRemove = ref<string | null>(null)
const pushDialogOpen = ref(false)
const pushTarget = ref<{ id: string, name: string } | null>(null)
const pushForm = reactive({
  title: '',
  body: ''
})

const alertDialog = reactive({
  open: false,
  title: '',
  description: ''
})

const showAlert = (title: string, description: string) => {
  alertDialog.title = title
  alertDialog.description = description
  alertDialog.open = true
}

const fetchDevices = async () => {
  loading.value = true
  try {
    const res: any = await request.get('/devices')
    if (res.code === 0) {
      devices.value = res.data?.devices || []
    }
  } catch (error) {
    console.error('Failed to fetch devices', error)
  } finally {
    loading.value = false
  }
}

const triggerRemoveDevice = (id: string) => {
  if (!id) return
  deviceToRemove.value = id
  confirmDialogOpen.value = true
}

const confirmRemoveDevice = async () => {
  if (!deviceToRemove.value) return
  const id = deviceToRemove.value
  try {
    const res: any = await request.delete(`/devices/${id}`)
    if (res.code === 0) {
      devices.value = devices.value.filter(d => (d.id || d.deviceId) !== id)
    } else {
      showAlert(t('dashboard.removeDeviceFailed'), res.message || '')
    }
  } catch (error) {
    showAlert(t('dashboard.removeDeviceFailed'), '')
  } finally {
    deviceToRemove.value = null
  }
}

const triggerPush = (device: any) => {
  const id = device.id || device.deviceId
  if (!id) return
  pushTarget.value = {
    id,
    name: device.name || t('dashboard.unnamedDevice')
  }
  pushForm.title = ''
  pushForm.body = ''
  pushDialogOpen.value = true
}

const sendPush = async () => {
  const target = pushTarget.value
  if (!target) return
  try {
    const res: any = await request.post('/push', {
      device_key: target.id,
      title: pushForm.title || undefined,
      body: pushForm.body || undefined
    }, {
      baseURL: '/api'
    })
    if (res.code === 200) {
      showAlert(t('dashboard.pushSent'), '')
    } else {
      showAlert(t('dashboard.pushFailed'), res.message || '')
    }
  } catch (error) {
    showAlert(t('dashboard.pushFailed'), '')
  } finally {
    pushTarget.value = null
  }
}

const formatDate = (ts: number) => {
  if (!ts) return t('common.never')
  return new Date(ts).toLocaleString()
}

onMounted(() => {
  fetchDevices()
})
</script>
