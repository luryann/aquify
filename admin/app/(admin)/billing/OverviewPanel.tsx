'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import {
  fetchHouseholds,
  fetchInvoices,
  fetchPaymentReconciliationReport,
  fetchPayments,
  type HouseholdRecord,
  type InvoiceRecord,
  type PaymentRecord,
} from '@/lib/adminData'

type InvoiceWithHousehold = InvoiceRecord & {
  household_name: string
}

type PaymentWithContext = PaymentRecord & {
  household_name: string
  invoice_number: string
}

function formatCurrency(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

export function OverviewPanel({ onNavigateTab }: { onNavigateTab: (tab: 'invoices' | 'ledger') => void }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [invoices, setInvoices] = useState<InvoiceWithHousehold[]>([])
  const [payments, setPayments] = useState<PaymentWithContext[]>([])
  const [reconciliation, setReconciliation] = useState<Awaited<ReturnType<typeof fetchPaymentReconciliationReport>> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [nextHouseholds, rawInvoices, rawPayments, nextReconciliation] = await Promise.all([
          fetchHouseholds(api, session.user.orgId),
          fetchInvoices(api, session.user.orgId),
          fetchPayments(api, session.user.orgId),
          fetchPaymentReconciliationReport(api, session.user.orgId).catch(() => null),
        ])
        const householdsByID = new Map(nextHouseholds.map((household) => [household.id, household.household_name]))
        const nextInvoices = rawInvoices.map((invoice) => ({
          ...invoice,
          household_name: householdsByID.get(invoice.household_id ?? '') ?? 'Unknown household',
        }))
        const invoicesByID = new Map(nextInvoices.map((invoice) => [invoice.id, invoice]))
        const nextPayments = rawPayments.map((payment) => {
          const invoice = invoicesByID.get(payment.invoice_id)
          return {
            ...payment,
            household_name: invoice?.household_name ?? 'Unknown household',
            invoice_number: invoice?.invoice_number ?? 'Unknown invoice',
          }
        })

        if (cancelled) return
        setHouseholds(nextHouseholds)
        setInvoices(nextInvoices)
        setPayments(nextPayments)
        setReconciliation(nextReconciliation)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load billing overview.')
          setHouseholds([])
          setInvoices([])
          setPayments([])
          setReconciliation(null)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const metrics = useMemo(() => {
    const totalReceivables = invoices.reduce((sum, invoice) => sum + invoice.total_cents, 0)
    const outstanding = invoices.reduce((sum, invoice) => sum + invoice.amount_due_cents, 0)
    const overdueInvoices = invoices.filter((invoice) => invoice.due_date && invoice.amount_due_cents > 0 && parseBackendDate(invoice.due_date) < new Date())
    const overdue = overdueInvoices.reduce((sum, invoice) => sum + invoice.amount_due_cents, 0)
    const collected30d = payments
      .filter((payment) => payment.paid_at && new Date(payment.paid_at).getTime() >= Date.now() - 30 * 24 * 60 * 60 * 1000)
      .reduce((sum, payment) => sum + payment.amount_cents, 0)

    return {
      totalReceivables,
      outstanding,
      overdue,
      collected30d,
      overdueInvoices,
    }
  }, [invoices, payments])

  const recentPayments = useMemo(() => [...payments]
    .filter((payment) => payment.paid_at)
    .sort((a, b) => (b.paid_at ?? '').localeCompare(a.paid_at ?? ''))
    .slice(0, 8), [payments])

  if (!session) {
    return <div style={{ padding: '32px 0' }}><EmptyState icon="credit-card" title="Sign in required" description="Sign in to view the billing overview." /></div>
  }

  return (
    <div>
      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <EmptyState icon="credit-card" title="Loading billing summary" description="Aggregating invoice and payment data." />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px' }}>
            {[
              { label: 'Households', value: String(households.length) },
              { label: 'Receivables', value: formatCurrency(metrics.totalReceivables) },
              { label: 'Outstanding', value: formatCurrency(metrics.outstanding) },
              { label: 'Collected (30d)', value: formatCurrency(metrics.collected30d) },
            ].map((stat) => (
              <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
                <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</div>
                <div style={{ fontSize: '22px', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>{stat.value}</div>
              </div>
            ))}
          </div>

          {reconciliation ? (
            <div style={{ padding: '14px 16px', borderRadius: '10px', background: reconciliation.unreconciled_row_count > 0 ? 'var(--warning-bg)' : 'var(--success-bg)', border: `1px solid ${reconciliation.unreconciled_row_count > 0 ? 'var(--warning-border)' : 'var(--success-border)'}` }}>
              <div style={{ fontSize: '13px', fontWeight: 600, color: reconciliation.unreconciled_row_count > 0 ? 'var(--warning)' : 'var(--success)', marginBottom: '4px' }}>
                {reconciliation.unreconciled_row_count > 0 ? 'Some payments need review' : 'All payments are accounted for'}
              </div>
              <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>
                {formatCurrency(reconciliation.net_collected_cents)} collected across {reconciliation.row_count} payment{reconciliation.row_count === 1 ? '' : 's'} in the last 30 days. {reconciliation.unreconciled_row_count} payment{reconciliation.unreconciled_row_count === 1 ? '' : 's'} still need matching to an invoice.
              </div>
            </div>
          ) : null}

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px' }}>
            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
                <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Recent Payments</h2>
                <button onClick={() => onNavigateTab('ledger')} style={{ fontSize: '12px', color: 'var(--accent)', background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}>Open ledger</button>
              </div>
              {recentPayments.length === 0 ? (
                <div style={{ padding: '20px' }}><EmptyState icon="credit-card" title="No payments yet" description="Successful payments will appear here." /></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  {recentPayments.map((payment, index) => (
                    <div key={payment.id} style={{ padding: '14px 18px', borderBottom: index < recentPayments.length - 1 ? '1px solid var(--border)' : 'none' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '4px' }}>
                        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{payment.household_name}</span>
                        <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--accent)' }}>{formatCurrency(payment.amount_cents)}</span>
                      </div>
                      <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                        {payment.invoice_number} · {payment.method} · {payment.paid_at ? new Date(payment.paid_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : 'Pending'}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>

            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
                <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Overdue Invoices</h2>
                <button onClick={() => onNavigateTab('invoices')} style={{ fontSize: '12px', color: 'var(--accent)', background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}>Open invoices</button>
              </div>
              {metrics.overdueInvoices.length === 0 ? (
                <div style={{ padding: '20px' }}><EmptyState icon="shield-check" title="No overdue invoices" description="All current balances are within their due dates." /></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  {metrics.overdueInvoices.slice(0, 8).map((invoice, index) => (
                    <div key={invoice.id} style={{ padding: '14px 18px', borderBottom: index < Math.min(metrics.overdueInvoices.length, 8) - 1 ? '1px solid var(--border)' : 'none' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '4px' }}>
                        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{invoice.household_name}</span>
                        <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--error)' }}>{formatCurrency(invoice.amount_due_cents)}</span>
                      </div>
                      <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                        {invoice.invoice_number} · Due {formatDateOnly(invoice.due_date)} · Status {invoice.status}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>
        </div>
      )}
    </div>
  )
}
