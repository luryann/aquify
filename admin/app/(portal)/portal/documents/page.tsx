'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ── Stub data ──────────────────────────────────────────────────────────────
const DOCUMENTS = [
  {
    id: 'doc-001',
    name: 'Season Liability Waiver — Emma Chen',
    category: 'waiver' as const,
    required: true,
    status: 'signed' as const,
    signedDate: 'Sep 3, 2025',
    expiresDate: 'Aug 31, 2026',
    daysUntilExpiry: 168,
    swimmer: 'Emma Chen',
    description: 'Annual liability release required for all competitive team members.',
  },
  {
    id: 'doc-002',
    name: 'Season Liability Waiver — Noah Chen',
    category: 'waiver' as const,
    required: true,
    status: 'signed' as const,
    signedDate: 'Sep 5, 2025',
    expiresDate: 'Aug 31, 2026',
    daysUntilExpiry: 168,
    swimmer: 'Noah Chen',
    description: 'Annual liability release required for all competitive team members.',
  },
  {
    id: 'doc-003',
    name: 'Photo & Video Release',
    category: 'release' as const,
    required: false,
    status: 'signed' as const,
    signedDate: 'Sep 3, 2025',
    expiresDate: null,
    daysUntilExpiry: null,
    swimmer: 'All Swimmers',
    description: 'Grants permission to use photos and videos of your swimmer in club materials.',
  },
  {
    id: 'doc-004',
    name: 'Medical Authorization — Noah Chen',
    category: 'medical' as const,
    required: true,
    status: 'pending' as const,
    signedDate: null,
    expiresDate: null,
    daysUntilExpiry: null,
    swimmer: 'Noah Chen',
    description: 'Authorizes medical treatment if a parent/guardian cannot be reached during an emergency.',
  },
  {
    id: 'doc-005',
    name: 'Insurance Card — Emma Chen',
    category: 'insurance' as const,
    required: true,
    status: 'requested' as const,
    signedDate: null,
    expiresDate: 'Mar 25, 2026',
    daysUntilExpiry: 8,
    swimmer: 'Emma Chen',
    description: 'Copy of health insurance card required for meet participation.',
  },
  {
    id: 'doc-006',
    name: 'USS Registration — Emma Chen',
    category: 'registration' as const,
    required: true,
    status: 'submitted' as const,
    signedDate: 'Sep 3, 2025',
    expiresDate: 'Sep 1, 2026',
    daysUntilExpiry: 168,
    swimmer: 'Emma Chen',
    description: 'USA Swimming annual registration confirmation.',
  },
]

// ── Types & Config ───────────────────────────────────────────────────────────
type DocStatus = 'signed' | 'submitted' | 'pending' | 'requested' | 'expired'

const STATUS_CONFIG: Record<DocStatus, {
  label: string; bg: string; color: string; icon: 'check-circle' | 'alert-circle' | 'clock' | 'upload'
}> = {
  signed:    { label: 'Signed',      bg: '#ecfdf5', color: '#059669', icon: 'check-circle' },
  submitted: { label: 'Submitted',   bg: '#ecfdf5', color: '#059669', icon: 'check-circle' },
  pending:   { label: 'Action Needed', bg: '#fffbeb', color: '#d97706', icon: 'alert-circle' },
  requested: { label: 'Upload Required', bg: '#fef2f2', color: '#dc2626', icon: 'upload' },
  expired:   { label: 'Expired',     bg: '#fef2f2', color: '#dc2626', icon: 'alert-circle' },
}

const CATEGORY_LABELS: Record<string, string> = {
  waiver:       'Waivers',
  medical:      'Medical',
  insurance:    'Insurance',
  release:      'Releases',
  registration: 'Registrations',
}

// ── Component ────────────────────────────────────────────────────────────────
export default function DocumentsPage() {
  const [uploadingId, setUploadingId] = useState<string | null>(null)
  const [uploadedIds, setUploadedIds] = useState<Set<string>>(new Set())
  const [filter, setFilter] = useState<'all' | 'pending'>('all')

  const pendingDocs = DOCUMENTS.filter(
    (d) => d.status === 'pending' || d.status === 'requested'
  )
  const visibleDocs = filter === 'pending' ? pendingDocs : DOCUMENTS

  const categories = [...new Set(visibleDocs.map((d) => d.category))]

  const handleUpload = async (docId: string) => {
    setUploadingId(docId)
    await new Promise((r) => setTimeout(r, 1200))
    setUploadingId(null)
    setUploadedIds((prev) => new Set([...prev, docId]))
  }

  return (
    <div>
      {/* Page header */}
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '30px',
            color: '#1A1714',
            margin: '0 0 6px',
            lineHeight: 1.1,
          }}
        >
          Documents
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Required forms, waivers, and certifications for your household.
        </p>
      </div>

      {/* Action needed alert */}
      {pendingDocs.length > 0 && (
        <div
          style={{
            background: 'linear-gradient(135deg, #fef2f2 0%, #fff7f5 100%)',
            border: '1px solid rgba(220,38,38,0.18)',
            borderRadius: '14px',
            padding: '18px 22px',
            marginBottom: '24px',
            animation: 'slideUp 0.35s ease both',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '14px' }}>
            <div
              style={{
                width: '40px',
                height: '40px',
                borderRadius: '50%',
                background: 'rgba(220,38,38,0.10)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: 0,
              }}
            >
              <Icon name="alert-circle" size={20} style={{ color: '#dc2626' }} />
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: '15px', fontWeight: 700, color: '#991b1b', marginBottom: '4px' }}>
                {pendingDocs.length} document{pendingDocs.length !== 1 ? 's' : ''} require immediate action
              </div>
              <ul style={{ margin: '0', padding: '0 0 0 16px', fontSize: '13px', color: '#b91c1c', lineHeight: 1.7 }}>
                {pendingDocs.map((doc) => (
                  <li key={doc.id}>
                    <strong>{doc.name}</strong>
                    {doc.expiresDate && doc.daysUntilExpiry !== null && doc.daysUntilExpiry <= 14
                      ? ` — due by ${doc.expiresDate}`
                      : ''}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Stats strip */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(3, 1fr)',
          gap: '12px',
          marginBottom: '24px',
          animation: 'slideUp 0.4s ease both',
        }}
      >
        {[
          {
            label: 'Complete',
            value: DOCUMENTS.filter((d) => d.status === 'signed' || d.status === 'submitted').length,
            total: DOCUMENTS.length,
            color: '#059669',
          },
          {
            label: 'Action Needed',
            value: pendingDocs.length,
            total: DOCUMENTS.length,
            color: '#dc2626',
          },
          {
            label: 'Expiring Soon',
            value: DOCUMENTS.filter(
              (d) => d.daysUntilExpiry !== null && d.daysUntilExpiry <= 30
            ).length,
            total: DOCUMENTS.length,
            color: '#d97706',
          },
        ].map((stat) => (
          <div
            key={stat.label}
            style={{
              background: '#ffffff',
              borderRadius: '12px',
              border: '1px solid rgba(0,0,0,0.07)',
              padding: '16px 20px',
              boxShadow: '0 1px 4px rgba(0,0,0,0.04)',
            }}
          >
            <div style={{ fontSize: '22px', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-mono)' }}>
              {stat.value}
              <span style={{ fontSize: '14px', color: '#C0BDBB', fontWeight: 400 }}>
                /{stat.total}
              </span>
            </div>
            <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
          </div>
        ))}
      </div>

      {/* Filter tabs */}
      <div
        style={{
          display: 'flex',
          gap: '2px',
          marginBottom: '20px',
          background: 'rgba(0,0,0,0.04)',
          borderRadius: '10px',
          padding: '4px',
          maxWidth: '320px',
          animation: 'slideUp 0.42s ease both',
        }}
      >
        {[
          { key: 'all', label: 'All Documents' },
          { key: 'pending', label: `Needs Action (${pendingDocs.length})` },
        ].map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setFilter(key as 'all' | 'pending')}
            style={{
              flex: 1,
              padding: '7px 12px',
              borderRadius: '7px',
              border: 'none',
              background: filter === key ? '#ffffff' : 'transparent',
              color: filter === key ? '#1A1714' : '#6B6866',
              fontSize: '13px',
              fontWeight: filter === key ? 600 : 450,
              cursor: 'pointer',
              boxShadow: filter === key ? '0 1px 4px rgba(0,0,0,0.08)' : 'none',
              transition: 'all 120ms ease',
              fontFamily: 'var(--font-body)',
              whiteSpace: 'nowrap',
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Documents by category */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
        {categories.map((cat, catIdx) => {
          const catDocs = visibleDocs.filter((d) => d.category === cat)
          return (
            <section
              key={cat}
              style={{ animation: `slideUp ${0.45 + catIdx * 0.06}s ease both` }}
            >
              <div
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: '#9A9693',
                  textTransform: 'uppercase',
                  letterSpacing: '0.08em',
                  marginBottom: '12px',
                }}
              >
                {CATEGORY_LABELS[cat] ?? cat}
              </div>

              <div
                style={{
                  background: '#ffffff',
                  borderRadius: '14px',
                  border: '1px solid rgba(0,0,0,0.07)',
                  boxShadow: '0 1px 6px rgba(0,0,0,0.04)',
                  overflow: 'hidden',
                }}
              >
                {catDocs.map((doc, i) => {
                  const isUploaded = uploadedIds.has(doc.id)
                  const effectiveStatus: DocStatus = isUploaded ? 'submitted' : (doc.status as DocStatus)
                  const statusCfg = STATUS_CONFIG[effectiveStatus]
                  const isUploading = uploadingId === doc.id
                  const needsAction = effectiveStatus === 'pending' || effectiveStatus === 'requested'
                  const expiringUrgent =
                    doc.daysUntilExpiry !== null && doc.daysUntilExpiry <= 14

                  return (
                    <div
                      key={doc.id}
                      style={{
                        display: 'flex',
                        gap: '16px',
                        padding: '18px 20px',
                        borderBottom: i < catDocs.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        alignItems: 'flex-start',
                        background: needsAction ? 'rgba(220,38,38,0.01)' : 'transparent',
                      }}
                    >
                      {/* Status icon */}
                      <div
                        style={{
                          width: '36px',
                          height: '36px',
                          borderRadius: '9px',
                          background: statusCfg.bg,
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          flexShrink: 0,
                        }}
                      >
                        <Icon
                          name={statusCfg.icon}
                          size={18}
                          style={{ color: statusCfg.color }}
                        />
                      </div>

                      {/* Content */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'flex-start', gap: '8px', flexWrap: 'wrap', marginBottom: '4px' }}>
                          <span style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714', lineHeight: 1.3 }}>
                            {doc.name}
                          </span>
                          {doc.required && (
                            <span
                              style={{
                                padding: '1px 6px',
                                background: 'rgba(0,0,0,0.05)',
                                borderRadius: '4px',
                                fontSize: '10.5px',
                                color: '#9A9693',
                                fontWeight: 500,
                                whiteSpace: 'nowrap',
                              }}
                            >
                              Required
                            </span>
                          )}
                        </div>

                        <div style={{ fontSize: '12.5px', color: '#9A9693', lineHeight: 1.4, marginBottom: '6px' }}>
                          {doc.description}
                        </div>

                        {/* Metadata row */}
                        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', fontSize: '11.5px', color: '#C0BDBB' }}>
                          <span>
                            Swimmer: <span style={{ color: '#6B6866' }}>{doc.swimmer}</span>
                          </span>
                          {doc.signedDate && (
                            <span>
                              Signed: <span style={{ color: '#6B6866' }}>{doc.signedDate}</span>
                            </span>
                          )}
                          {doc.expiresDate && (
                            <span>
                              Expires:{' '}
                              <span
                                style={{
                                  color: expiringUrgent ? '#d97706' : '#6B6866',
                                  fontWeight: expiringUrgent ? 600 : 400,
                                }}
                              >
                                {doc.expiresDate}
                                {expiringUrgent && doc.daysUntilExpiry !== null && (
                                  <span style={{ marginLeft: '4px', color: '#d97706' }}>
                                    ({doc.daysUntilExpiry}d)
                                  </span>
                                )}
                              </span>
                            </span>
                          )}
                        </div>
                      </div>

                      {/* Action area */}
                      <div style={{ flexShrink: 0, display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '6px' }}>
                        <span
                          style={{
                            padding: '3px 9px',
                            borderRadius: '999px',
                            fontSize: '11.5px',
                            fontWeight: 600,
                            background: statusCfg.bg,
                            color: statusCfg.color,
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {statusCfg.label}
                        </span>

                        {needsAction && !isUploaded && (
                          doc.category === 'waiver' || doc.category === 'medical' ? (
                            <button
                              style={{
                                padding: '6px 14px',
                                background: '#eb5d1e',
                                color: '#fff',
                                border: 'none',
                                borderRadius: '7px',
                                fontSize: '12.5px',
                                fontWeight: 600,
                                cursor: 'pointer',
                                fontFamily: 'var(--font-body)',
                              }}
                            >
                              Sign Online →
                            </button>
                          ) : (
                            <button
                              onClick={() => handleUpload(doc.id)}
                              disabled={isUploading}
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '5px',
                                padding: '6px 14px',
                                background: isUploading ? 'rgba(235,93,30,0.5)' : '#eb5d1e',
                                color: '#fff',
                                border: 'none',
                                borderRadius: '7px',
                                fontSize: '12.5px',
                                fontWeight: 600,
                                cursor: isUploading ? 'wait' : 'pointer',
                                fontFamily: 'var(--font-body)',
                              }}
                            >
                              <Icon name="upload" size={12} />
                              {isUploading ? 'Uploading…' : 'Upload File'}
                            </button>
                          )
                        )}

                        {(effectiveStatus === 'signed' || effectiveStatus === 'submitted') && (
                          <button
                            style={{
                              background: 'none',
                              border: 'none',
                              cursor: 'pointer',
                              fontSize: '12px',
                              color: '#9A9693',
                              fontFamily: 'var(--font-body)',
                              padding: '0',
                            }}
                          >
                            View PDF
                          </button>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </section>
          )
        })}
      </div>

      {/* Help notice */}
      <div
        style={{
          marginTop: '28px',
          padding: '14px 18px',
          background: 'rgba(0,0,0,0.03)',
          borderRadius: '10px',
          fontSize: '13px',
          color: '#9A9693',
          lineHeight: 1.5,
          animation: 'slideUp 0.7s ease both',
          display: 'flex',
          gap: '10px',
          alignItems: 'flex-start',
        }}
      >
        <Icon name="info" size={14} style={{ color: '#C0BDBB', marginTop: '2px', flexShrink: 0 }} />
        <span>
          Missing or expired documents may affect meet participation eligibility. Contact{' '}
          <span style={{ color: '#eb5d1e', fontWeight: 500 }}>office@aquify.club</span>{' '}
          if you have questions about any document.
        </span>
      </div>

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
