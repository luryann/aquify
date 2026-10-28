'use client'

import Link from 'next/link'
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
  fetchRegistrations,
  createDocumentSubmission,
  requestDocumentUploadUrl,
  attachDocumentFile,
  type DocumentTypeRecord,
  type DocumentSubmissionRecord,
  type SwimmerRecord,
  type RegistrationRecord,
} from '@/lib/adminData'
import { getPortalDocumentAction, selectLatestSubmission } from '@/lib/document-submissions'
import { summarizeRegistrationWaiver } from '@/lib/portal-registration-status'
import { parseBackendDate } from '@/lib/dates'

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtDate(iso?: string | null): string {
  if (!iso) return ''
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
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
  verified:  { label: 'Verified',        bg: 'var(--success-bg)', color: 'var(--success)', icon: 'check-circle' },
  submitted: { label: 'Under review',    bg: 'var(--warning-bg)', color: 'var(--warning)', icon: 'clock' },
  pending:   { label: 'Action needed',   bg: 'var(--warning-bg)', color: 'var(--warning)', icon: 'alert-circle' },
  rejected:  { label: 'Resubmit',        bg: 'var(--error-bg)', color: 'var(--error)', icon: 'x-circle' },
  expired:   { label: 'Expired',         bg: 'var(--error-bg)', color: 'var(--error)', icon: 'alert-circle' },
  missing:   { label: 'Upload required', bg: 'var(--error-bg)', color: 'var(--error)', icon: 'upload' },
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
  const [registrations, setRegistrations] = useState<RegistrationRecord[]>([])
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
        const [types, subs, allSwimmers, allRegs] = await Promise.all([
          fetchDocumentTypes(api, orgId, true),
          fetchDocumentSubmissions(api, orgId, householdId),
          fetchSwimmers(api, orgId),
          fetchRegistrations(api, orgId),
        ])
        if (cancelled) return
        setDocTypes(types)
        setSubmissions(subs)
        // Only keep swimmers that belong to this household
        setSwimmers(allSwimmers.filter(s => s.household_id === householdId))
        setRegistrations(allRegs.filter(r => r.household_id === householdId && (r.status === 'active' || r.status === 'pending')))
      } catch (error) {
        if (!cancelled) {
          setDocTypes([])
          setSubmissions([])
          setSwimmers([])
          setRegistrations([])
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
  // 'pending' = a submission was started but no file was attached (e.g. the
  // upload failed), so it still needs an upload from the family.
  const pendingDocs = merged.filter(d => d.effectiveStatus === 'missing' || d.effectiveStatus === 'pending' || d.effectiveStatus === 'rejected' || d.effectiveStatus === 'expired')
  const visibleDocs = filter === 'pending' ? pendingDocs : merged

  const categories = [...new Set(visibleDocs.map(d => d.typeCategory))]

  // Registration waiver acceptance lives on the registration record itself, separately
  // from the document_types/submissions system above — surface it here too so this
  // page and the Registrations page never disagree about whether a waiver is pending.
  const waiverRows = registrations.map(reg => {
    const swimmer = swimmers.find(s => s.id === reg.swimmer_id)
    const summary = summarizeRegistrationWaiver(reg)
    return {
      id: reg.id,
      swimmerLabel: swimmer ? `${swimmer.first_name} ${swimmer.last_name}` : 'Household',
      season: reg.season,
      accepted: summary.accepted,
      count: summary.count,
      types: summary.types,
      lastAcceptedAt: summary.lastAcceptedAt ?? null,
    }
  })
  const pendingWaivers = waiverRows.filter(w => !w.accepted)
  const visibleWaivers = filter === 'pending' ? pendingWaivers : waiverRows

  const totalItemCount = merged.length + waiverRows.length
  const totalPendingCount = pendingDocs.length + pendingWaivers.length

  const completedCount = merged.filter(d => d.effectiveStatus === 'verified' || d.effectiveStatus === 'submitted').length
    + waiverRows.filter(w => w.accepted).length
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
      setUploadError('Couldn’t upload that file. Try again.')
      // A submission may have been created before the failure; reload so a
      // retry reuses it instead of creating a duplicate.
      fetchDocumentSubmissions(api, orgId, householdId).then(setSubmissions).catch(() => {})
    } finally {
      setUploadingId(null)
      setPendingUploadTarget(null)
    }
  }

  if (loading) {
    return (
      <div>
        <div className="page-heading">
          <h1 className="page-title">Documents</h1>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
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
      <div className="page-heading">
        <h1 className="page-title">
          Documents
        </h1>
      </div>

      {loadError && (
        <div style={{ padding: '12px 16px', background: 'var(--warning-bg)', border: '1px solid #fdba74', borderRadius: '12px', marginBottom: '16px', fontSize: 'var(--fs-base)', color: 'var(--warning)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px' }}>
          <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <span>{loadError}</span>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: 'var(--bg-elevated)', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: 'var(--warning)', fontSize: 'var(--fs-sm)', fontWeight: 600, padding: '8px 16px', whiteSpace: 'nowrap' }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Upload error */}
      {uploadError && (
        <div style={{ padding: '12px 16px', background: 'rgba(220,38,38,0.07)', border: '1px solid rgba(220,38,38,0.18)', borderRadius: '12px', marginBottom: '16px', fontSize: 'var(--fs-base)', color: 'var(--error)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{uploadError}</span>
          <button onClick={() => setUploadError(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--error)', fontSize: 'var(--fs-lg)', lineHeight: 1, padding: 0 }}>✕</button>
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
      {!loadError && totalPendingCount > 0 && (
        <div style={{ background: 'var(--error-bg)', border: '1px solid rgba(220,38,38,0.18)', borderRadius: '12px', padding: '20px 24px', marginBottom: '24px', animation: 'slideUp 0.35s ease both' }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '16px' }}>
            <div style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'rgba(220,38,38,0.10)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <Icon name="alert-circle" size={20} style={{ color: 'var(--error)' }} />
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: 'var(--error)', marginBottom: '4px' }}>
                {totalPendingCount} item{totalPendingCount !== 1 ? 's' : ''} require immediate action
              </div>
              <ul style={{ margin: 0, padding: '0 0 0 16px', fontSize: 'var(--fs-base)', color: 'var(--error)', lineHeight: 1.7 }}>
                {pendingDocs.map(doc => (
                  <li key={doc.typeId}>
                    <strong>{doc.typeName}</strong>
                    {doc.expiresAt && daysUntil(doc.expiresAt) !== null && (daysUntil(doc.expiresAt) ?? 99) <= 14
                      ? ` — due by ${fmtDate(doc.expiresAt)}`
                      : ''}
                  </li>
                ))}
                {pendingWaivers.map(w => (
                  <li key={w.id}>
                    <strong>Registration waiver — {w.swimmerLabel}</strong> — not yet accepted
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Stats strip */}
      <div className="cols-3" style={{ gap: '12px', marginBottom: '24px', animation: 'slideUp 0.4s ease both' }}>
        {[
          { label: 'Complete',      value: completedCount,     total: totalItemCount, color: 'var(--success)' },
          { label: 'Action needed', value: totalPendingCount,  total: totalItemCount, color: 'var(--error)' },
          { label: 'Expiring soon', value: expiringCount,       total: totalItemCount, color: 'var(--warning)' },
        ].map(stat => (
          <div key={stat.label} className="card" style={{ padding: '16px 20px' }}>
            <div style={{ fontSize: 'var(--fs-2xl)', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-body)' }}>
              {stat.value}<span style={{ fontSize: 'var(--fs-md)', color: 'var(--border-strong)', fontWeight: 400 }}>/{stat.total}</span>
            </div>
            <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '4px' }}>{stat.label}</div>
          </div>
        ))}
      </div>

      {/* Filter tabs */}
      <div style={{ display: 'flex', gap: '2px', marginBottom: '20px', background: 'rgba(0,0,0,0.04)', borderRadius: '12px', padding: '4px', maxWidth: '320px', animation: 'slideUp 0.42s ease both' }}>
        {[{ key: 'all', label: 'All documents' }, { key: 'pending', label: `Needs Action (${totalPendingCount})` }].map(({ key, label }) => (
          <button key={key} onClick={() => setFilter(key as 'all' | 'pending')} style={{ flex: 1, padding: '8px 12px', borderRadius: '8px', border: 'none', background: filter === key ? 'var(--bg-elevated)' : 'transparent', color: filter === key ? 'var(--text)' : 'var(--text-subtle)', fontSize: 'var(--fs-base)', fontWeight: filter === key ? 600 : 450, cursor: 'pointer', boxShadow: filter === key ? '0 1px 4px rgba(0,0,0,0.08)' : 'none', transition: 'all 120ms ease', fontFamily: 'var(--font-body)', whiteSpace: 'nowrap' }}>
            {label}
          </button>
        ))}
      </div>

      {/* Empty state */}
      {shouldShowCollectionEmptyState(loading, loadError, totalItemCount) && (
        <div className="card" style={{ textAlign: 'center', padding: '64px 20px' }}>
          <div style={{ fontSize: 'var(--fs-4xl)', marginBottom: '12px' }}>📄</div>
          <div style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)', marginBottom: '6px' }}>No documents required yet</div>
          <div style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)' }}>Your coach will request documents when needed.</div>
        </div>
      )}

      {/* Registration waivers */}
      {!loadError && visibleWaivers.length > 0 && (
        <section style={{ marginBottom: 'var(--space-8)', animation: 'slideUp 0.45s ease both' }}>
          <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '12px' }}>
            Registration waivers
          </div>
          <div className="card" style={{ overflow: 'hidden' }}>
            {visibleWaivers.map((w, i) => {
              const statusCfg = w.accepted ? STATUS_CONFIG.verified : STATUS_CONFIG.pending
              return (
                <div key={w.id} style={{ display: 'flex', gap: '16px', padding: '20px 20px', borderBottom: i < visibleWaivers.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', alignItems: 'flex-start', background: w.accepted ? 'transparent' : 'rgba(220,38,38,0.01)' }}>
                  <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: statusCfg.bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                    <Icon name={statusCfg.icon} size={18} style={{ color: statusCfg.color }} />
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>
                      Registration waiver — {w.swimmerLabel}
                    </div>
                    <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                      {w.accepted
                        ? `${w.count} accepted waiver${w.count === 1 ? '' : 's'}${w.lastAcceptedAt ? ` • last accepted ${fmtDate(w.lastAcceptedAt)}` : ''} (${w.season})`
                        : `No registration waiver acceptance is recorded yet (${w.season}).`}
                    </div>
                  </div>
                  <span style={{ padding: '4px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, whiteSpace: 'nowrap', flexShrink: 0 }}>
                    {statusCfg.label}
                  </span>
                </div>
              )
            })}
          </div>
        </section>
      )}

      {/* Documents by category */}
      {!loadError ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
        {categories.map((cat, catIdx) => {
          const catDocs = visibleDocs.filter(d => d.typeCategory === cat)
          return (
            <section key={cat} style={{ animation: `slideUp ${0.45 + catIdx * 0.06}s ease both` }}>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '12px' }}>
                {CATEGORY_LABELS[cat] ?? cat}
              </div>

              <div className="card" style={{ overflow: 'hidden' }}>
                {catDocs.map((doc, i) => {
                  const statusCfg = STATUS_CONFIG[doc.effectiveStatus]
                  const isUploading = uploadingId === doc.typeId
                  const needsAction = doc.effectiveStatus === 'missing' || doc.effectiveStatus === 'pending' || doc.effectiveStatus === 'rejected' || doc.effectiveStatus === 'expired'
                  const actionKind = getPortalDocumentAction({
                    effectiveStatus: doc.effectiveStatus,
                    typeCategory: doc.typeCategory,
                  })
                  const days = daysUntil(doc.expiresAt)
                  const expiringUrgent = days !== null && days >= 0 && days <= 14

                  return (
                    <div key={doc.typeId} style={{ display: 'flex', gap: '16px', padding: '20px 20px', borderBottom: i < catDocs.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', alignItems: 'flex-start', background: needsAction ? 'rgba(220,38,38,0.01)' : 'transparent' }}>
                      {/* Status icon */}
                      <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: statusCfg.bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <Icon name={statusCfg.icon} size={18} style={{ color: statusCfg.color }} />
                      </div>

                      {/* Content */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ display: 'flex', alignItems: 'flex-start', gap: '8px', flexWrap: 'wrap', marginBottom: '4px' }}>
                          <span style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.3 }}>{doc.typeName}</span>
                          {doc.isRequired && (
                            <span style={{ padding: '1px 6px', background: 'rgba(0,0,0,0.05)', borderRadius: '4px', fontSize: 'var(--fs-2xs)', color: 'var(--text-muted)', fontWeight: 500, whiteSpace: 'nowrap' }}>Required</span>
                          )}
                          {doc.submission?.review_notes && (doc.effectiveStatus === 'rejected') && (
                            <span style={{ padding: '1px 8px', background: 'rgba(220,38,38,0.08)', borderRadius: '4px', fontSize: 'var(--fs-2xs)', color: 'var(--error)', fontWeight: 500 }}>
                              {doc.submission.review_notes.slice(0, 60)}
                            </span>
                          )}
                        </div>
                        {doc.typeDescription && (
                          <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', lineHeight: 1.4, marginBottom: '6px' }}>{doc.typeDescription}</div>
                        )}
                        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', fontSize: 'var(--fs-xs)', color: 'var(--border-strong)' }}>
                          <span>For: <span style={{ color: 'var(--text-subtle)' }}>{doc.swimmerLabel}</span></span>
                          {doc.submittedAt && <span>Submitted: <span style={{ color: 'var(--text-subtle)' }}>{fmtDate(doc.submittedAt)}</span></span>}
                          {doc.expiresAt && (
                            <span>
                              Expires:{' '}
                              <span style={{ color: expiringUrgent ? 'var(--warning)' : 'var(--text-subtle)', fontWeight: expiringUrgent ? 600 : 400 }}>
                                {fmtDate(doc.expiresAt)}
                                {expiringUrgent && days !== null && (
                                  <span style={{ marginLeft: '4px', color: 'var(--warning)' }}>({days}d)</span>
                                )}
                              </span>
                            </span>
                          )}
                        </div>
                      </div>

                      {/* Action area */}
                      <div style={{ flexShrink: 0, display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '6px' }}>
                        <span style={{ padding: '4px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: statusCfg.bg, color: statusCfg.color, whiteSpace: 'nowrap' }}>
                          {statusCfg.label}
                        </span>

                        {actionKind === 'upload' && needsAction && (
                          <button
                            onClick={() => handleUploadClick(doc.rawTypeId, doc.swimmerId || undefined)}
                            disabled={isUploading}
                            style={{ display: 'flex', alignItems: 'center', gap: '4px', padding: '6px 16px', background: isUploading ? 'rgba(235,93,30,0.5)' : 'var(--accent)', color: 'var(--bg-elevated)', border: 'none', borderRadius: '8px', fontSize: 'var(--fs-sm)', fontWeight: 600, cursor: isUploading ? 'wait' : 'pointer', fontFamily: 'var(--font-body)' }}
                          >
                            <Icon name="upload" size={12} />
                            {isUploading ? 'Uploading…' : 'Upload File'}
                          </button>
                        )}

                        {(doc.effectiveStatus === 'verified' || doc.effectiveStatus === 'submitted') && doc.submission?.file_name && (
                          <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
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
        <div className="card" style={{ textAlign: 'center', padding: '40px 20px', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>
          Document requirements will appear here after a successful reload.
        </div>
      )}

      {/* Help notice */}
      <div style={{ marginTop: '24px', padding: '16px 20px', background: 'rgba(0,0,0,0.03)', borderRadius: '12px', fontSize: 'var(--fs-base)', color: 'var(--text-muted)', lineHeight: 1.5, animation: 'slideUp 0.7s ease both', display: 'flex', gap: '12px', alignItems: 'flex-start' }}>
        <Icon name="info" size={14} style={{ color: 'var(--border-strong)', marginTop: '2px', flexShrink: 0 }} />
        <span>
          Missing or expired documents may affect meet participation eligibility. Questions about a document?{' '}
          <Link href="/portal/messages" style={{ color: 'var(--accent)', fontWeight: 500, textDecoration: 'none' }}>Message the club staff</Link>.
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
