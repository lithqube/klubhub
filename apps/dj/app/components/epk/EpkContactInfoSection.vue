<script setup lang="ts">
import { ref, onMounted } from 'vue'
import Input from '~/components/ui/input/Input.vue'

type ContactInfo = { name: string; email: string; phone: string }

const contact = ref<ContactInfo>({ name: '', email: '', phone: '' })

// Load existing contact info from settings API
onMounted(async () => {
  try {
    const data = await $fetch<Record<string, any>>('/api/v1/settings')
    if (data.contact_info && typeof data.contact_info === 'object') {
      contact.value = {
        name: data.contact_info.name ?? '',
        email: data.contact_info.email ?? '',
        phone: data.contact_info.phone ?? '',
      }
    }
  } catch {
    // Ignore load errors — blank fields is acceptable default
  }
})

let saveTimer: ReturnType<typeof setTimeout> | null = null

function scheduleSave() {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(async () => {
    try {
      await $fetch('/api/v1/settings', {
        method: 'PUT',
        body: { contact_info: contact.value },
      })
    } catch {
      // Ignore save errors silently
    }
  }, 1500)
}

function onFieldChange(field: keyof ContactInfo, value: string | number) {
  contact.value = { ...contact.value, [field]: String(value) }
  scheduleSave()
}
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Section label -->
    <p class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">CONTACT INFO</p>

    <div class="space-y-3">
      <!-- Name -->
      <div class="space-y-1">
        <label class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">NAME</label>
        <Input
          :model-value="contact.name"
          data-testid="contact-name-input"
          placeholder="Contact name..."
          @update:model-value="onFieldChange('name', $event)"
        />
      </div>

      <!-- Email -->
      <div class="space-y-1">
        <label class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">EMAIL</label>
        <Input
          :model-value="contact.email"
          data-testid="contact-email-input"
          type="email"
          placeholder="Contact email..."
          @update:model-value="onFieldChange('email', $event)"
        />
      </div>

      <!-- Phone -->
      <div class="space-y-1">
        <label class="text-xs tracking-widest text-muted-foreground uppercase font-terminal">PHONE</label>
        <Input
          :model-value="contact.phone"
          data-testid="contact-phone-input"
          type="tel"
          placeholder="Contact phone..."
          @update:model-value="onFieldChange('phone', $event)"
        />
      </div>
    </div>
  </div>
</template>
