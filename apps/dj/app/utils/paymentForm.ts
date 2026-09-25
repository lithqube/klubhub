// Pre-validation for the payment form. Mirrors the API limits so the user
// sees the problem before saving: money in is capped at the outstanding
// balance, refunds at what has been received (minus refunds still pending).

import type { Invoice, Payment, PaymentKind } from '../types/finance'
import { formatMinor, parseMoney } from './money'

export const PAYMENT_KINDS: { value: PaymentKind; label: string; hint: string }[] = [
  { value: 'deposit', label: 'DEPOSIT', hint: 'Part paid up front.' },
  { value: 'payment', label: 'PAYMENT', hint: 'Money received against the invoice.' },
  { value: 'refund', label: 'REFUND', hint: 'Money you paid back to the promoter.' },
]

type BalanceFields = Pick<Invoice, 'currency' | 'outstanding_minor' | 'received_minor'>

/** Refunds recorded but not yet completed; they already reserve received money. */
export function pendingRefunds(payments: Pick<Payment, 'kind' | 'status' | 'amount_minor'>[] = []): number {
  return payments
    .filter((p) => p.kind === 'refund' && p.status === 'pending')
    .reduce((sum, p) => sum + p.amount_minor, 0)
}

/** Largest amount (minor units) that can be recorded for a kind. */
export function paymentLimit(
  inv: BalanceFields,
  kind: PaymentKind,
  payments: Pick<Payment, 'kind' | 'status' | 'amount_minor'>[] = [],
): number {
  if (kind === 'refund') return Math.max(0, inv.received_minor - pendingRefunds(payments))
  return Math.max(0, inv.outstanding_minor)
}

export interface AmountCheck {
  /** Parsed amount in minor units, or null when the text is not an amount. */
  amount: number | null
  limit: number
  valid: boolean
  /** User-facing reason when invalid ('' while the field is still empty). */
  message: string
}

export function checkPaymentAmount(
  text: string,
  kind: PaymentKind,
  inv: BalanceFields,
  payments: Pick<Payment, 'kind' | 'status' | 'amount_minor'>[] = [],
): AmountCheck {
  const limit = paymentLimit(inv, kind, payments)
  const refund = kind === 'refund'
  if (limit === 0) {
    return {
      amount: null,
      limit,
      valid: false,
      message: refund
        ? 'Nothing has been received yet, so there is nothing to refund.'
        : 'Nothing is outstanding on this invoice.',
    }
  }
  if (text.trim() === '') return { amount: null, limit, valid: false, message: '' }
  const amount = parseMoney(text, inv.currency)
  if (amount === null) {
    return { amount, limit, valid: false, message: `Enter an amount like ${formatMinor(25000, inv.currency)}.` }
  }
  if (amount <= 0) return { amount, limit, valid: false, message: 'The amount must be above zero.' }
  if (amount > limit) {
    const max = formatMinor(limit, inv.currency)
    return {
      amount,
      limit,
      valid: false,
      message: refund
        ? `At most ${max} can be refunded: that is what has been received.`
        : `At most ${max} can be recorded: that is what is still outstanding.`,
    }
  }
  return { amount, limit, valid: true, message: '' }
}
