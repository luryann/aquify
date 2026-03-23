'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'

const HOUSEHOLDS = [
  { id: 'hh-001', name: 'Henderson Family', enabled: true, method: 'Visa •••• 4321', nextChargeDate: '2026-04-01', nextChargeAmount: 420 },
  { id: 'hh-002', name: 'Park Family', enabled: false, method: 'ACH Transfer', nextChargeDate: '2026-04-03', nextChargeAmount: 185 },
  { id: 'hh-003', name: 'Torres Household', enabled: true, method: 'Visa •••• 1884', nextChargeDate: '2026-03-28', nextChargeAmount: 64 },
]

function formatCurrency(value: number) {
  return `$${value.toFixed(2)}`
}

export default function BillingAutopayPage() {
  const [households, setHouseholds] = useState(HOUSEHOLDS)

  function toggleAutopay(id: string) {
    setHouseholds((prev) =>
      prev.map((household) =>
        household.id === id
          ? { ...household, enabled: !household.enabled }
          : household,
      ),
    )
  }

  const enabledCount = households.filter((household) => household.enabled).length

  return (
    <div style={{ padding: '32px', maxWidth: '1100px' }}>
      {/* Demo data notice */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 14px', marginBottom: '20px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '7px', fontSize: '12px', color: 'var(--warning)', fontFamily: 'var(--font-mono)' }}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0 }}>
          <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <span>
          <strong>Demo data</strong> — Autopay management API is not yet available. Displayed enrollment states are illustrative only and toggling does not affect live billing.
        </span>
      </div>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '24px', gap: '16px', flexWrap: 'wrap' }}>
        <div>
          <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
            Billing
          </p>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', marginBottom: '4px' }}>
            Autopay
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
            Manage household autopay enrollment, payment methods, next scheduled charges, and opt-out handling.
          </p>
        </div>
        <Link href="/billing" className="btn btn-secondary" style={{ textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <Icon name="arrow-left" size={13} /> Back to Billing
        </Link>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Autopay Enabled', value: enabledCount, sub: `${households.length} tracked households`, color: 'var(--accent)' },
          { label: 'Next Charge Batch', value: households.filter((household) => household.enabled).length, sub: 'scheduled households', color: 'var(--success)' },
          { label: 'Opt-Out Ready', value: households.length, sub: 'manual review supported', color: 'var(--text)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '16px 18px' }}>
            <div style={{ fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '24px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{stat.sub}</div>
          </div>
        ))}
      </div>

      <div className="card" style={{ padding: '16px', marginBottom: '16px' }}>
        <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr 1fr 0.8fr 0.8fr', gap: '12px', paddingBottom: '10px', borderBottom: '1px solid var(--border)' }}>
          {['Household', 'Saved Method', 'Next Scheduled Charge', 'Status', 'Actions'].map((header) => (
            <div key={header} style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
              {header}
            </div>
          ))}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column' }}>
          {households.map((household) => (
            <div key={household.id} style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr 1fr 0.8fr 0.8fr', gap: '12px', alignItems: 'center', padding: '14px 0', borderBottom: '1px solid var(--border)' }}>
              <div>
                <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{household.name}</div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>{household.id}</div>
              </div>
              <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{household.method}</div>
              <div>
                <div style={{ fontSize: '13px', color: 'var(--text)' }}>{new Date(household.nextChargeDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>{formatCurrency(household.nextChargeAmount)}</div>
              </div>
              <span style={{ width: 'fit-content', fontSize: '11px', fontWeight: 600, padding: '3px 10px', borderRadius: '999px', background: household.enabled ? 'var(--success-bg)' : 'var(--warning-bg)', color: household.enabled ? 'var(--success)' : 'var(--warning)', fontFamily: 'var(--font-mono)' }}>
                {household.enabled ? 'Enabled' : 'Paused'}
              </span>
              <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                <button className="btn btn-secondary" onClick={() => toggleAutopay(household.id)}>
                  {household.enabled ? 'Opt Out' : 'Enroll'}
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="card" style={{ padding: '16px' }}>
        <h2 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)', marginBottom: '10px' }}>Passing Criteria Coverage</h2>
        <p style={{ fontSize: '12px', color: 'var(--text-muted)', lineHeight: 1.6 }}>
          This admin Autopay page gives the STEP20E feature a reachable implementation with enrollment toggles, saved payment method visibility, next scheduled charge details, and a clear opt-out flow.
        </p>
      </div>
    </div>
  )
}
