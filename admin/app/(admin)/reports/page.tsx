'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type ReportFormat = 'CSV' | 'PDF' | 'XLSX' | 'JSON'
type GeneratedStatus = 'ready' | 'generating' | 'expired' | 'failed'

type ReportDefinition = {
  id: string
  name: string
  description: string
  category: string
  formats: ReportFormat[]
  allowedRoles: string[]
  estimatedRows: string
}

type GeneratedReport = {
  id: string
  reportId: string
  reportName: string
  format: ReportFormat
  status: GeneratedStatus
  fileSizeKb: number | null
  generatedBy: string
  generatedAt: string
  expiresAt: string | null
  downloadUrl: string | null
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const REPORT_DEFINITIONS: ReportDefinition[] = [
  {
    id: 'billing-summary',
    name: 'Billing Summary',
    description: 'Invoices, payments, and outstanding balances by household for a given date range. Includes total collected, outstanding, and overdue amounts.',
    category: 'Billing',
    formats: ['CSV', 'XLSX', 'PDF'],
    allowedRoles: ['owner', 'admin', 'billing'],
    estimatedRows: '50–200',
  },
  {
    id: 'ledger-export',
    name: 'Ledger Export',
    description: 'Full append-only ledger export with all financial events. Useful for reconciliation and tax reporting.',
    category: 'Billing',
    formats: ['CSV', 'XLSX'],
    allowedRoles: ['owner', 'admin', 'billing'],
    estimatedRows: '100–500',
  },
  {
    id: 'member-roster',
    name: 'Member Roster',
    description: 'Full list of active swimmers and their household contacts. Includes group assignments, registration status, and emergency contacts.',
    category: 'Members',
    formats: ['CSV', 'XLSX', 'PDF'],
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    estimatedRows: '80–300',
  },
  {
    id: 'registration-summary',
    name: 'Season Registration Summary',
    description: 'Registration counts and waiver completion rates by group and season. Useful for board reporting and planning.',
    category: 'Members',
    formats: ['CSV', 'PDF'],
    allowedRoles: ['owner', 'admin', 'read-only'],
    estimatedRows: '20–50',
  },
  {
    id: 'meet-results',
    name: 'Meet Results Export',
    description: 'Individual event results for a specific meet or date range. Supports filtering by group, event, and swimmer.',
    category: 'Competition',
    formats: ['CSV', 'XLSX'],
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    estimatedRows: '100–2000',
  },
  {
    id: 'best-times',
    name: 'Best Times Report',
    description: 'Personal best times for all active swimmers across all events, with optional previous-season comparison.',
    category: 'Competition',
    formats: ['CSV', 'PDF'],
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    estimatedRows: '300–1000',
  },
  {
    id: 'time-standards',
    name: 'Time Standards Export',
    description: 'All configured time standard cuts and qualification counts per event. Useful for meet preparation.',
    category: 'Competition',
    formats: ['CSV', 'PDF'],
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    estimatedRows: '50–200',
  },
  {
    id: 'audit-log-export',
    name: 'Audit Log Export',
    description: 'Filtered export of audit events for compliance and governance review. Supports all audit log filters.',
    category: 'Compliance',
    formats: ['CSV', 'JSON'],
    allowedRoles: ['owner', 'admin'],
    estimatedRows: '50–500',
  },
  {
    id: 'suppression-list',
    name: 'Email Suppression List',
    description: 'Current email suppression list (opt-outs, bounces). Required for CAN-SPAM / CASL compliance review.',
    category: 'Compliance',
    formats: ['CSV'],
    allowedRoles: ['owner', 'admin'],
    estimatedRows: '0–50',
  },
]

const MOCK_GENERATED: GeneratedReport[] = [
  {
    id: 'gr-001', reportId: 'billing-summary', reportName: 'Billing Summary',
    format: 'XLSX', status: 'ready', fileSizeKb: 48,
    generatedBy: 'Admin User', generatedAt: '2026-03-15T09:00:00Z',
    expiresAt: '2026-03-22T09:00:00Z', downloadUrl: '/reports/gr-001/download',
  },
  {
    id: 'gr-002', reportId: 'member-roster', reportName: 'Member Roster',
    format: 'CSV', status: 'ready', fileSizeKb: 22,
    generatedBy: 'Coach Kim', generatedAt: '2026-03-14T14:30:00Z',
    expiresAt: '2026-03-21T14:30:00Z', downloadUrl: '/reports/gr-002/download',
  },
  {
    id: 'gr-003', reportId: 'meet-results', reportName: 'Meet Results Export',
    format: 'CSV', status: 'ready', fileSizeKb: 134,
    generatedBy: 'Coach Martinez', generatedAt: '2026-03-13T16:00:00Z',
    expiresAt: '2026-03-20T16:00:00Z', downloadUrl: '/reports/gr-003/download',
  },
  {
    id: 'gr-004', reportId: 'audit-log-export', reportName: 'Audit Log Export',
    format: 'JSON', status: 'expired', fileSizeKb: null,
    generatedBy: 'Owner', generatedAt: '2026-03-08T10:00:00Z',
    expiresAt: '2026-03-15T10:00:00Z', downloadUrl: null,
  },
  {
    id: 'gr-005', reportId: 'best-times', reportName: 'Best Times Report',
    format: 'PDF', status: 'ready', fileSizeKb: 210,
    generatedBy: 'Admin User', generatedAt: '2026-03-12T08:00:00Z',
    expiresAt: '2026-03-19T08:00:00Z', downloadUrl: '/reports/gr-005/download',
  },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function isExpiringSoon(iso: string): boolean {
  return new Date(iso).getTime() - Date.now() < 2 * 24 * 60 * 60 * 1000
}

const FORMAT_COLORS: Record<ReportFormat, string> = {
  CSV:  'var(--success)',
  XLSX: 'var(--info)',
  PDF:  'var(--warning)',
  JSON: 'var(--accent)',
}

const FORMAT_BG: Record<ReportFormat, string> = {
  CSV:  'var(--success-bg)',
  XLSX: 'var(--info-bg)',
  PDF:  'var(--warning-bg)',
  JSON: 'var(--accent-subtle)',
}

const FORMAT_BORDER: Record<ReportFormat, string> = {
  CSV:  'var(--success-border)',
  XLSX: 'var(--info-border)',
  PDF:  'var(--warning-border)',
  JSON: 'var(--accent-border)',
}

const CATEGORIES = ['All', 'Billing', 'Members', 'Competition', 'Compliance']

const STATUS_COLORS: Record<GeneratedStatus, string> = {
  ready:      'var(--success)',
  generating: 'var(--info)',
  expired:    'var(--text-muted)',
  failed:     'var(--error)',
}

// ─── Components ───────────────────────────────────────────────────────────────

function FormatBadge({ format }: { format: ReportFormat }) {
  return (
    <span style={{
      padding: '2px 6px', borderRadius: 3,
      fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700,
      color: FORMAT_COLORS[format],
      background: FORMAT_BG[format],
      border: `1px solid ${FORMAT_BORDER[format]}`,
    }}>
      {format}
    </span>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ReportsPage() {
  const [category, setCategory]       = useState('All')
  const [generating, setGenerating]   = useState<string | null>(null)
  const [generatedReports, setGeneratedReports] = useState(MOCK_GENERATED)
  const [selectedFormats, setSelectedFormats]   = useState<Record<string, ReportFormat>>({})

  const definitions = REPORT_DEFINITIONS.filter(r =>
    category === 'All' || r.category === category
  )

  function getSelectedFormat(def: ReportDefinition): ReportFormat {
    return selectedFormats[def.id] ?? def.formats[0]
  }

  function handleGenerate(defId: string) {
    const def = REPORT_DEFINITIONS.find(d => d.id === defId)
    if (!def) return
    setGenerating(defId)
    setTimeout(() => {
      const newReport: GeneratedReport = {
        id: 'gr-new-' + Date.now(),
        reportId: defId,
        reportName: def.name,
        format: getSelectedFormat(def),
        status: 'ready',
        fileSizeKb: Math.floor(Math.random() * 200) + 20,
        generatedBy: 'Admin User',
        generatedAt: new Date().toISOString(),
        expiresAt: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
        downloadUrl: '/reports/new/download',
      }
      setGeneratedReports(prev => [newReport, ...prev])
      setGenerating(null)
    }, 2200)
  }

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1200, margin: '0 auto' }}>

      {/* Header */}
      <div style={{ marginBottom: 28 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
          <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Icon name="bar-chart-2" size={16} color="var(--accent)" />
          </div>
          <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>
            Reports
          </h1>
        </div>
        <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>
          Generate and download data exports. Reports are available for 7 days after generation.
        </p>
      </div>

      {/* Category filter */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 24, flexWrap: 'wrap' }}>
        {CATEGORIES.map(cat => (
          <button
            key={cat}
            onClick={() => setCategory(cat)}
            style={{
              padding: '6px 14px', borderRadius: 6,
              background: category === cat ? 'var(--accent)' : 'var(--bg-elevated)',
              border: category === cat ? 'none' : '1px solid var(--border)',
              color: category === cat ? '#fff' : 'var(--text-subtle)',
              fontSize: 13, fontWeight: category === cat ? 600 : 400, cursor: 'pointer',
              transition: 'all 150ms',
            }}
          >
            {cat}
          </button>
        ))}
      </div>

      {/* Report definitions grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))', gap: 14, marginBottom: 40 }}>
        {definitions.map(def => (
          <div key={def.id} style={{
            padding: '20px', borderRadius: 10,
            background: 'var(--bg-elevated)', border: '1px solid var(--border)',
            display: 'flex', flexDirection: 'column', gap: 12,
          }}>
            <div>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8, marginBottom: 6 }}>
                <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700, color: 'var(--text)', lineHeight: 1.3 }}>
                  {def.name}
                </h3>
                <span style={{
                  flexShrink: 0, padding: '2px 7px', borderRadius: 4,
                  fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em',
                  color: 'var(--text-muted)', background: 'var(--bg)', border: '1px solid var(--border)',
                }}>
                  {def.category}
                </span>
              </div>
              <p style={{ margin: 0, fontSize: 12, color: 'var(--text-muted)', lineHeight: 1.6 }}>
                {def.description}
              </p>
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Formats:</span>
              {def.formats.map(f => <FormatBadge key={f} format={f} />)}
              <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--text-muted)' }}>
                ~{def.estimatedRows} rows
              </span>
            </div>

            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <button
                onClick={() => handleGenerate(def.id)}
                disabled={generating === def.id}
                style={{
                  flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
                  padding: '8px 14px', borderRadius: 7,
                  background: generating === def.id ? 'var(--accent-border)' : 'var(--accent)',
                  border: 'none', color: '#fff', fontSize: 12, fontWeight: 600,
                  cursor: generating === def.id ? 'not-allowed' : 'pointer',
                  transition: 'background 200ms',
                }}
              >
                {generating === def.id ? (
                  <>
                    <Icon name="refresh-cw" size={13} /> Generating…
                  </>
                ) : (
                  <>
                    <Icon name="download" size={13} /> Generate {getSelectedFormat(def)}
                  </>
                )}
              </button>
              {def.formats.length > 1 && (
                <select
                  value={getSelectedFormat(def)}
                  onChange={e => setSelectedFormats(prev => ({ ...prev, [def.id]: e.target.value as ReportFormat }))}
                  disabled={generating === def.id}
                  style={{
                    padding: '8px 10px', borderRadius: 7,
                    background: 'var(--bg)', border: '1px solid var(--border)',
                    color: 'var(--text-muted)', fontSize: 12, cursor: 'pointer',
                  }}
                >
                  {def.formats.map(f => <option key={f} value={f}>{f}</option>)}
                </select>
              )}
            </div>
          </div>
        ))}
      </div>

      {/* Generated reports history */}
      <div>
        <h2 style={{ fontSize: 16, fontWeight: 700, color: 'var(--text)', margin: '0 0 14px', letterSpacing: '-0.01em' }}>
          Generated Reports
        </h2>

        {generatedReports.length === 0 ? (
          <EmptyState icon="bar-chart-2" title="No reports generated yet" description="Click 'Generate' on any report type above to create your first export." />
        ) : (
          <div style={{
            borderRadius: 10, overflow: 'hidden',
            border: '1px solid var(--border)',
            background: 'var(--bg-elevated)',
          }}>
            {/* Header */}
            <div style={{
              display: 'grid', gridTemplateColumns: '1fr 80px 100px 140px 140px 120px',
              padding: '10px 16px', borderBottom: '1px solid var(--border)',
              background: 'var(--bg)',
            }}>
              {['Report', 'Format', 'Size', 'Generated By', 'Generated At', ''].map(h => (
                <span key={h} style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{h}</span>
              ))}
            </div>

            {generatedReports.map((r, i) => {
              const expiring = r.expiresAt && r.status === 'ready' && isExpiringSoon(r.expiresAt)
              return (
                <div key={r.id} style={{
                  display: 'grid', gridTemplateColumns: '1fr 80px 100px 140px 140px 120px',
                  padding: '12px 16px',
                  borderBottom: i < generatedReports.length - 1 ? '1px solid var(--border)' : 'none',
                  opacity: r.status === 'expired' ? 0.5 : 1,
                }}>
                  <div>
                    <div style={{ fontSize: 13, color: 'var(--text)', fontWeight: 500 }}>{r.reportName}</div>
                    {expiring && r.expiresAt && (
                      <div style={{ fontSize: 11, color: 'var(--warning)', marginTop: 2 }}>
                        Expires {formatDate(r.expiresAt)}
                      </div>
                    )}
                  </div>
                  <div style={{ alignSelf: 'center' }}>
                    <FormatBadge format={r.format} />
                  </div>
                  <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', alignSelf: 'center' }}>
                    {r.fileSizeKb ? `${r.fileSizeKb} KB` : '—'}
                  </span>
                  <span style={{ fontSize: 12, color: 'var(--text-muted)', alignSelf: 'center' }}>
                    {r.generatedBy}
                  </span>
                  <span style={{ fontSize: 12, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', alignSelf: 'center' }}>
                    {formatDate(r.generatedAt)}
                  </span>
                  <div style={{ alignSelf: 'center' }}>
                    {r.status === 'ready' && r.downloadUrl ? (
                      <a
                        href={r.downloadUrl}
                        style={{
                          display: 'inline-flex', alignItems: 'center', gap: 5,
                          padding: '5px 10px', borderRadius: 5,
                          background: 'var(--accent)', border: 'none',
                          color: '#fff', fontSize: 12, fontWeight: 600, cursor: 'pointer',
                          textDecoration: 'none',
                        }}
                      >
                        <Icon name="download" size={12} /> Download
                      </a>
                    ) : (
                      <span style={{ fontSize: 12, color: STATUS_COLORS[r.status], textTransform: 'capitalize' }}>
                        {r.status}
                      </span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
