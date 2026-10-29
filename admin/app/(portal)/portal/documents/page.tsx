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
import { documentNeedsAction, getPortalDocumentAction, resolveDocumentStatus, selectLatestSubmission, type DocumentStatus } from '@/lib/document-submissions'
import { summarizeRegistrationWaiver } from '@/lib/portal-registration-status'
import { formatDateOnly } from '@/lib/dates'
import './page.inline.css'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'

// ── Helpers ───────────────────────────────────────────────────────────────────

function daysUntil(iso?: string | null): number | null {
  if (!iso) return null
  const diff = new Date(iso).getTime() - Date.now()
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

// ── Types ─────────────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<DocumentStatus, {
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
  effectiveStatus: DocumentStatus
  expiresAt: string | null
  submittedAt: string | null
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
          effectiveStatus: resolveDocumentStatus(sub),
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
            effectiveStatus: resolveDocumentStatus(sub),
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
        effectiveStatus: resolveDocumentStatus(sub),
        expiresAt: sub?.expires_at ?? null,
        submittedAt: sub?.submitted_at ?? null,
      })
    }
  }

  return docs
}

type StatusCfg = (typeof STATUS_CONFIG)[DocumentStatus]

function StatusIcon({ cfg }: { cfg: StatusCfg }) {
  return (
    <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: cfg.bg, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
      <Icon name={cfg.icon} size={18} style={{ color: cfg.color }} />
    </div>
  )
}

function StatusBadge({ cfg }: { cfg: StatusCfg }) {
  return (
    <span style={{ padding: '4px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', fontWeight: 600, background: cfg.bg, color: cfg.color, whiteSpace: 'nowrap', flexShrink: 0 }}>
      {cfg.label}
    </span>
  )
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
      setLoadError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
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
            'We could not load this page. Check your connection and try again.',
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
  const pendingDocs = merged.filter(d => documentNeedsAction(d.effectiveStatus))
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
        <div className="is-930d87f">
          {[1, 2, 3].map(i => (
            <div key={i} style={{ height: '68px', borderRadius: '12px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }} />
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
        <LoadErrorBanner message={loadError} onRetry={() => setReloadKey((key) => key + 1)} />
      )}

      {/* Upload error */}
      {uploadError && (
        <div className="is-78da8de">
          <span>{uploadError}</span>
          <button type="button" onClick={() => setUploadError(null)} className="is-3e74408">✕</button>
        </div>
      )}

      <input aria-label="Upload document file"
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
        <div className="is-fe04bf3">
          <div className="is-005cdcc">
            <div style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'rgba(220,38,38,0.10)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <Icon name="alert-circle" size={20} style={{ color: 'var(--error)' }} />
            </div>
            <div style={{ flex: 1 }}>
              <div className="is-417d5c5">
                {totalPendingCount} item{totalPendingCount !== 1 ? 's' : ''} require immediate action
              </div>
              <ul className="is-092ffa9">
                {pendingDocs.map(doc => (
                  <li key={doc.typeId}>
                    <strong>{doc.typeName}</strong>
                    {doc.expiresAt && daysUntil(doc.expiresAt) !== null && (daysUntil(doc.expiresAt) ?? 99) <= 14
                      ? ` — due by ${formatDateOnly(doc.expiresAt)}`
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
      <div className="cols-3 is-ce77c34" >
        {[
          { label: 'Complete',      value: completedCount,     total: totalItemCount, color: 'var(--success)' },
          { label: 'Action needed', value: totalPendingCount,  total: totalItemCount, color: 'var(--error)' },
          { label: 'Expiring soon', value: expiringCount,       total: totalItemCount, color: 'var(--warning)' },
        ].map(stat => (
          <div key={stat.label} className="card is-401e6d8" >
            <div style={{ fontSize: 'var(--fs-2xl)', fontWeight: 700, color: stat.color, fontFamily: 'var(--font-body)' }}>
              {stat.value}<span className="is-f57b355">/{stat.total}</span>
            </div>
            <div className="is-dd721e0">{stat.label}</div>
          </div>
        ))}
      </div>

      {/* Filter tabs */}
      <div className="is-7c2ea4b">
        {[{ key: 'all', label: 'All documents' }, { key: 'pending', label: `Needs Action (${totalPendingCount})` }].map(({ key, label }) => (
          <button type="button" key={key} onClick={() => setFilter(key as 'all' | 'pending')} style={{ flex: 1, padding: '8px 12px', borderRadius: '8px', border: 'none', background: filter === key ? 'var(--bg-elevated)' : 'transparent', color: filter === key ? 'var(--text)' : 'var(--text-subtle)', fontSize: 'var(--fs-base)', fontWeight: filter === key ? 600 : 450, cursor: 'pointer', boxShadow: filter === key ? 'var(--shadow-xs)' : 'none', transition: 'all 120ms ease', fontFamily: 'var(--font-body)', whiteSpace: 'nowrap' }}>
            {label}
          </button>
        ))}
      </div>

      {/* Empty state */}
      {shouldShowCollectionEmptyState(loading, loadError, totalItemCount) && (
        <div className="card is-4d5e0fe" >
          <div className="is-ed44472">No documents required yet</div>
          <div className="is-550d8c4">Your coach will request documents when needed.</div>
        </div>
      )}

      {/* Registration waivers */}
      {!loadError && visibleWaivers.length > 0 && (
        <section className="is-c3e1920">
          <div className="is-a1a186f">
            Registration waivers
          </div>
          <div className="card" style={{ overflow: 'hidden' }}>
            {visibleWaivers.map((w, i) => {
              const statusCfg = w.accepted ? STATUS_CONFIG.verified : STATUS_CONFIG.pending
              return (
                <div key={w.id} style={{ display: 'flex', gap: '16px', padding: '20px 20px', borderBottom: i < visibleWaivers.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'flex-start', background: w.accepted ? 'transparent' : 'rgba(220,38,38,0.01)' }}>
                  <StatusIcon cfg={statusCfg} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="is-ff14055">
                      Registration waiver — {w.swimmerLabel}
                    </div>
                    <div className="is-1042204">
                      {w.accepted
                        ? `${w.count} accepted waiver${w.count === 1 ? '' : 's'}${w.lastAcceptedAt ? ` • last accepted ${formatDateOnly(w.lastAcceptedAt)}` : ''} (${w.season})`
                        : `No registration waiver acceptance is recorded yet (${w.season}).`}
                    </div>
                  </div>
                  <StatusBadge cfg={statusCfg} />
                </div>
              )
            })}
          </div>
        </section>
      )}

      {/* Documents by category */}
      {!loadError ? (
        <div className="is-4ac1208">
        {categories.map((cat, catIdx) => {
          const catDocs = visibleDocs.filter(d => d.typeCategory === cat)
          return (
            <section key={cat} style={{ animation: `slideUp ${0.45 + catIdx * 0.06}s ease both` }}>
              <div className="is-a1a186f">
                {CATEGORY_LABELS[cat] ?? cat}
              </div>

              <div className="card" style={{ overflow: 'hidden' }}>
                {catDocs.map((doc, i) => {
                  const statusCfg = STATUS_CONFIG[doc.effectiveStatus]
                  const isUploading = uploadingId === doc.typeId
                  const needsAction = documentNeedsAction(doc.effectiveStatus)
                  const actionKind = getPortalDocumentAction({
                    effectiveStatus: doc.effectiveStatus,
                    typeCategory: doc.typeCategory,
                  })
                  const days = daysUntil(doc.expiresAt)
                  const expiringUrgent = days !== null && days >= 0 && days <= 14

                  return (
                    <div key={doc.typeId} style={{ display: 'flex', gap: '16px', padding: '20px 20px', borderBottom: i < catDocs.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'flex-start', background: needsAction ? 'rgba(220,38,38,0.01)' : 'transparent' }}>
                      {/* Status icon */}
                      <StatusIcon cfg={statusCfg} />

                      {/* Content */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div className="is-02c4154">
                          <span className="is-c13e111">{doc.typeName}</span>
                          {doc.isRequired && (
                            <span className="is-b09e6ca">Required</span>
                          )}
                          {doc.submission?.review_notes && (doc.effectiveStatus === 'rejected') && (
                            <span className="is-4b6d077">
                              {doc.submission.review_notes.slice(0, 60)}
                            </span>
                          )}
                        </div>
                        {doc.typeDescription && (
                          <div className="is-888a004">{doc.typeDescription}</div>
                        )}
                        <div className="is-9b73712">
                          <span>For: <span style={{ color: 'var(--text-subtle)' }}>{doc.swimmerLabel}</span></span>
                          {doc.submittedAt && <span>Submitted: <span style={{ color: 'var(--text-subtle)' }}>{formatDateOnly(doc.submittedAt)}</span></span>}
                          {doc.expiresAt && (
                            <span>
                              Expires:{' '}
                              <span style={{ color: expiringUrgent ? 'var(--warning)' : 'var(--text-subtle)', fontWeight: expiringUrgent ? 600 : 400 }}>
                                {formatDateOnly(doc.expiresAt)}
                                {expiringUrgent && days !== null && (
                                  <span className="is-3ef9660">({days}d)</span>
                                )}
                              </span>
                            </span>
                          )}
                        </div>
                      </div>

                      {/* Action area */}
                      <div className="is-0da4d5a">
                        <StatusBadge cfg={statusCfg} />

                        {actionKind === 'upload' && needsAction && (
                          <button type="button"
                            onClick={() => handleUploadClick(doc.rawTypeId, doc.swimmerId || undefined)}
                            disabled={isUploading}
                            style={{ display: 'flex', alignItems: 'center', gap: '4px', padding: '6px 16px', background: isUploading ? 'rgba(235,93,30,0.5)' : 'var(--accent)', color: 'var(--bg-elevated)', border: 'none', borderRadius: '8px', fontSize: 'var(--fs-sm)', fontWeight: 600, cursor: isUploading ? 'wait' : 'pointer', fontFamily: 'var(--font-body)' }}
                          >
                            <Icon name="upload" size={12} />
                            {isUploading ? 'Uploading…' : 'Upload File'}
                          </button>
                        )}

                        {(doc.effectiveStatus === 'verified' || doc.effectiveStatus === 'submitted') && doc.submission?.file_name && (
                          <span className="is-915a8b5">
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
        <div className="card is-a0babcf" >
          Document requirements will appear here after a successful reload.
        </div>
      )}

      {/* Help notice */}
      <div className="is-a8080b5">
        <Icon name="info" size={14} style={{ color: 'var(--border-strong)', marginTop: '2px', flexShrink: 0 }} />
        <span>
          Missing or expired documents may affect meet participation eligibility. Questions about a document?{' '}
          <Link href="/portal/messages" className="is-bc2c7b1">Message the club staff</Link>.
        </span>
      </div>

    </div>
  )
}
