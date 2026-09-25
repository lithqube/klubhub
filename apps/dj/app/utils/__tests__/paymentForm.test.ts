import { describe, it, expect } from 'vitest'
import { checkPaymentAmount, paymentLimit, pendingRefunds } from '../paymentForm'

const issued = { currency: 'EUR', outstanding_minor: 50000, received_minor: 30000 }

describe('paymentLimit', () => {
  it('caps money in at the outstanding balance', () => {
    expect(paymentLimit(issued, 'payment')).toBe(50000)
    expect(paymentLimit(issued, 'deposit')).toBe(50000)
  })

  it('caps refunds at received minus pending refunds', () => {
    expect(paymentLimit(issued, 'refund')).toBe(30000)
    const payments = [
      { kind: 'refund' as const, status: 'pending' as const, amount_minor: 10000 },
      { kind: 'refund' as const, status: 'completed' as const, amount_minor: 99999 },
      { kind: 'payment' as const, status: 'pending' as const, amount_minor: 5000 },
    ]
    expect(pendingRefunds(payments)).toBe(10000)
    expect(paymentLimit(issued, 'refund', payments)).toBe(20000)
  })

  it('never goes negative', () => {
    expect(paymentLimit({ ...issued, received_minor: 0 }, 'refund', [{ kind: 'refund', status: 'pending', amount_minor: 5 }])).toBe(0)
  })
})

describe('checkPaymentAmount', () => {
  it('is silent but invalid while empty', () => {
    expect(checkPaymentAmount('', 'payment', issued)).toMatchObject({ valid: false, message: '', amount: null })
  })

  it('accepts an amount up to the limit', () => {
    expect(checkPaymentAmount('500', 'payment', issued)).toMatchObject({ valid: true, amount: 50000 })
    expect(checkPaymentAmount('250,50', 'deposit', issued)).toMatchObject({ valid: true, amount: 25050 })
  })

  it('rejects more than the outstanding balance with the max in the message', () => {
    const r = checkPaymentAmount('500.01', 'payment', issued)
    expect(r.valid).toBe(false)
    expect(r.message).toContain('€500.00')
    expect(r.message).toContain('outstanding')
  })

  it('rejects refunds above what was received', () => {
    const r = checkPaymentAmount('301', 'refund', issued)
    expect(r.valid).toBe(false)
    expect(r.message).toContain('€300.00')
    expect(r.message).toContain('received')
  })

  it('rejects zero, garbage and too many decimals', () => {
    expect(checkPaymentAmount('0', 'payment', issued).message).toMatch(/above zero/)
    expect(checkPaymentAmount('abc', 'payment', issued).message).toMatch(/Enter an amount/)
    expect(checkPaymentAmount('1.234', 'payment', issued).valid).toBe(false)
  })

  it('explains when nothing can be recorded', () => {
    expect(checkPaymentAmount('10', 'payment', { ...issued, outstanding_minor: 0 }).message).toMatch(/Nothing is outstanding/)
    expect(checkPaymentAmount('10', 'refund', { ...issued, received_minor: 0 }).message).toMatch(/nothing to refund/)
  })
})
