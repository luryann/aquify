'use client'

import { useEffect, useState, useCallback } from 'react'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  type DocumentTypeRecord,
  type DocumentSubmissionRecord,
  fetchDocumentTypes,
  createDocumentType,
  updateDocumentType,
  deleteDocumentType,
  fetchDocumentSubmissions,
  verifyDocumentSubmission,
  rejectDocumentSubmission,
} from '@/lib/adminData'

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmt(value?: string | null) {
  if (!value) return '—'
  return new Date(value).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

const CATEGORY_COLORS: Record<string, { bg: string; text: string }> = {
  waiver:       { bg: 'rgba(235,93,30,0.10)', text: '#c04010' },
  medical:      { bg: 'rgba(239,68,68,0.10)', text: '#b91c1c' },
  insurance:    { bg: 'rgba(59,130,246,0.10)', text: '#1d4ed8' },
  release:      { bg: 'rgba(16,185,129,0.10)', text: '#065f46' },
  registration: { bg: 'rgba(139,92,246,0.10)', text: '#5b21b6' },
  other:        { bg: 'rgba(107,114,128,0.10)', text: '#374151' },
}

const STATUS_COLORS: Record<string, { bg: string; text: string; dot: string }> = {
  pending:   { bg: 'rgba(107,114,128,0.10)', text: '#374151', dot: '#9ca3af' },
  submitted: { bg: 'rgba(245,158,11,0.12)', text: '#92400e', dot: '#f59e0b' },
  verified:  { bg: 'rgba(16,185,129,0.10)', text: '#065f46', dot: '#10b981' },
  rejected:  { bg: 'rgba(239,68,68,0.10)', text: '#b91c1c', dot: '#ef4444' },
  expired:   { bg: 'rgba(239,68,68,0.10)', text: '#b91c1c', dot: '#ef4444' },
}

// ── Sub-components ────────────────────────────────────────────────────────────

function Pill({ value, map }: { value: string; map: Record<string, { bg: string; text: string }> }) {
  const colors = map[value] ?? { bg: 'rgba(0,0,0,0.06)', text: '#555' }
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: '5px',
      padding: '2px 9px', borderRadius: '999px', fontSize: '11px', fontWeight: 600,
      background: colors.bg, color: colors.text, fontFamily: 'var(--font-mono)',
      letterSpacing: '0.02em', textTransform: 'capitalize',
    }}>
      {value}
    </span>
  )
}

function StatusPill({ status }: { status: string }) {
  const s = STATUS_COLORS[status] ?? STATUS_COLORS.pending
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: '6px',
      padding: '3px 10px', borderRadius: '999px', fontSize: '11px', fontWeight: 600,
      background: s.bg, color: s.text, fontFamily: 'var(--font-mono)',
      textTransform: 'capitalize',
    }}>
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
      setErr(ex instanceof ApiError ? ex.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{
      position: 'fixed', inset: 0, zIndex: 1000,
      background: 'rgba(10,8,6,0.55)', backdropFilter: 'blur(4px)',
      display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px',
    }}>
      <div style={{
        background: 'var(--bg-elevated)', borderRadius: '18px',
        border: '1px solid var(--border)', boxShadow: '0 24px 80px rgba(0,0,0,0.28)',
        width: '100%', maxWidth: '520px',
        animation: 'modalIn 0.22s cubic-bezier(0.22,1,0.36,1) both',
      }}>
        <div style={{ padding: '22px 26px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>
              {initial ? 'Edit' : 'New'} Document Type
            </p>
            <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', fontWeight: 400, color: 'var(--text)' }}>
              {initial ? initial.name : 'Define a required document'}
            </h2>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', fontSize: '18px', lineHeight: 1 }}>✕</button>
        </div>

        <form onSubmit={handleSubmit} style={{ padding: '22px 26px 24px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {err && <div style={{ padding: '10px 14px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: '12px' }}>{err}</div>}

          <div>
            <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px' }}>Name *</label>
            <input
              value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
              required placeholder="e.g. Season Liability Waiver"
              style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)', fontSize: '13px', boxSizing: 'border-box', outline: 'none' }}
            />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
            <div>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px' }}>Category</label>
              <select value={form.category} onChange={e => setForm(f => ({ ...f, category: e.target.value }))}
                style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)', fontSize: '13px' }}>
                {['waiver','medical','insurance','release','registration','other'].map(c => <option key={c} value={c}>{c.charAt(0).toUpperCase()+c.slice(1)}</option>)}
              </select>
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px' }}>Applies To</label>
              <select value={form.applies_to} onChange={e => setForm(f => ({ ...f, applies_to: e.target.value }))}
                style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)', fontSize: '13px' }}>
                {['swimmer','household','all'].map(a => <option key={a} value={a}>{a.charAt(0).toUpperCase()+a.slice(1)}</option>)}
              </select>
            </div>
          </div>

          <div>
            <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px' }}>Description</label>
            <textarea
              value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
              rows={2} placeholder="Brief description for members…"
              style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)', fontSize: '13px', resize: 'vertical', boxSizing: 'border-box', fontFamily: 'var(--font-body)' }}
            />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
            <div>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '6px' }}>Expiry (months)</label>
              <input
                type="number" min="1" value={form.expiry_months}
                onChange={e => setForm(f => ({ ...f, expiry_months: e.target.value }))}
                placeholder="Leave blank = never"
                style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)', fontSize: '13px', boxSizing: 'border-box' }}
              />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', gap: '10px' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                <input type="checkbox" checked={form.is_required} onChange={e => setForm(f => ({ ...f, is_required: e.target.checked }))} style={{ width: '15px', height: '15px', accentColor: 'var(--accent)' }} />
                <span style={{ fontSize: '13px', color: 'var(--text)' }}>Required</span>
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                <input type="checkbox" checked={form.is_active} onChange={e => setForm(f => ({ ...f, is_active: e.target.checked }))} style={{ width: '15px', height: '15px', accentColor: 'var(--accent)' }} />
                <span style={{ fontSize: '13px', color: 'var(--text)' }}>Active</span>
              </label>
            </div>
          </div>

          <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
            <button type="button" onClick={onClose} style={{ padding: '9px 18px', borderRadius: '8px', border: '1px solid var(--border)', background: 'transparent', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer' }}>Cancel</button>
            <button type="submit" disabled={saving} style={{ padding: '9px 22px', borderRadius: '8px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: saving ? 'wait' : 'pointer' }}>
              {saving ? 'Saving…' : (initial ? 'Save Changes' : 'Create Type')}
            </button>
          </div>
        </form>
      </div>
    </div>
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
      setError(e instanceof Error ? e.message : 'Action failed. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ position: 'fixed', inset: 0, zIndex: 1100, background: 'rgba(10,8,6,0.55)', backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px' }}>
      <div style={{ background: 'var(--bg-elevated)', borderRadius: '16px', border: '1px solid var(--border)', boxShadow: '0 20px 60px rgba(0,0,0,0.24)', width: '100%', maxWidth: '400px', padding: '24px', animation: 'modalIn 0.2s ease both' }}>
        <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 400, color: 'var(--text)', marginBottom: '12px' }}>
          {action === 'verify' ? '✓ Verify Document' : '✗ Reject Document'}
        </h3>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '14px' }}>
          {action === 'verify' ? 'Mark this submission as verified.' : 'Reject and request re-submission.'}
        </p>
        {error && (
          <p style={{ color: 'var(--error, #dc2626)', fontSize: '12px', marginBottom: '12px', padding: '8px 12px', background: 'var(--error-bg, rgba(220,38,38,0.07))', borderRadius: '6px', border: '1px solid var(--error-border, rgba(220,38,38,0.2))' }}>
            {error}
          </p>
        )}
        <textarea
          value={notes} onChange={e => setNotes(e.target.value)}
          rows={3} placeholder="Optional review notes for the household…"
          style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg)', color: 'var(--text)', fontSize: '13px', resize: 'vertical', boxSizing: 'border-box', fontFamily: 'var(--font-body)', marginBottom: '16px' }}
        />
        <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
          <button onClick={onClose} style={{ padding: '8px 16px', borderRadius: '8px', border: '1px solid var(--border)', background: 'transparent', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer' }}>Cancel</button>
          <button onClick={() => void handle()} disabled={loading} style={{
            padding: '8px 18px', borderRadius: '8px', border: 'none',
            background: action === 'verify' ? '#059669' : '#dc2626',
            color: '#fff', fontSize: '13px', fontWeight: 600, cursor: loading ? 'wait' : 'pointer',
          }}>
            {loading ? '…' : (action === 'verify' ? 'Verify' : 'Reject')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main Page ─────────────────────────────────────────────────────────────────

const STATUS_FILTERS = ['all', 'pending', 'submitted', 'verified', 'rejected', 'expired'] as const
type StatusFilter = typeof STATUS_FILTERS[number]

export default function DocumentCenterPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [tab, setTab] = useState<'types' | 'submissions'>('types')
  const [docTypes, setDocTypes] = useState<DocumentTypeRecord[]>([])
  const [submissions, setSubmissions] = useState<DocumentSubmissionRecord[]>([])
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
      const [types, subs] = await Promise.all([
        fetchDocumentTypes(api, orgId, false),
        fetchDocumentSubmissions(api, orgId),
      ])
      setDocTypes(types)
      setSubmissions(subs)
    } catch (ex) {
      setError(ex instanceof ApiError ? ex.message : 'Failed to load document center data.')
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
      await createDocumentType(api, orgId, { ...payload, name: payload.name, is_required: payload.is_required, is_active: payload.is_active })
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
    <div style={{ padding: '32px 36px 80px', maxWidth: 1200, margin: '0 auto' }}>
      <style>{`
        @keyframes modalIn {
          from { opacity: 0; transform: scale(0.96) translateY(8px); }
          to   { opacity: 1; transform: scale(1)    translateY(0);   }
        }
        @keyframes fadeUp {
          from { opacity: 0; transform: translateY(10px); }
          to   { opacity: 1; transform: translateY(0);     }
        }
      `}</style>

      {/* Header */}
      <div style={{ marginBottom: '28px', animation: 'fadeUp 0.3s ease both' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Compliance / Documents
        </p>
        <div style={{ display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px' }}>
          <div>
            <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '4px' }}>
              Document Center
            </h1>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
              Manage required document types and review household submissions.
            </p>
          </div>
          {pendingCount > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '8px 14px', borderRadius: '10px', background: 'rgba(245,158,11,0.10)', border: '1px solid rgba(245,158,11,0.25)' }}>
              <span style={{ width: '8px', height: '8px', borderRadius: '50%', background: '#f59e0b', flexShrink: 0 }} />
              <span style={{ fontSize: '12px', fontWeight: 600, color: '#92400e' }}>
                {pendingCount} submission{pendingCount !== 1 ? 's' : ''} need review
              </span>
            </div>
          )}
        </div>
      </div>

      {error && (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      )}

      {/* Tabs */}
      <div style={{ display: 'flex', gap: '2px', marginBottom: '24px', background: 'var(--bg-elevated)', borderRadius: '12px', padding: '5px', maxWidth: '360px', border: '1px solid var(--border)', animation: 'fadeUp 0.35s ease both' }}>
        {([['types', 'Document Types'], ['submissions', 'Submissions']] as const).map(([key, label]) => (
          <button key={key} onClick={() => setTab(key)} style={{
            flex: 1, padding: '8px 16px', borderRadius: '8px', border: 'none', cursor: 'pointer',
            background: tab === key ? 'var(--accent)' : 'transparent',
            color: tab === key ? '#fff' : 'var(--text-subtle)',
            fontSize: '13px', fontWeight: tab === key ? 600 : 400,
            fontFamily: 'var(--font-body)',
            transition: 'all 150ms ease',
          }}>
            {label}
            {key === 'submissions' && pendingCount > 0 && (
              <span style={{ marginLeft: '6px', padding: '1px 6px', borderRadius: '999px', fontSize: '10px', background: tab === 'submissions' ? 'rgba(255,255,255,0.25)' : 'rgba(245,158,11,0.2)', color: tab === 'submissions' ? '#fff' : '#92400e', fontWeight: 700 }}>
                {pendingCount}
              </span>
            )}
          </button>
        ))}
      </div>

      {loading ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', animation: 'fadeUp 0.4s ease both' }}>
          {[1, 2, 3, 4].map(i => (
            <div key={i} style={{ height: '52px', borderRadius: '10px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', opacity: 1 - i * 0.15 }} />
          ))}
        </div>
      ) : tab === 'types' ? (
        /* ── Document Types Tab ─────────────────────────────────────────────── */
        <div style={{ animation: 'fadeUp 0.35s ease both' }}>
          <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '14px' }}>
            <button
              onClick={() => setEditingType('new')}
              style={{ display: 'flex', alignItems: 'center', gap: '7px', padding: '9px 18px', borderRadius: '10px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer' }}
            >
              <span style={{ fontSize: '16px', lineHeight: 1 }}>+</span> New Document Type
            </button>
          </div>

          {docTypes.length === 0 ? (
            <div className="card" style={{ padding: '40px', textAlign: 'center' }}>
              <p style={{ fontFamily: 'var(--font-display)', fontSize: '20px', color: 'var(--text-subtle)', marginBottom: '8px' }}>No document types defined yet</p>
              <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Create types to define what documents households must provide.</p>
            </div>
          ) : (
            <div className="card" style={{ overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
                    {['Name', 'Category', 'Applies To', 'Expiry', 'Required', 'Status', ''].map(h => (
                      <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', whiteSpace: 'nowrap' }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {docTypes.map((dt, i) => (
                    <tr key={dt.id} style={{ borderBottom: i < docTypes.length - 1 ? '1px solid var(--border)' : 'none', transition: 'background 120ms' }}
                      onMouseEnter={e => (e.currentTarget.style.background = 'var(--bg-elevated)')}
                      onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}>
                      <td style={{ padding: '13px 16px' }}>
                        <div style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>{dt.name}</div>
                        {dt.description && <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{dt.description}</div>}
                      </td>
                      <td style={{ padding: '13px 16px' }}><Pill value={dt.category} map={CATEGORY_COLORS} /></td>
                      <td style={{ padding: '13px 16px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{dt.applies_to}</td>
                      <td style={{ padding: '13px 16px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>
                        {dt.expiry_months ? `${dt.expiry_months}mo` : 'Never'}
                      </td>
                      <td style={{ padding: '13px 16px' }}>
                        <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: dt.is_required ? '#059669' : 'var(--text-muted)' }}>
                          {dt.is_required ? 'yes' : 'no'}
                        </span>
                      </td>
                      <td style={{ padding: '13px 16px' }}>
                        <span style={{
                          display: 'inline-block', padding: '2px 9px', borderRadius: '999px', fontSize: '11px', fontFamily: 'var(--font-mono)',
                          background: dt.is_active ? 'rgba(16,185,129,0.10)' : 'rgba(107,114,128,0.10)',
                          color: dt.is_active ? '#065f46' : '#6b7280',
                        }}>
                          {dt.is_active ? 'active' : 'inactive'}
                        </span>
                      </td>
                      <td style={{ padding: '13px 16px', textAlign: 'right' }}>
                        <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
                          <button onClick={() => setEditingType(dt)} style={{ padding: '5px 12px', borderRadius: '7px', border: '1px solid var(--border)', background: 'transparent', color: 'var(--text-subtle)', fontSize: '12px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>
                            Edit
                          </button>
                          <button
                            onClick={() => setDeletingTypeId(dt.id)}
                            style={{ padding: '5px 12px', borderRadius: '7px', border: '1px solid rgba(239,68,68,0.3)', background: 'transparent', color: '#dc2626', fontSize: '12px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}
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
          )}
        </div>
      ) : (
        /* ── Submissions Tab ────────────────────────────────────────────────── */
        <div style={{ animation: 'fadeUp 0.35s ease both' }}>
          {/* Status filter */}
          <div style={{ display: 'flex', gap: '6px', marginBottom: '16px', flexWrap: 'wrap' }}>
            {STATUS_FILTERS.map(sf => (
              <button key={sf} onClick={() => setStatusFilter(sf)} style={{
                padding: '5px 14px', borderRadius: '999px', border: `1px solid ${statusFilter === sf ? 'var(--accent)' : 'var(--border)'}`,
                background: statusFilter === sf ? 'var(--accent)' : 'transparent',
                color: statusFilter === sf ? '#fff' : 'var(--text-subtle)',
                fontSize: '12px', fontWeight: statusFilter === sf ? 600 : 400,
                cursor: 'pointer', fontFamily: 'var(--font-body)', transition: 'all 120ms ease',
                textTransform: 'capitalize',
              }}>
                {sf === 'all' ? `All (${submissions.length})` : sf}
              </button>
            ))}
          </div>

          {filteredSubs.length === 0 ? (
            <div className="card" style={{ padding: '40px', textAlign: 'center' }}>
              <p style={{ fontFamily: 'var(--font-display)', fontSize: '20px', color: 'var(--text-subtle)', marginBottom: '8px' }}>No submissions</p>
              <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
                {statusFilter === 'all' ? 'Households have not submitted any documents yet.' : `No ${statusFilter} submissions found.`}
              </p>
            </div>
          ) : (
            <div className="card" style={{ overflow: 'hidden' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
                    {['Document Type', 'Household', 'Swimmer', 'Status', 'Submitted', 'Expires', 'Actions'].map(h => (
                      <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', whiteSpace: 'nowrap' }}>{h}</th>
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
                        <td style={{ padding: '12px 16px' }}>
                          <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{typeName}</div>
                          {sub.file_name && <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px', fontFamily: 'var(--font-mono)' }}>{sub.file_name}</div>}
                        </td>
                        <td style={{ padding: '12px 16px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>
                          {sub.household_id.slice(0, 8)}…
                        </td>
                        <td style={{ padding: '12px 16px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>
                          {sub.swimmer_id ? sub.swimmer_id.slice(0, 8) + '…' : '—'}
                        </td>
                        <td style={{ padding: '12px 16px' }}><StatusPill status={sub.status} /></td>
                        <td style={{ padding: '12px 16px', fontSize: '12px', color: 'var(--text-subtle)' }}>{fmt(sub.submitted_at)}</td>
                        <td style={{ padding: '12px 16px', fontSize: '12px', color: sub.expires_at && new Date(sub.expires_at) < new Date() ? '#dc2626' : 'var(--text-subtle)' }}>
                          {fmt(sub.expires_at)}
                        </td>
                        <td style={{ padding: '12px 16px' }}>
                          {canReview ? (
                            <div style={{ display: 'flex', gap: '6px' }}>
                              <button
                                onClick={() => setReviewing({ id: sub.id, action: 'verify' })}
                                style={{ padding: '4px 10px', borderRadius: '6px', border: '1px solid rgba(16,185,129,0.4)', background: 'rgba(16,185,129,0.06)', color: '#059669', fontSize: '11px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}
                              >
                                ✓ Verify
                              </button>
                              <button
                                onClick={() => setReviewing({ id: sub.id, action: 'reject' })}
                                style={{ padding: '4px 10px', borderRadius: '6px', border: '1px solid rgba(239,68,68,0.3)', background: 'rgba(239,68,68,0.05)', color: '#dc2626', fontSize: '11px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}
                              >
                                ✗ Reject
                              </button>
                            </div>
                          ) : (
                            sub.review_notes ? (
                              <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontStyle: 'italic' }} title={sub.review_notes}>
                                {sub.review_notes.slice(0, 30)}{sub.review_notes.length > 30 ? '…' : ''}
                              </span>
                            ) : (
                              <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>—</span>
                            )
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Delete confirm */}
      {deletingTypeId && (
        <div style={{ position: 'fixed', inset: 0, zIndex: 1000, background: 'rgba(10,8,6,0.55)', backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px' }}>
          <div style={{ background: 'var(--bg-elevated)', borderRadius: '16px', border: '1px solid var(--border)', boxShadow: '0 20px 60px rgba(0,0,0,0.24)', padding: '28px', maxWidth: '380px', width: '100%', animation: 'modalIn 0.2s ease both' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '10px' }}>Delete document type?</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '22px' }}>This will soft-delete the type. Existing submissions will be unaffected.</p>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setDeletingTypeId(null)} style={{ padding: '8px 16px', borderRadius: '8px', border: '1px solid var(--border)', background: 'transparent', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer' }}>Cancel</button>
              <button onClick={() => void handleDeleteType(deletingTypeId)} style={{ padding: '8px 18px', borderRadius: '8px', border: 'none', background: '#dc2626', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer' }}>Delete</button>
            </div>
          </div>
        </div>
      )}

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
