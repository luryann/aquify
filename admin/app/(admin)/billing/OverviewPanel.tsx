'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
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
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { formatCents } from '@/lib/financeData'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

type InvoiceWithHousehold = InvoiceRecord & {
  household_name: string
}

type PaymentWithContext = PaymentRecord & {
  household_name: string
  invoice_number: string
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
          setError(uiErrorMessage(err, 'Couldn’t load billing overview. Try again.'))
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
    return <div className="u-p-32px-0" ><EmptyState icon="credit-card" title="Sign in required" description="Sign in to view the billing overview." /></div>
  }

  return (
    <div>
      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <PageSkeleton variant="list" noHeader rows={4} />
      ) : (
        <div className="u-gap-24px" style={{ display: 'flex', flexDirection: 'column' }}>
          <div className="cols-4 u-gap-12px" >
            {[
              { label: 'Households', value: String(households.length) },
              { label: 'Receivables', value: formatCents(metrics.totalReceivables) },
              { label: 'Outstanding', value: formatCents(metrics.outstanding) },
              { label: 'Collected (30d)', value: formatCents(metrics.collected30d) },
            ].map((stat) => (
              <div key={stat.label} className="card-elevated u-p-16px-16px" >
                <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
                <div className="u-fs-fs-2xl u-ff-font-body" style={{ color: 'var(--text)' }}>{stat.value}</div>
              </div>
            ))}
          </div>

          {reconciliation && reconciliation.row_count > 0 ? (
            <div className="u-p-16px-16px" style={{ borderRadius: '12px', background: reconciliation.unreconciled_row_count > 0 ? 'var(--warning-bg)' : 'var(--success-bg)', border: `1px solid ${reconciliation.unreconciled_row_count > 0 ? 'var(--warning-border)' : 'var(--success-border)'}` }}>
              <div className="u-fs-fs-base u-fw-600 u-mb-4px" style={{ color: reconciliation.unreconciled_row_count > 0 ? 'var(--warning)' : 'var(--success)' }}>
                {reconciliation.unreconciled_row_count > 0 ? 'Some payments need review' : 'All payments are accounted for'}
              </div>
              <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
                {formatCents(reconciliation.net_collected_cents)} collected across {reconciliation.row_count} payment{reconciliation.row_count === 1 ? '' : 's'} in the last 30 days.
                {reconciliation.unreconciled_row_count > 0
                  ? ` ${reconciliation.unreconciled_row_count} still need matching to an invoice.`
                  : ''}
              </div>
            </div>
          ) : reconciliation ? (
            <div className="u-p-16px-16px u-fs-fs-sm" style={{ borderRadius: '12px', background: 'var(--bg-subtle, var(--bg-card))', border: '1px solid var(--border)', color: 'var(--text-subtle)' }}>
              No payments in the last 30 days.
            </div>
          ) : null}

          <div className="cols-2 u-gap-20px" >
            <section className="card" style={{ overflow: 'hidden' }}>
              <div className="u-p-16px-20px u-gap-12px" style={{ borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap' }}>
                <h2 className="card-title">Recent payments</h2>
                <button type="button" onClick={() => onNavigateTab('ledger')} className="u-fs-fs-sm u-p-0" style={{ color: 'var(--accent-text, var(--accent))', background: 'none', border: 'none', cursor: 'pointer' }}>Open ledger</button>
              </div>
              {recentPayments.length === 0 ? (
                <div className="u-p-20px" ><EmptyState icon="credit-card" title="No payments yet" description="Successful payments will appear here." /></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  {recentPayments.map((payment, index) => (
                    <div key={payment.id} className="u-p-16px-20px" style={{ borderBottom: index < recentPayments.length - 1 ? '1px solid var(--border)' : 'none' }}>
                      <div className="u-gap-12px u-mb-4px" style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{payment.household_name}</span>
                        <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--accent-text, var(--accent))' }}>{formatCents(payment.amount_cents)}</span>
                      </div>
                      <div className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>
                        {payment.invoice_number} · {payment.method} · {formatDateOnly(payment.paid_at, undefined, 'Pending')}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>

            <section className="card" style={{ overflow: 'hidden' }}>
              <div className="u-p-16px-20px u-gap-12px" style={{ borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap' }}>
                <h2 className="card-title">Overdue invoices</h2>
                <button type="button" onClick={() => onNavigateTab('invoices')} className="u-fs-fs-sm u-p-0" style={{ color: 'var(--accent-text, var(--accent))', background: 'none', border: 'none', cursor: 'pointer' }}>Open invoices</button>
              </div>
              {metrics.overdueInvoices.length === 0 ? (
                <div className="u-p-20px" ><EmptyState icon="shield-check" title="No overdue invoices" description="All current balances are within their due dates." /></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  {metrics.overdueInvoices.slice(0, 8).map((invoice, index) => (
                    <div key={invoice.id} className="u-p-16px-20px" style={{ borderBottom: index < Math.min(metrics.overdueInvoices.length, 8) - 1 ? '1px solid var(--border)' : 'none' }}>
                      <div className="u-gap-12px u-mb-4px" style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>{invoice.household_name}</span>
                        <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--error)' }}>{formatCents(invoice.amount_due_cents)}</span>
                      </div>
                      <div className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>
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
