/**
 * Copy for the sealed tier (P2.6): error codes from the API and the
 * client-side crypto turned into plain sentences, and the state lines.
 * Pure, no Vue.
 */
import type { Me } from '~/types/session'
import { authIsStale, isStepUp } from '~/utils/privacy'

/** The API wants a second factor for security.manage (owner key actions). */
export const isMfaRequired = (code: string) => code === 'mfa_required'

/** One sentence per refusal; the caller adds SIGN IN AGAIN / SET UP 2FA links for step-up and MFA. */
export function sealedErrorText(err: { error: string, count?: number } | null | undefined): string {
  if (!err) return ''
  if (isStepUp(err.error)) return 'For safety, managing encryption keys needs a recent sign-in (in the last 15 minutes). Sign in again and you\'ll come straight back here.'
  switch (err.error) {
    case 'mfa_required': return 'Managing encryption keys needs two-factor authentication on your account.'
    case 'no_role_grant': case 'forbidden': return 'Only owners can manage the collective\'s encryption keys.'
    case 'wrong_passphrase': return 'That passphrase does not unlock your key. Check it and try again.'
    case 'passphrase_short': return 'Use at least 12 characters.'
    case 'passphrase_mismatch': return 'The two passphrases are not the same.'
    case 'wrap_unreadable': return 'Your key unlocked, but the collective key given to you does not open. Ask an owner to grant you access again.'
    case 'no_access': return 'You have not been given access to the collective key yet. Ask an owner.'
    case 'already_setup': case 'conflict': return 'Sealed data was set up meanwhile (maybe by another owner). Reload to see it.'
    case 'version_conflict': case 'key_version_stale': return 'The collective key changed meanwhile (someone rotated it). Reload and try again.'
    case 'kit_mismatch': return 'Those groups don\'t match the kit. Check the two groups asked for and try again.'
    case 'kit_length': return `A recovery kit has 8 groups of 7 characters (56 in all)${err.count ? `; you typed ${err.count}` : ''}.`
    case 'kit_chars': return 'A recovery kit only uses the letters A–Z and the digits 2–7.'
    case 'kit_checksum': return 'That kit has a typo: one or more characters are wrong. Compare it group by group.'
    case 'kit_wrong_org': return 'This kit belongs to another collective or an older key: its fingerprint does not match.'
    case 'kit_unreadable': return 'The kit is valid but does not open this collective\'s recovery key.'
    case 'unreadable_entries': return `${err.count ?? 'Some'} ban list ${err.count === 1 ? 'entry does' : 'entries do'} not open with the current key. Remove ${err.count === 1 ? 'it' : 'them'} on the ban list first, then rotate.`
    case 'locked': return 'Your key is locked. Unlock it first.'
    case 'recovery_mismatch': return 'The recovery key the server returned does not match the kit\'s fingerprint. Nothing was changed; contact whoever runs your server.'
    case 'not_setup': return 'Sealed data is not set up for this collective yet.'
    case 'no_public_key': return 'This door device has no key. Register it again from the door browser.'
    case 'public_key_mismatch': return 'You already have a key. Only an owner recovering with the kit can replace it.'
    case 'local_identity_required': return 'Sealed data needs a KlubHub account (single sign-on accounts are not supported yet).'
    case 'no_member_key': return 'Create your key first.'
    case 'not_found': return 'That no longer exists. Reload the page.'
    case 'network_error': return 'The server could not be reached. Check your connection and try again.'
    default: return 'Something went wrong. Try again.'
  }
}

/** Ban list form and table refusals. */
export function banErrorText(err: { error: string, field?: string } | null | undefined): string {
  if (!err) return ''
  switch (err.error) {
    case 'key_version_stale': case 'version_conflict': return 'The ban list key changed meanwhile (someone rotated it). RELOAD AND RETRY re-opens the new key and saves again.'
    case 'invalid': return err.field === 'expires_at' ? 'Pick an expiry between tomorrow and 3 years from now.' : 'Check the highlighted fields.'
    case 'no_role_grant': case 'forbidden': return 'Your role can see the ban list but not change it.'
    case 'id_conflict': case 'ban_entry_exists': return 'This entry was already saved. Reload the list.'
    case 'ban_list_full': return 'The ban list is full (2,000 entries). Remove old entries first.'
    case 'not_found': return 'This entry no longer exists (it expired or someone removed it). Reload the list.'
    case 'locked': return 'Your key locked. Unlock it and try again.'
    case 'not_setup': return 'Sealed data is not set up yet.'
    case 'network_error': return 'The server could not be reached. Nothing was saved.'
    default: return 'Could not save. Try again.'
  }
}

/** "42 %" style progress for the Argon2id step (null when not measurable). */
export function progressText(fraction: number | null | undefined): string {
  if (fraction === null || fraction === undefined || !Number.isFinite(fraction)) return ''
  return `${Math.min(100, Math.max(0, Math.round(fraction * 100)))} %`
}

/**
 * Before an owner key action (security.manage: second factor + sign-in in
 * the last 15 minutes): what is missing, so the page can say so before
 * the user types a kit or reads one out. The server checks again.
 */
export function securityBlock(me: Pick<Me, 'mfa' | 'auth_time'> | null | undefined, now = Date.now()): 'mfa' | 'stale' | null {
  if (!me) return null
  if (!me.mfa) return 'mfa'
  return authIsStale(me.auth_time, now) ? 'stale' : null
}

/** The security.manage refusal behind an ApiError, if it is one. */
export function securityRefusal(err: { error: string } | null | undefined): 'mfa' | 'stale' | null {
  if (!err) return null
  if (isMfaRequired(err.error)) return 'mfa'
  return isStepUp(err.error) ? 'stale' : null
}
