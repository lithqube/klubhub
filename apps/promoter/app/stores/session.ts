import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Me } from '~/types/session'
import { apiFetch } from '~/utils/api'

/** Roles the policy requires a second factor for (security, finance). */
const MFA_ROLES = new Set(['owner', 'admin', 'finance'])

function errorCode(e: unknown): string {
  const data = (e as { data?: { error?: string } })?.data
  return data?.error ?? 'network_error'
}

export const useSessionStore = defineStore('session', () => {
  const me = ref<Me | null>(null)
  const loaded = ref(false)

  const isAuthenticated = computed(() => me.value !== null)
  /** Owner/admin/finance without TOTP: most of their actions will be refused. */
  const needsMfa = computed(() => !!me.value && !me.value.mfa && me.value.roles.some(r => MFA_ROLES.has(r)))

  async function fetchMe(): Promise<Me | null> {
    try {
      me.value = await apiFetch<Me>('/api/v1/auth/me')
    } catch {
      me.value = null
    }
    loaded.value = true
    return me.value
  }

  /** Returns null on success or an error code (invalid_credentials, totp_required, …). */
  async function login(email: string, password: string, totp?: string): Promise<string | null> {
    try {
      await apiFetch('/api/v1/auth/login', { method: 'POST', body: { email, password, totp: totp ?? '' } })
      await fetchMe()
      return null
    } catch (e) {
      return errorCode(e)
    }
  }

  async function logout(): Promise<void> {
    try {
      await apiFetch('/api/v1/auth/logout', { method: 'POST' })
    } finally {
      me.value = null
    }
  }

  async function completeSetup(token: string, password: string): Promise<string | null> {
    try {
      await apiFetch('/api/v1/auth/setup', { method: 'POST', body: { token, password } })
      return null
    } catch (e) {
      return errorCode(e)
    }
  }

  async function enrollTotp(): Promise<string> {
    const res = await apiFetch<{ otpauth_uri: string }>('/api/v1/auth/totp/enroll', { method: 'POST' })
    return res.otpauth_uri
  }

  async function confirmTotp(code: string): Promise<string | null> {
    try {
      await apiFetch('/api/v1/auth/totp/confirm', { method: 'POST', body: { code } })
      await fetchMe()
      return null
    } catch (e) {
      return errorCode(e)
    }
  }

  return { me, loaded, isAuthenticated, needsMfa, fetchMe, login, logout, completeSetup, enrollTotp, confirmTotp }
})
