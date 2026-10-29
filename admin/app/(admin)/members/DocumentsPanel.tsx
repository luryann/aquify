'use client'

import { useEffect, useState, useCallback } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { formatDateOnly } from '@/lib/dates'
import {
  type DocumentTypeRecord,
  type DocumentSubmissionRecord,
  fetchDocumentTypes,
  createDocumentType,
  updateDocumentType,
  deleteDocumentType,
  fetchDocumentSubmissions,
  fetchHouseholds,
  fetchSwimmers,
  verifyDocumentSubmission,
  rejectDocumentSubmission,
} from '@/lib/adminData'
import { Icon } from '@/components/ui/icons'
import { Modal } from '@/components/ui/Modal'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import './DocumentsPanel.inline.css'

// ── Helpers ───────────────────────────────────────────────────────────────────


const CATEGORY_COLORS: Record<string, { bg: string; text: string }> = {
  waiver:       { bg: 'rgba(235,93,30,0.10)', text: 'var(--accent-text)' },
  medical:      { bg: 'rgba(239,68,68,0.10)', text: 'var(--error)' },
  insurance:    { bg: 'rgba(59,130,246,0.10)', text: 'var(--info)' },
  release:      { bg: 'rgba(16,185,129,0.10)', text: 'var(--success)' },
  registration: { bg: 'rgba(139,92,246,0.10)', text: 'var(--text-subtle)' },
  other:        { bg: 'rgba(107,114,128,0.10)', text: 'var(--text-subtle)' },
}

const STATUS_COLORS: Record<string, { bg: string; text: string; dot: string }> = {
  pending:   { bg: 'rgba(107,114,128,0.10)', text: 'var(--text-subtle)', dot: '#9ca3af' },
  submitted: { bg: 'rgba(245,158,11,0.12)', text: 'var(--warning)', dot: '#f59e0b' },
  verified:  { bg: 'rgba(16,185,129,0.10)', text: 'var(--success)', dot: '#10b981' },
  rejected:  { bg: 'rgba(239,68,68,0.10)', text: 'var(--error)', dot: '#ef4444' },
  expired:   { bg: 'rgba(239,68,68,0.10)', text: 'var(--error)', dot: '#ef4444' },
}

// ── Sub-components ────────────────────────────────────────────────────────────

function Pill({ value, map }: { value: string; map: Record<string, { bg: string; text: string }> }) {
  const colors = map[value] ?? { bg: 'var(--border)', text: '#555' }
  return (
    <span className="u-gap-4px u-p-2px-8px u-fs-fs-xs u-fw-600 u-ff-font-body u-ls-0_02em" style={{ display: 'inline-flex', alignItems: 'center', borderRadius: '999px', background: colors.bg, color: colors.text, textTransform: 'capitalize' }}>
      {value}
    </span>
  )
}

function StatusPill({ status }: { status: string }) {
  const s = STATUS_COLORS[status] ?? STATUS_COLORS.pending
  return (
    <span className="u-gap-6px u-p-4px-12px u-fs-fs-xs u-fw-600 u-ff-font-body" style={{ display: 'inline-flex', alignItems: 'center', borderRadius: '999px', background: s.bg, color: s.text, textTransform: 'capitalize' }}>
      <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: s.dot, flexShrink: 0 }} />
      {status}
    </span>
  )
}

// ── Create/Edit Modal ─────────────────────────────────────────────────────────

interface TypeFormData {
  name: string
  category: string
  description: string
  is_required: boolean
  applies_to: string
  expiry_months: string
  is_active: boolean
}

const defaultForm: TypeFormData = {
  name: '', category: 'waiver', description: '',
  is_required: true, applies_to: 'swimmer', expiry_months: '', is_active: true,
}

function TypeModal({ initial, onSave, onClose }: {
  initial?: DocumentTypeRecord | null
  onSave: (data: TypeFormData) => Promise<void>
  onClose: () => void
}) {
  const [form, setForm] = useState<TypeFormData>(
    initial
      ? {
          name: initial.name, category: initial.category, description: initial.description ?? '',
          is_required: initial.is_required, applies_to: initial.applies_to,
          expiry_months: initial.expiry_months != null ? String(initial.expiry_months) : '',
          is_active: initial.is_active,
        }
      : defaultForm
  )
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setErr(null)
    try {
      await onSave(form)
      onClose()
    } catch (ex) {
      setErr(uiErrorMessage(ex, 'Save failed.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal open onClose={onClose} title={initial ? `Edit ${initial.name}` : 'New document type'} width={520}>
        <form onSubmit={handleSubmit} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
          {err && <div role="alert" className="form-banner-error">{err}</div>}

          <div>
            <label className="form-label form-label-required">Name</label>
            <input aria-label="e.g. Season Liability Waiver"
              value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
              required placeholder="e.g. Season Liability Waiver"
              className="u-p-8px-12px u-fs-fs-base" style={{ width: '100%' }}
            />
          </div>

          <div className="cols-2 u-gap-12px" >
            <div>
              <label className="form-label">Category</label>
              <select value={form.category} onChange={e => setForm(f => ({ ...f, category: e.target.value }))}
                className="u-p-8px-12px u-fs-fs-base" style={{ width: '100%' }}>
                {['waiver','medical','insurance','release','registration','other'].map(c => <option key={c} value={c}>{c.charAt(0).toUpperCase()+c.slice(1)}</option>)}
              </select>
            </div>
            <div>
              <label className="form-label">Applies to</label>
              <select value={form.applies_to} onChange={e => setForm(f => ({ ...f, applies_to: e.target.value }))}
                className="u-p-8px-12px u-fs-fs-base" style={{ width: '100%' }}>
                {['swimmer','household','all'].map(a => <option key={a} value={a}>{a.charAt(0).toUpperCase()+a.slice(1)}</option>)}
              </select>
            </div>
          </div>

          <div>
            <label className="form-label">Description</label>
            <textarea aria-label="Brief description for members"
              value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
              rows={2} placeholder="Brief description for members…"
              className="u-p-8px-12px u-fs-fs-base u-ff-font-body" style={{ width: '100%', resize: 'vertical' }}
            />
          </div>

          <div className="cols-2 u-gap-12px" >
            <div>
              <label className="form-label">Expiry (months)</label>
              <input aria-label="Never expires"
                type="number" min="1" value={form.expiry_months}
                onChange={e => setForm(f => ({ ...f, expiry_months: e.target.value }))}
                placeholder="Never expires"
                className="u-p-8px-12px u-fs-fs-base" style={{ width: '100%' }}
              />
            </div>
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column', justifyContent: 'flex-end' }}>
              <label className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', cursor: 'pointer' }}>
                <input type="checkbox" checked={form.is_required} onChange={e => setForm(f => ({ ...f, is_required: e.target.checked }))} />
                <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>Required</span>
              </label>
              <label className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', cursor: 'pointer' }}>
                <input type="checkbox" checked={form.is_active} onChange={e => setForm(f => ({ ...f, is_active: e.target.checked }))} />
                <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>Active</span>
              </label>
            </div>
          </div>

          <div className="form-actions">
            <button type="button" onClick={onClose} className="btn">Cancel</button>
            <button type="submit" disabled={saving} className="btn btn-primary">
              {saving ? 'Saving…' : (initial ? 'Save changes' : 'Create type')}
            </button>
          </div>
        </form>
    </Modal>
  )
}

// ── Review Notes Dialog ───────────────────────────────────────────────────────

function ReviewDialog({ action, onConfirm, onClose }: {
  action: 'verify' | 'reject'
  onConfirm: (notes: string) => Promise<void>
  onClose: () => void
}) {
  const [notes, setNotes] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handle() {
    setLoading(true)
    setError(null)
    try {
      await onConfirm(notes)
      onClose()
    } catch (e) {
      setError(uiErrorMessage(e, 'Couldn’t update this document. Try again.'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal open onClose={onClose} title={action === 'verify' ? 'Verify document' : 'Reject document'} width={420}>
        {error && <div role="alert" className="form-banner-error u-mb-12px">{error}</div>}
        <textarea aria-label="Optional review notes for the household"
          value={notes} onChange={e => setNotes(e.target.value)}
          rows={3} placeholder="Optional review notes for the household…"
          className="u-p-8px-12px u-fs-fs-base u-ff-font-body u-mb-16px" style={{ width: '100%', resize: 'vertical' }}
        />
        <div className="form-actions">
          <button type="button" onClick={onClose} className="btn">Cancel</button>
          <button type="button" onClick={() => void handle()} disabled={loading} className="u-p-8px-20px u-fs-fs-base u-fw-600" style={{ borderRadius: '8px', border: 'none', background: action === 'verify' ? '#059669' : '#dc2626', color: '#fff', cursor: loading ? 'wait' : 'pointer' }}>
            {loading ? '…' : (action === 'verify' ? 'Verify' : 'Reject')}
          </button>
        </div>
    </Modal>
  )
}

// ── Main Page ─────────────────────────────────────────────────────────────────

const STATUS_FILTERS = ['all', 'pending', 'submitted', 'verified', 'rejected', 'expired'] as const
type StatusFilter = typeof STATUS_FILTERS[number]

export function DocumentsPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [tab, setTab] = useState<'types' | 'submissions'>('types')
  const [docTypes, setDocTypes] = useState<DocumentTypeRecord[]>([])
  const [submissions, setSubmissions] = useState<DocumentSubmissionRecord[]>([])
  const [householdNames, setHouseholdNames] = useState<Map<string, string>>(new Map())
  const [swimmerNames, setSwimmerNames] = useState<Map<string, string>>(new Map())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Type management
  const [editingType, setEditingType] = useState<DocumentTypeRecord | null | 'new'>()
  const [deletingTypeId, setDeletingTypeId] = useState<string | null>(null)

  // Submission review
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [reviewing, setReviewing] = useState<{ id: string; action: 'verify' | 'reject' } | null>(null)

  const loadData = useCallback(async () => {
    if (!orgId) { setLoading(false); return }
    try {
      setLoading(true)
      setError(null)
      const [types, subs, households, swimmers] = await Promise.all([
        fetchDocumentTypes(api, orgId, false),
        fetchDocumentSubmissions(api, orgId),
        fetchHouseholds(api, orgId),
        fetchSwimmers(api, orgId),
      ])
      setDocTypes(types)
      setSubmissions(subs)
      setHouseholdNames(new Map(households.map((h) => [h.id, h.household_name])))
      setSwimmerNames(new Map(swimmers.map((sw) => [sw.id, `${sw.first_name} ${sw.last_name}`])))
    } catch (ex) {
      setError(uiErrorMessage(ex, 'Couldn’t load document center data. Try again.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => {
    void loadData()
  }, [loadData])

  async function handleSaveType(form: TypeFormData) {
    const payload = {
      name: form.name.trim(),
      category: form.category,
      description: form.description.trim() || undefined,
      is_required: form.is_required,
      applies_to: form.applies_to,
      expiry_months: form.expiry_months ? parseInt(form.expiry_months, 10) : undefined,
      is_active: form.is_active,
    }
    if (editingType && editingType !== 'new') {
      await updateDocumentType(api, orgId, editingType.id, payload)
    } else {
      await createDocumentType(api, orgId, payload)
    }
    await loadData()
  }

  async function handleDeleteType(id: string) {
    await deleteDocumentType(api, orgId, id)
    setDeletingTypeId(null)
    await loadData()
  }

  async function handleReview(notes: string) {
    if (!reviewing) return
    if (reviewing.action === 'verify') {
      await verifyDocumentSubmission(api, orgId, reviewing.id, notes || undefined)
    } else {
      await rejectDocumentSubmission(api, orgId, reviewing.id, notes || undefined)
    }
    await loadData()
  }

  const filteredSubs = statusFilter === 'all'
    ? submissions
    : submissions.filter(s => s.status === statusFilter)

  const typeMap = new Map(docTypes.map(t => [t.id, t]))

  const pendingCount = submissions.filter(s => s.status === 'submitted').length

  return (
    <div>

      {pendingCount > 0 && (
        <div className="u-gap-8px u-p-8px-16px u-mb-20px" style={{ display: 'flex', alignItems: 'center', borderRadius: '12px', background: 'rgba(245,158,11,0.10)', border: '1px solid rgba(245,158,11,0.25)', width: 'fit-content', animation: 'fadeUp 0.3s ease both' }}>
          <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: '#f59e0b', flexShrink: 0 }} />
          <span className="u-fs-fs-sm u-fw-600" style={{ color: 'var(--warning)' }}>
            {pendingCount} submission{pendingCount !== 1 ? 's' : ''} need review
          </span>
        </div>
      )}

      {error && (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      )}

      {/* Tabs */}
      <div className="seg u-mb-24px" role="tablist" style={{ animation: 'fadeUp 0.35s ease both' }}>
        {([['types', 'Document Types'], ['submissions', 'Submissions']] as const).map(([key, label]) => (
          <button key={key} type="button" role="tab" aria-selected={tab === key} onClick={() => setTab(key)} className="seg-item">
            {label}
            {key === 'submissions' && pendingCount > 0 && (
              <span className="seg-count">{pendingCount}</span>
            )}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column', animation: 'fadeUp 0.4s ease both' }}>
          {[1, 2, 3, 4].map(i => (
            <div key={i} style={{ height: '52px', borderRadius: '12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', opacity: 1 - i * 0.15 }} />
          ))}
        </div>
      ) : tab === 'types' ? (
        /* ── Document Types Tab ─────────────────────────────────────────────── */
        <div style={{ animation: 'fadeUp 0.35s ease both' }}>
          <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button type="button"
              onClick={() => setEditingType('new')}
              className="btn btn-primary"
            >
              <Icon name="plus" size={14} /> New document type
            </button>
          </div>

          {docTypes.length === 0 ? (
            <div className="card u-p-40px" style={{ textAlign: 'center' }}>
              <p className="u-ff-font-display u-fw-display-weight u-fs-fs-xl u-mb-8px" style={{ color: 'var(--text-subtle)' }}>No document types defined yet</p>
            </div>
          ) : (
            <div className="card" style={{ overflow: 'hidden' }}>
              <div className="table-scroll" style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
                      {['Name', 'Category', 'Applies To', 'Expiry', 'Required', 'Status', ''].map(h => (
                        <th key={h} className="col-head u-p-12px-16px" style={{ textAlign: 'left', whiteSpace: 'nowrap' }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {docTypes.map((dt, i) => (
                      <tr key={dt.id} style={{ borderBottom: i < docTypes.length - 1 ? '1px solid var(--border)' : 'none', transition: 'background 120ms' }}
                        onMouseEnter={e => (e.currentTarget.style.background = 'var(--bg-elevated)')}
                        onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}>
                        <td className="u-p-12px-16px" >
                          <div className="u-fs-fs-md u-fw-600" style={{ color: 'var(--text)' }}>{dt.name}</div>
                          {dt.description && <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>{dt.description}</div>}
                        </td>
                        <td className="u-p-12px-16px" ><Pill value={dt.category} map={CATEGORY_COLORS} /></td>
                        <td className="u-p-12px-16px u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>{dt.applies_to}</td>
                        <td className="u-p-12px-16px u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>
                          {dt.expiry_months ? `${dt.expiry_months}mo` : 'Never'}
                        </td>
                        <td className="u-p-12px-16px" >
                          <span className="u-fs-fs-xs u-ff-font-body" style={{ color: dt.is_required ? '#059669' : 'var(--text-muted)' }}>
                            {dt.is_required ? 'yes' : 'no'}
                          </span>
                        </td>
                        <td className="u-p-12px-16px" >
                          <span className="badge" style={{
                            background: dt.is_active ? 'var(--success-bg)' : 'var(--bg-elevated)',
                            color: dt.is_active ? 'var(--success)' : 'var(--text-muted)',
                          }}>
                            {dt.is_active ? 'active' : 'inactive'}
                          </span>
                        </td>
                        <td className="u-p-12px-16px" style={{ textAlign: 'right' }}>
                          <div className="u-gap-6px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
                            <button type="button" onClick={() => setEditingType(dt)} className="u-p-4px-12px u-fs-fs-sm u-ff-font-body" style={{ borderRadius: '8px', border: '1px solid var(--border)', background: 'transparent', color: 'var(--text-subtle)', cursor: 'pointer' }}>
                              Edit
                            </button>
                            <button type="button"
                              onClick={() => setDeletingTypeId(dt.id)}
                              className="u-p-4px-12px u-fs-fs-sm u-ff-font-body" style={{ borderRadius: '8px', border: '1px solid rgba(239,68,68,0.3)', background: 'transparent', color: '#dc2626', cursor: 'pointer' }}
                            >
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      ) : (
        /* ── Submissions Tab ────────────────────────────────────────────────── */
        <div style={{ animation: 'fadeUp 0.35s ease both' }}>
          {/* Status filter */}
          <div className="u-gap-6px u-mb-16px" style={{ display: 'flex', flexWrap: 'wrap' }}>
            {STATUS_FILTERS.map(sf => (
              <button type="button" key={sf} onClick={() => setStatusFilter(sf)} className="u-fs-fs-sm u-ff-font-body" style={{ padding: '4px 16px', borderRadius: '999px', border: `1px solid ${statusFilter === sf ? 'var(--accent)' : 'var(--border)'}`, background: statusFilter === sf ? 'var(--accent)' : 'transparent', color: statusFilter === sf ? '#fff' : 'var(--text-subtle)', fontWeight: statusFilter === sf ? 600 : 400, cursor: 'pointer', transition: 'all 120ms ease', textTransform: 'capitalize' }}>
                {sf === 'all' ? `All (${submissions.length})` : sf}
              </button>
            ))}
          </div>

          {filteredSubs.length === 0 ? (
            <div className="card u-p-40px" style={{ textAlign: 'center' }}>
              <p className="u-ff-font-display u-fw-display-weight u-fs-fs-xl u-mb-8px" style={{ color: 'var(--text-subtle)' }}>No submissions</p>
              <p className="page-subtitle">
                {statusFilter === 'all' ? 'Households have not submitted any documents yet.' : `No ${statusFilter} submissions found.`}
              </p>
            </div>
          ) : (
            <div className="card" style={{ overflow: 'hidden' }}>
              <div className="table-scroll" style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
                      {['Document Type', 'Household', 'Swimmer', 'Status', 'Submitted', 'Expires', 'Actions'].map(h => (
                        <th key={h} className="col-head u-p-12px-16px" style={{ textAlign: 'left', whiteSpace: 'nowrap' }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {filteredSubs.map((sub, i) => {
                      const typeName = sub.document_type_name ?? typeMap.get(sub.document_type_id)?.name ?? sub.document_type_id.slice(0, 8)
                      const canReview = sub.status === 'submitted'
                      return (
                        <tr key={sub.id} style={{ borderBottom: i < filteredSubs.length - 1 ? '1px solid var(--border)' : 'none', transition: 'background 120ms' }}
                          onMouseEnter={e => (e.currentTarget.style.background = 'var(--bg-elevated)')}
                          onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}>
                          <td className="u-p-12px-16px" >
                            <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{typeName}</div>
                            {sub.file_name && <div className="u-fs-fs-xs u-mt-2px u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{sub.file_name}</div>}
                          </td>
                          <td className="u-p-12px-16px u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>
                            {householdNames.get(sub.household_id) ?? 'Unknown household'}
                          </td>
                          <td className="u-p-12px-16px u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>
                            {sub.swimmer_id ? (swimmerNames.get(sub.swimmer_id) ?? 'Unknown swimmer') : '—'}
                          </td>
                          <td className="u-p-12px-16px" ><StatusPill status={sub.status} /></td>
                          <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDateOnly(sub.submitted_at)}</td>
                          <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: sub.expires_at && new Date(sub.expires_at) < new Date() ? '#dc2626' : 'var(--text-subtle)' }}>
                            {formatDateOnly(sub.expires_at)}
                          </td>
                          <td className="u-p-12px-16px" >
                            {canReview ? (
                              <div className="u-gap-6px" style={{ display: 'flex' }}>
                                <button type="button"
                                  onClick={() => setReviewing({ id: sub.id, action: 'verify' })}
                                  className="u-p-4px-12px u-fs-fs-xs u-fw-600 u-ff-font-body" style={{ borderRadius: '6px', border: '1px solid rgba(16,185,129,0.4)', background: 'rgba(16,185,129,0.06)', color: '#059669', cursor: 'pointer' }}
                                >
                                  ✓ Verify
                                </button>
                                <button type="button"
                                  onClick={() => setReviewing({ id: sub.id, action: 'reject' })}
                                  className="u-p-4px-12px u-fs-fs-xs u-fw-600 u-ff-font-body" style={{ borderRadius: '6px', border: '1px solid rgba(239,68,68,0.3)', background: 'rgba(239,68,68,0.05)', color: '#dc2626', cursor: 'pointer' }}
                                >
                                  ✗ Reject
                                </button>
                              </div>
                            ) : (
                              sub.review_notes ? (
                                <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }} title={sub.review_notes}>
                                  {sub.review_notes.slice(0, 30)}{sub.review_notes.length > 30 ? '…' : ''}
                                </span>
                              ) : (
                                <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>—</span>
                              )
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Delete confirm */}
      <Modal open={Boolean(deletingTypeId)} onClose={() => setDeletingTypeId(null)} title="Delete document type?" width={400}>
        <p className="u-fs-fs-base u-mb-20px u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>This soft-deletes the type. Existing submissions are unaffected.</p>
        <div className="form-actions">
          <button type="button" onClick={() => setDeletingTypeId(null)} className="btn">Cancel</button>
          <button type="button" onClick={() => { if (deletingTypeId) void handleDeleteType(deletingTypeId) }} className="btn" style={{ background: 'var(--error)', color: '#fff', border: 'none' }}>Delete type</button>
        </div>
      </Modal>

      {/* Type create/edit modal */}
      {editingType !== undefined && (
        <TypeModal
          initial={editingType === 'new' ? null : editingType}
          onSave={handleSaveType}
          onClose={() => setEditingType(undefined)}
        />
      )}

      {/* Review dialog */}
      {reviewing && (
        <ReviewDialog
          action={reviewing.action}
          onConfirm={handleReview}
          onClose={() => setReviewing(null)}
        />
      )}
    </div>
  )
}
