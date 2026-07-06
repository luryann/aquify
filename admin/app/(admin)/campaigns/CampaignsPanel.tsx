'use client'

import { useRouter } from 'next/navigation'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  createCampaign,
  fetchCampaigns,
  fetchOrgSettings,
  type EmailCampaignRecord,
} from '@/lib/adminData'

function formatDateTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const FIELD_INPUT_STYLE: React.CSSProperties = {
  width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)',
  background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', boxSizing: 'border-box',
}

export function CampaignsPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const router = useRouter()
  const [campaigns, setCampaigns] = useState<EmailCampaignRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [form, setForm] = useState({
    name: '',
    subject: '',
    body_html: '<p></p>',
    body_text: '',
    from_name: '',
    from_email: '',
    audience: 'all',
  })

  async function loadCampaigns() {
    if (!session?.user.orgId) return
    const next = await fetchCampaigns(api, session.user.orgId)
    setCampaigns(next)
  }

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
        const [next, orgSettings] = await Promise.all([
          fetchCampaigns(api, session.user.orgId),
          fetchOrgSettings(api, session.user.orgId).catch(() => null),
        ])
        if (cancelled) return
        setCampaigns(next)
        if (orgSettings) {
          setForm((current) => ({
            ...current,
            from_name: current.from_name || orgSettings.email_sender_name || orgSettings.team_name || '',
            from_email: current.from_email || orgSettings.email_sender_address || '',
          }))
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load campaigns.')
          setCampaigns([])
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

  const stats = useMemo(() => ({
    total: campaigns.length,
    sent: campaigns.filter((campaign) => campaign.status === 'sent').length,
    draft: campaigns.filter((campaign) => campaign.status === 'draft').length,
    recipients: campaigns.reduce((sum, campaign) => sum + campaign.total_recipients, 0),
  }), [campaigns])

  const visibleCampaigns = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return campaigns
    return campaigns.filter((c) => `${c.name} ${c.subject}`.toLowerCase().includes(q))
  }, [campaigns, search])

  async function handleCreate() {
    if (!session?.user.orgId || !form.name.trim() || !form.subject.trim() || !form.body_html.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      const created = await createCampaign(api, session.user.orgId, {
        name: form.name.trim(),
        subject: form.subject.trim(),
        body_html: form.body_html.trim(),
        body_text: form.body_text.trim() || undefined,
        from_name: form.from_name.trim(),
        from_email: form.from_email.trim(),
        audience: form.audience,
      })
      setForm((current) => ({ ...current, name: '', subject: '', body_text: '' }))
      await loadCampaigns()
      setModalOpen(false)
      router.push(`/campaigns/${created.id}`)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to create campaign.')
    } finally {
      setSubmitting(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px 0' }}><EmptyState icon="mail" title="Sign in required" description="Authenticate to inspect email campaigns." /></div>
  }

  const columns = [
    col<EmailCampaignRecord>({ key: 'name', label: 'Campaign', sortable: true }),
    col<EmailCampaignRecord>({ key: 'subject', label: 'Subject', sortable: true, cellStyle: { color: 'var(--text-subtle)' } }),
    col<EmailCampaignRecord>({
      key: 'status',
      label: 'Status',
      sortable: true,
      render: (row) => (
        <span className={`badge ${row.status === 'sent' ? 'badge-success' : row.status === 'draft' ? 'badge-warning' : 'badge-default'}`}>
          {row.status}
        </span>
      ),
    }),
    col<EmailCampaignRecord>({
      key: 'total_recipients',
      label: 'Recipients',
      sortable: true,
      render: (row) => (
        <span style={{ color: 'var(--text-subtle)' }}>
          {row.total_recipients} · sent {row.sent_count} · failed {row.failed_count} · suppressed {row.suppressed_count}
        </span>
      ),
    }),
    col<EmailCampaignRecord>({
      key: 'sent_at',
      label: 'Sent',
      sortable: true,
      render: (row) => formatDateTime(row.sent_at),
    }),
  ]

  return (
    <div>
      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 220px', maxWidth: '320px' }}>
          <span style={{ position: 'absolute', left: '11px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
            <Icon name="search" size={14} />
          </span>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search campaigns…"
            style={{ ...FIELD_INPUT_STYLE, paddingLeft: '32px' }}
          />
        </div>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <span className="badge badge-default">{stats.total} campaigns</span>
          <span className="badge badge-success">{stats.sent} sent</span>
          <span className="badge badge-warning">{stats.draft} draft</span>
          <span className="badge badge-default">{stats.recipients} recipients</span>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary" style={{ marginLeft: 'auto' }}>
          <Icon name="plus" size={14} />
          New Campaign
        </button>
      </div>

      <Table
        columns={columns}
        data={visibleCampaigns}
        keyField="id"
        loading={loading}
        paginated
        pageSize={20}
        onRowClick={(row) => router.push(`/campaigns/${row.id}`)}
        emptyTitle={campaigns.length === 0 ? 'No campaigns yet' : 'No matches'}
        emptyDescription={campaigns.length === 0 ? 'Create a draft to start emailing your club.' : 'Try a different search.'}
      />

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="New Campaign"
        description="Creates a draft — you'll review and send it from the campaign page."
        width={560}
        footer={
          <>
            <button onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button onClick={() => void handleCreate()} disabled={submitting || !form.name.trim() || !form.subject.trim() || !form.body_html.trim()} className="btn btn-primary">
              {submitting ? 'Creating…' : 'Create Draft'}
            </button>
          </>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          <input value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder="Campaign name" style={FIELD_INPUT_STYLE} />
          <input value={form.subject} onChange={(event) => setForm((current) => ({ ...current, subject: event.target.value }))} placeholder="Subject line" style={FIELD_INPUT_STYLE} />
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
            <input value={form.from_name} onChange={(event) => setForm((current) => ({ ...current, from_name: event.target.value }))} placeholder="From name" style={FIELD_INPUT_STYLE} />
            <input value={form.from_email} onChange={(event) => setForm((current) => ({ ...current, from_email: event.target.value }))} placeholder="From address" type="email" style={FIELD_INPUT_STYLE} />
          </div>
          <select
            value={form.audience}
            onChange={(event) => setForm((current) => ({ ...current, audience: event.target.value }))}
            style={FIELD_INPUT_STYLE}
          >
            <option value="all">All members</option>
            <option value="group">Specific group</option>
          </select>
          <textarea value={form.body_html} onChange={(event) => setForm((current) => ({ ...current, body_html: event.target.value }))} rows={7} placeholder="Email body (HTML)" style={{ ...FIELD_INPUT_STYLE, fontFamily: 'var(--font-body)', resize: 'vertical' }} />
          <textarea value={form.body_text} onChange={(event) => setForm((current) => ({ ...current, body_text: event.target.value }))} rows={3} placeholder="Optional plain text fallback" style={{ ...FIELD_INPUT_STYLE, resize: 'vertical' }} />
          <p style={{ margin: 0, fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1.5 }}>
            Recipients are resolved automatically from the audience you choose once you send.
          </p>
        </div>
      </Modal>
    </div>
  )
}
