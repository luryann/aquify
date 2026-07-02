'use client'

import { useState, useEffect, useRef } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchDocumentTypes,
  fetchDocumentSubmissions,
  fetchSwimmers,
  createDocumentSubmission,
  requestDocumentUploadUrl,
  attachDocumentFile,
  type DocumentTypeRecord,
  type DocumentSubmissionRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { getPortalDocumentAction, selectLatestSubmission } from '@/lib/document-submissions'

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtDate(iso?: string | null): string {
  if (!iso) return ''
  return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function daysUntil(iso?: string | null): number | null {
  if (!iso) return null
  const diff = new Date(iso).getTime() - Date.now()
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

// ── Types ─────────────────────────────────────────────────────────────────────

type DocStatus = 'verified' | 'submitted' | 'pending' | 'rejected' | 'expired' | 'missing'

const STATUS_CONFIG: Record<DocStatus, {
  label: string; bg: string; color: string; icon: 'check-circle' | 'alert-circle' | 'clock' | 'upload' | 'x-circle'
}> = {
  verified:  { label: 'Verified',        bg: '#ecfdf5', color: '#059669', icon: 'check-circle' },
  submitted: { label: 'Under Review',    bg: '#fffbeb', color: '#d97706', icon: 'clock' },
  pending:   { label: 'Action Needed',   bg: '#fffbeb', color: '#d97706', icon: 'alert-circle' },
  rejected:  { label: 'Resubmit',        bg: '#fef2f2', color: '#dc2626', icon: 'x-circle' },
  expired:   { label: 'Expired',         bg: '#fef2f2', color: '#dc2626', icon: 'alert-circle' },
  missing:   { label: 'Upload Required', bg: '#fef2f2', color: '#dc2626', icon: 'upload' },
}

const CATEGORY_LABELS: Record<string, string> = {
  waiver:       'Waivers',
  medical:      'Medical',
  insurance:    'Insurance',
  release:      'Releases',
  registration: 'Registrations',
  other:        'Other',
}

// Merge document types with their current submission for a given household
interface MergedDoc {
  /** Unique React key: `${docTypeId}::${swimmerId}` for swimmer docs, docTypeId otherwise */
  typeId: string
  /** Actual document type UUID for API calls */
  rawTypeId: string
  /** Swimmer UUID for swimmer-scoped docs; empty string otherwise */
  swimmerId: string
  typeName: string
  typeCategory: string
  typeDescription: string
  isRequired: boolean
  appliesTo: string
  expiryMonths: number | null
  swimmerLabel: string
  submission: DocumentSubmissionRecord | null
  effectiveStatus: DocStatus
  expiresAt: string | null
  submittedAt: string | null
}

function resolveStatus(sub: DocumentSubmissionRecord | null): DocStatus {
  if (!sub) return 'missing'
  if (sub.expires_at && new Date(sub.expires_at) < new Date() && sub.status !== 'verified') {
    return 'expired'
  }
  return (sub.status as DocStatus) || 'pending'
}

function mergeDocuments(
  types: DocumentTypeRecord[],
  submissions: DocumentSubmissionRecord[],
  swimmers: Array<{ id: string; first_name: string; last_name: string }>,
): MergedDoc[] {
  const docs: MergedDoc[] = []

  for (const dt of types) {
    if (!dt.is_active) continue

    if (dt.applies_to === 'swimmer') {
      // Generate one row per swimmer so multi-swimmer households are not collapsed
      if (swimmers.length === 0) {
        // No swimmers fetched yet — show a single placeholder row
        const sub = selectLatestSubmission(submissions, dt.id)
        docs.push({
          typeId: dt.id,
          rawTypeId: dt.id,
          swimmerId: '',
          typeName: dt.name,
          typeCategory: dt.category,
          typeDescription: dt.description ?? '',
          isRequired: dt.is_required,
          appliesTo: dt.applies_to,
          expiryMonths: dt.expiry_months ?? null,
          swimmerLabel: 'All Swimmers',
          submission: sub,
          effectiveStatus: resolveStatus(sub),
          expiresAt: sub?.expires_at ?? null,
          submittedAt: sub?.submitted_at ?? null,
        })
      } else {
        for (const sw of swimmers) {
          const sub = selectLatestSubmission(submissions, dt.id, sw.id)
          docs.push({
            typeId: `${dt.id}::${sw.id}`,
            rawTypeId: dt.id,
            swimmerId: sw.id,
            typeName: dt.name,
            typeCategory: dt.category,
            typeDescription: dt.description ?? '',
            isRequired: dt.is_required,
            appliesTo: dt.applies_to,
            expiryMonths: dt.expiry_months ?? null,
            swimmerLabel: `${sw.first_name} ${sw.last_name}`,
            submission: sub,
            effectiveStatus: resolveStatus(sub),
            expiresAt: sub?.expires_at ?? null,
            submittedAt: sub?.submitted_at ?? null,
          })
        }
      }
    } else {
      // household or all — one row per type
      const sub = selectLatestSubmission(submissions, dt.id)
      docs.push({
        typeId: dt.id,
        rawTypeId: dt.id,
        swimmerId: '',
        typeName: dt.name,
        typeCategory: dt.category,
        typeDescription: dt.description ?? '',
        isRequired: dt.is_required,
        appliesTo: dt.applies_to,
        expiryMonths: dt.expiry_months ?? null,
        swimmerLabel: dt.applies_to === 'household' ? 'Household' : 'All Swimmers',
        submission: sub,
        effectiveStatus: resolveStatus(sub),
        expiresAt: sub?.expires_at ?? null,
        submittedAt: sub?.submitted_at ?? null,
      })
    }
  }

  return docs
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function DocumentsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const uploadInputRef = useRef<HTMLInputElement>(null)

  const [docTypes, setDocTypes] = useState<DocumentTypeRecord[]>([])
  const [submissions, setSubmissions] = useState<DocumentSubmissionRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [uploadingId, setUploadingId] = useState<string | null>(null)
  const [uploadError, setUploadError] = useState<string | null>(null)
  const [pendingUploadTarget, setPendingUploadTarget] = useState<{ typeId: string; swimmerId?: string } | null>(null)
  const [filter, setFilter] = useState<'all' | 'pending'>('all')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoading(false)
      setLoadError('Document portal is unavailable because household context is missing.')
      return
    }
    let cancelled = false

    async function run() {
      try {
        setLoading(true)
        setLoadError(null)
        const [types, subs, allSwimmers] = await Promise.all([
          fetchDocumentTypes(api, orgId, true),
          fetchDocumentSubmissions(api, orgId, householdId),
          fetchSwimmers(api, orgId),
        ])
        if (cancelled) return
        setDocTypes(types)
        setSubmissions(subs)
        // Only keep swimmers that belong to this household
        setSwimmers(allSwimmers.filter(s => s.household_id === householdId))
      } catch (error) {
        if (!cancelled) {
          setDocTypes([])
          setSubmissions([])
          setSwimmers([])
          setLoadError(reportUIError(
            'portal-documents-load',
            error,
            'Document data is unavailable right now.',
            { orgId, householdId },
          ))
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => { cancelled = true }
  }, [api, orgId, householdId, reloadKey])

  const merged = mergeDocuments(docTypes, submissions, swimmers)
  const pendingDocs = merged.filter(d => d.effectiveStatus === 'missing' || d.effectiveStatus === 'rejected' || d.effectiveStatus === 'expired')
  const visibleDocs = filter === 'pending' ? pendingDocs : merged

  const categories = [...new Set(visibleDocs.map(d => d.typeCategory))]

  const completedCount = merged.filter(d => d.effectiveStatus === 'verified' || d.effectiveStatus === 'submitted').length
  const expiringCount = merged.filter(d => {
    const days = daysUntil(d.expiresAt)
    return days !== null && days >= 0 && days <= 30
  }).length

  const handleUploadClick = (typeId: string, swimmerId?: string) => {
    setPendingUploadTarget({ typeId, swimmerId })
    uploadInputRef.current?.click()
  }

  const handleFileSelected = async (file: File | null) => {
    if (!file || !pendingUploadTarget) return
    const { typeId, swimmerId } = pendingUploadTarget
    if (!orgId || !householdId || uploadingId) return
    setUploadError(null)
    setUploadingId(swimmerId ? `${typeId}::${swimmerId}` : typeId)
    try {
      const existingSub = selectLatestSubmission(submissions, typeId, swimmerId)
      let subId = existingSub?.id
      if (!subId) {
        const newSub = await createDocumentSubmission(api, orgId, {
          document_type_id: typeId,
          household_id: householdId,
          ...(swimmerId ? { swimmer_id: swimmerId } : {}),
        })
        subId = newSub.id
      }
      const mimeType = file.type || 'application/octet-stream'
      const upload = await requestDocumentUploadUrl(api, orgId, subId, {
        file_name: file.name,
        file_mime: mimeType,
        file_size_bytes: file.size,
      })
      const uploadRes = await fetch(upload.upload_url, {
        method: 'PUT',
        body: file,
        headers: { 'Content-Type': mimeType },
      })
      if (!uploadRes.ok) {
        throw new Error(`upload failed: ${uploadRes.status}`)
      }
      await attachDocumentFile(api, orgId, subId, {
        file_object_key: upload.object_key,
        file_name: file.name,
        file_mime: mimeType,
        file_size_bytes: file.size,
      })
      const subs = await fetchDocumentSubmissions(api, orgId, householdId)
      setSubmissions(subs)
    } catch {
      setUploadError('Upload failed. Please try again.')
    } finally {
      setUploadingId(null)
      setPendingUploadTarget(null)
    }
  }

  if (loading) {
    return (
      <div>
        <div style={{ marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: '#1A1714', margin: '0 0 6px', lineHeight: 1.1 }}>Documents</h1>
          <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>Loading your household documents…</p>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {[1, 2, 3].map(i => (
            <div key={i} style={{ height: '68px', borderRadius: '12px', background: '#f5f4f3', border: '1px solid rgba(0,0,0,0.06)' }} />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div>
      {/* Page header */}
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.3s ease both' }}>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', color: '#1A1714', margin: '0 0 6px', lineHeight: 1.1 }}>
          Documents
        </h1>
        <p style={{ margin: 0, fontSize: '14px', color: '#9A9693' }}>
          Required forms, waivers, and certifications for your household.
        </p>
      </div>

      {loadError && (
        <div style={{ padding: '12px 16px', background: '#fff7ed', border: '1px solid #fdba74', borderRadius: '10px', marginBottom: '16px', fontSize: '13px', color: '#9a3412', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px' }}>
          <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={15} style={{ color: '#ea580c', marginTop: '1px', flexShrink: 0 }} />
            <span>{loadError}</span>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: '#ffffff', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: '#9a3412', fontSize: '12px', fontWeight: 600, padding: '8px 14px', whiteSpace: 'nowrap' }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Upload error */}
      {uploadError && (
        <div style={{ padding: '12px 16px', background: 'rgba(220,38,38,0.07)', border: '1px solid rgba(220,38,38,0.18)', borderRadius: '10px', marginBottom: '16px', fontSize: '13px', color: '#b91c1c', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{uploadError}</span>
          <button onClick={() => setUploadError(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#b91c1c', fontSize: '16px', lineHeight: 1, padding: 0 }}>✕</button>
        </div>
      )}

      <input
        ref={uploadInputRef}
        type="file"
        accept=".pdf,.png,.jpg,.jpeg,.doc,.docx"
        style={{ display: 'none' }}
        onChange={(e) => {
          const file = e.target.files?.[0] ?? null
          e.currentTarget.value = ''
          void handleFileSelected(file)
        }}
      />

      {/* Action needed alert */}
      {!loadError && pendingDocs.length > 0 && (
        <div style={{ background: 'linear-gradient(135deg, #fef2f2 0%, #fff7f5 100%)', border: '1px solid rgba(220,38,38,0.18)', borderRadius: '14px', padding: '18px 22px', marginBottom: '24px', animation: 'slideUp 0.35s ease both' }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '14px' }}>
            <div style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'rgba(220,38,38,0.10)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <Icon name="alert-circle" size={20} style={{ color: '#dc2626' }} />
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: '15px', fontWeight: 700, color: '#991b1b', marginBottom: '4px' }}>
                {pendingDocs.length} document{pendingDocs.length !== 1 ? 's' : ''} require immediate action
              </div>
              <ul style={{ margin: 0, padding: '0 0 0 16px', fontSize: '13px', color: '#b91c1c', lineHeight: 1.7 }}>
                {pendingDocs.map(doc => (
                  <li key={doc.typeId}>
                    <strong>{doc.typeName}</strong>
                    {doc.expiresAt && daysUntil(doc.expiresAt) !== null && (daysUntil(doc.expiresAt) ?? 99) <= 14
                      ? ` — due by ${fmtDate(doc.expiresAt)}`
                      : ''}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '24px', animation: 'slideUp 0.4s ease both' }}>
        {[
          { label: 'Complete',      value: completedCount,      total: merged.length, color: '#059669' },
          { label: 'Action Needed', value: pendingDocs.length,  total: merged.length, color: '#dc2626' },
          { label: 'Expiring Soon', value: expiringCount,        total: merged.length, color: '#d97706' },
        ].map(stat => (
          <div key={stat.label} style={{ background: '#ffffff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px', boxShadow: '0 1px 4px rgba(0,0,0,0.04)' }}>
            <div style={{ fontSize: '22px', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-mono)' }}>
              {stat.value}<span style={{ fontSize: '14px', color: '#C0BDBB', fontWeight: 400 }}>/{stat.total}</span>
            </div>
            <div style={{ fontSize: '12px', color: '#9A9693', marginTop: '4px' }}>{stat.label}</div>
          </div>
        ))}
      </div>

      {/* Filter tabs */}
      <div style={{ display: 'flex', gap: '2px', marginBottom: '20px', background: 'rgba(0,0,0,0.04)', borderRadius: '10px', padding: '4px', maxWidth: '320px', animation: 'slideUp 0.42s ease both' }}>
        {[{ key: 'all', label: 'All Documents' }, { key: 'pending', label: `Needs Action (${pendingDocs.length})` }].map(({ key, label }) => (
          <button key={key} onClick={() => setFilter(key as 'all' | 'pending')} style={{ flex: 1, padding: '7px 12px', borderRadius: '7px', border: 'none', background: filter === key ? '#ffffff' : 'transparent', color: filter === key ? '#1A1714' : '#6B6866', fontSize: '13px', fontWeight: filter === key ? 600 : 450, cursor: 'pointer', boxShadow: filter === key ? '0 1px 4px rgba(0,0,0,0.08)' : 'none', transition: 'all 120ms ease', fontFamily: 'var(--font-body)', whiteSpace: 'nowrap' }}>
            {label}
          </button>
        ))}
      </div>

      {/* Empty state */}
      {shouldShowCollectionEmptyState(loading, loadError, merged.length) && (
        <div style={{ textAlign: 'center', padding: '60px 20px', background: '#ffffff', borderRadius: '16px', border: '1px solid rgba(0,0,0,0.07)' }}>
          <div style={{ fontSize: '32px', marginBottom: '12px' }}>📄</div>
          <div style={{ fontFamily: 'var(--font-display)', fontSize: '20px', color: '#1A1714', marginBottom: '6px' }}>No documents required yet</div>
          <div style={{ fontSize: '14px', color: '#9A9693' }}>Your coach will request documents when needed.</div>
        </div>
      )}

      {/* Documents by category */}
      {!loadError ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
        {categories.map((cat, catIdx) => {
          const catDocs = visibleDocs.filter(d => d.typeCategory === cat)
          return (
            <section key={cat} style={{ animation: `slideUp ${0.45 + catIdx * 0.06}s ease both` }}>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#9A9693', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '12px' }}>
                {CATEGORY_LABELS[cat] ?? cat}
              </div>

              <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.07)', boxShadow: '0 1px 6px rgba(0,0,0,0.04)', overflow: 'hidden' }}>
                {catDocs.map((doc, i) => {
                  const statusCfg = STATUS_CONFIG[doc.effectiveStatus]
                  const isUploading = uploadingId === doc.typeId
                  const needsAction = doc.effectiveStatus === 'missing' || doc.effectiveStatus === 'rejected' || doc.effectiveStatus === 'expired'
                  const actionKind = getPortalDocumentAction({
                    effectiveStatus: doc.effectiveStatus,
                    typeCategory: doc.typeCategory,
                  })
                  const days = daysUntil(doc.expiresAt)
                  const expiringUrgent = days !== null && days >= 0 && days <= 14

                  return (
                    <div key={doc.typeId} style={{ display: 'flex', gap: '16px', padding: '18px 20px', borderBottom: i < catDocs.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', alignItems: 'flex-start', background: needsAction ? 'rgba(220,38,38,0.01)' : 'transparent' }}>
                      {/* Status icon */}
                      <div style={{ width: '36px', height: '36px', borderRadius: '9px', background: statusCfg.bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <Icon name={statusCfg.icon} size={18} style={{ color: statusCfg.color }} />
                      </div>

                      {/* Content */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'flex-start', gap: '8px', flexWrap: 'wrap', marginBottom: '4px' }}>
                          <span style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714', lineHeight: 1.3 }}>{doc.typeName}</span>
                          {doc.isRequired && (
                            <span style={{ padding: '1px 6px', background: 'rgba(0,0,0,0.05)', borderRadius: '4px', fontSize: '10.5px', color: '#9A9693', fontWeight: 500, whiteSpace: 'nowrap' }}>Required</span>
                          )}
                          {doc.submission?.review_notes && (doc.effectiveStatus === 'rejected') && (
                            <span style={{ padding: '1px 8px', background: 'rgba(220,38,38,0.08)', borderRadius: '4px', fontSize: '10.5px', color: '#dc2626', fontWeight: 500 }}>
                              {doc.submission.review_notes.slice(0, 60)}
                            </span>
                          )}
                        </div>
                        {doc.typeDescription && (
                          <div style={{ fontSize: '12.5px', color: '#9A9693', lineHeight: 1.4, marginBottom: '6px' }}>{doc.typeDescription}</div>
                        )}
                        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', fontSize: '11.5px', color: '#C0BDBB' }}>
                          <span>For: <span style={{ color: '#6B6866' }}>{doc.swimmerLabel}</span></span>
                          {doc.submittedAt && <span>Submitted: <span style={{ color: '#6B6866' }}>{fmtDate(doc.submittedAt)}</span></span>}
                          {doc.expiresAt && (
                            <span>
                              Expires:{' '}
                              <span style={{ color: expiringUrgent ? '#d97706' : '#6B6866', fontWeight: expiringUrgent ? 600 : 400 }}>
                                {fmtDate(doc.expiresAt)}
                                {expiringUrgent && days !== null && (
                                  <span style={{ marginLeft: '4px', color: '#d97706' }}>({days}d)</span>
                                )}
                              </span>
                            </span>
                          )}
                        </div>
                      </div>

                      {/* Action area */}
                      <div style={{ flexShrink: 0, display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '6px' }}>
                        <span style={{ padding: '3px 9px', borderRadius: '999px', fontSize: '11.5px', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, whiteSpace: 'nowrap' }}>
                          {statusCfg.label}
                        </span>

                        {actionKind === 'upload' && needsAction && (
                          <button
                            onClick={() => handleUploadClick(doc.rawTypeId, doc.swimmerId || undefined)}
                            disabled={isUploading}
                            style={{ display: 'flex', alignItems: 'center', gap: '5px', padding: '6px 14px', background: isUploading ? 'rgba(235,93,30,0.5)' : '#eb5d1e', color: '#fff', border: 'none', borderRadius: '7px', fontSize: '12.5px', fontWeight: 600, cursor: isUploading ? 'wait' : 'pointer', fontFamily: 'var(--font-body)' }}
                          >
                            <Icon name="upload" size={12} />
                            {isUploading ? 'Uploading…' : 'Upload File'}
                          </button>
                        )}

                        {(doc.effectiveStatus === 'verified' || doc.effectiveStatus === 'submitted') && doc.submission?.file_name && (
                          <span style={{ fontSize: '12px', color: '#9A9693', fontFamily: 'var(--font-body)' }}>
                            Uploaded: {doc.submission.file_name}
                          </span>
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
      ) : (
        <div style={{ textAlign: 'center', padding: '40px 20px', background: '#ffffff', borderRadius: '16px', border: '1px solid rgba(0,0,0,0.07)', color: '#9A9693', fontSize: '14px' }}>
          Document requirements will appear here after a successful reload.
        </div>
      )}

      {/* Help notice */}
      <div style={{ marginTop: '28px', padding: '14px 18px', background: 'rgba(0,0,0,0.03)', borderRadius: '10px', fontSize: '13px', color: '#9A9693', lineHeight: 1.5, animation: 'slideUp 0.7s ease both', display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
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
