'use client'

import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  createCampaign,
  fetchCampaignDeliveries,
  fetchCampaigns,
  sendCampaign,
  type EmailCampaignRecord,
  type EmailDeliveryRecord,
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

export default function CampaignsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [campaigns, setCampaigns] = useState<EmailCampaignRecord[]>([])
  const [selectedCampaignId, setSelectedCampaignId] = useState<string | null>(null)
  const [deliveries, setDeliveries] = useState<EmailDeliveryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [form, setForm] = useState({
    name: '',
    subject: '',
    body_html: '<p>Hello from DARE Aquatics.</p>',
    body_text: '',
    from_name: 'DARE Aquatics',
    from_email: 'noreply@dareaquatics.org',
    audience: 'all',
  })

  async function loadCampaigns() {
    if (!session?.user.orgId) return
    const next = await fetchCampaigns(api, session.user.orgId)
    setCampaigns(next)
    if (!selectedCampaignId && next[0]) {
      setSelectedCampaignId(next[0].id)
    }
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
        const next = await fetchCampaigns(api, session.user.orgId)
        if (cancelled) return
        setCampaigns(next)
        if (!selectedCampaignId && next[0]) setSelectedCampaignId(next[0].id)
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
  }, [api, selectedCampaignId, session?.user.orgId])

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId || !selectedCampaignId) {
        setDeliveries([])
        return
      }
      try {
        const next = await fetchCampaignDeliveries(api, session.user.orgId, selectedCampaignId)
        if (!cancelled) setDeliveries(next)
      } catch {
        if (!cancelled) setDeliveries([])
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, selectedCampaignId, session?.user.orgId])

  const selectedCampaign = campaigns.find((campaign) => campaign.id === selectedCampaignId) ?? null
  const stats = useMemo(() => ({
    total: campaigns.length,
    sent: campaigns.filter((campaign) => campaign.status === 'sent').length,
    draft: campaigns.filter((campaign) => campaign.status === 'draft').length,
    recipients: campaigns.reduce((sum, campaign) => sum + campaign.total_recipients, 0),
  }), [campaigns])

  async function handleCreate() {
    if (!session?.user.orgId || !form.name.trim() || !form.subject.trim() || !form.body_html.trim()) return
    setSubmitting(true)
    setError(null)
    setNotice(null)
    try {
      await createCampaign(api, session.user.orgId, {
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
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to create campaign.')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleSend() {
    if (!session?.user.orgId || !selectedCampaign) return

    setSending(true)
    setError(null)
    setNotice(null)
    try {
      const sentCampaign = await sendCampaign(api, session.user.orgId, selectedCampaign.id)
      await loadCampaigns()
      const nextDeliveries = await fetchCampaignDeliveries(api, session.user.orgId, selectedCampaign.id)
      setDeliveries(nextDeliveries)
      setNotice(`Sent campaign to ${sentCampaign.total_recipients} eligible recipient${sentCampaign.total_recipients === 1 ? '' : 's'}.`)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to send campaign.')
    } finally {
      setSending(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="mail" title="Sign in required" description="Authenticate to inspect live email campaigns." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1240, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Communications
        </p>
        <h1 className="page-title">
          Email Campaigns
        </h1>
        <p className="page-subtitle">
          Live campaign drafts, sends, and delivery history from the communications service.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {notice ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', fontSize: '12px' }}>
          {notice}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Campaigns', value: String(stats.total) },
          { label: 'Sent', value: String(stats.sent) },
          { label: 'Drafts', value: String(stats.draft) },
          { label: 'Recipients', value: String(stats.recipients) },
        ].map((stat) => (
          <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '360px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '18px' }}>
          <h2 style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '14px' }}>Create Draft</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <input value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder="Campaign name" style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px' }} />
            <input value={form.subject} onChange={(event) => setForm((current) => ({ ...current, subject: event.target.value }))} placeholder="Subject line" style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px' }} />
            <textarea value={form.body_html} onChange={(event) => setForm((current) => ({ ...current, body_html: event.target.value }))} rows={7} style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', resize: 'vertical' }} />
            <textarea value={form.body_text} onChange={(event) => setForm((current) => ({ ...current, body_text: event.target.value }))} rows={3} placeholder="Optional plain text fallback" style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', resize: 'vertical' }} />
            <button onClick={() => void handleCreate()} disabled={submitting || !form.name.trim() || !form.subject.trim() || !form.body_html.trim()} className="btn btn-primary">
              {submitting ? 'Creating…' : 'Create Draft'}
            </button>
            <p style={{ margin: 0, fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1.5 }}>
              Draft creation is live here, and draft sends can run against the backend send endpoint when recipient resolution is allowed for the current environment.
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div className="card" style={{ overflow: 'hidden' }}>
            <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
              <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Campaign List</h2>
            </div>
            {loading ? (
              <div style={{ padding: '20px' }}><EmptyState icon="mail" title="Loading campaigns" description="Fetching live campaign records." /></div>
            ) : campaigns.length === 0 ? (
              <div style={{ padding: '20px' }}><EmptyState icon="mail" title="No campaigns yet" description="Create a draft to populate the campaign log." /></div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                {campaigns.map((campaign, index) => (
                  <button
                    key={campaign.id}
                    onClick={() => setSelectedCampaignId(campaign.id)}
                    style={{
                      width: '100%',
                      padding: '16px 18px',
                      border: 'none',
                      borderBottom: index < campaigns.length - 1 ? '1px solid var(--border)' : 'none',
                      background: selectedCampaignId === campaign.id ? 'var(--accent-subtle)' : 'transparent',
                      textAlign: 'left',
                      cursor: 'pointer',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '6px' }}>
                      <div>
                        <Link href={`/campaigns/${campaign.id}`} style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>
                          {campaign.name}
                        </Link>
                        <div style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '2px' }}>{campaign.subject}</div>
                      </div>
                      <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: campaign.status === 'sent' ? 'var(--success)' : campaign.status === 'draft' ? 'var(--warning)' : 'var(--text-muted)' }}>
                        {campaign.status}
                      </span>
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                      Recipients {campaign.total_recipients} · Sent {campaign.sent_count} · Failed {campaign.failed_count} · Suppressed {campaign.suppressed_count}
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="card" style={{ overflow: 'hidden' }}>
            <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
              <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>
                {selectedCampaign ? `Delivery Snapshot · ${selectedCampaign.name}` : 'Delivery Snapshot'}
              </h2>
            </div>
            {!selectedCampaign ? (
              <div style={{ padding: '20px' }}><EmptyState icon="mail" title="Select a campaign" description="Choose a campaign to inspect its live delivery state." /></div>
            ) : selectedCampaign.status === 'draft' ? (
              <div style={{ padding: '20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                  Status {selectedCampaign.status} · Created {formatDateTime(selectedCampaign.created_at)} · Sent {formatDateTime(selectedCampaign.sent_at)}
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                    Recipients are resolved server-side from the campaign audience and current org membership records.
                  </span>
                  <button
                    onClick={() => void handleSend()}
                    disabled={sending}
                    className="btn btn-primary"
                  >
                    {sending ? 'Sending…' : 'Send Campaign'}
                  </button>
                </div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1.5 }}>
                  Group campaigns send only to households with active or pending registrations in the selected group. Suppressed addresses are skipped automatically.
                </div>
              </div>
            ) : deliveries.length === 0 ? (
              <div style={{ padding: '20px' }}>
                <div style={{ marginBottom: '10px', fontSize: '12px', color: 'var(--text-muted)' }}>
                  Status {selectedCampaign.status} · Created {formatDateTime(selectedCampaign.created_at)} · Sent {formatDateTime(selectedCampaign.sent_at)}
                </div>
                <EmptyState icon="mail" title="No deliveries recorded" description="This campaign has not produced delivery rows yet." />
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                {deliveries.slice(0, 20).map((delivery, index) => (
                  <div key={delivery.id} style={{ padding: '14px 18px', borderBottom: index < Math.min(deliveries.length, 20) - 1 ? '1px solid var(--border)' : 'none' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '4px' }}>
                      <span style={{ fontSize: '13px', color: 'var(--text)' }}>{delivery.recipient_email}</span>
                      <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: delivery.status === 'delivered' ? 'var(--success)' : delivery.status === 'failed' ? 'var(--error)' : 'var(--text-subtle)' }}>{delivery.status}</span>
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                      Sent {formatDateTime(delivery.sent_at)} · Delivered {formatDateTime(delivery.delivered_at)} · Message {delivery.provider_message_id || '—'}
                    </div>
                    {delivery.error_message ? (
                      <div style={{ marginTop: '4px', fontSize: '11px', color: 'var(--error)' }}>{delivery.error_message}</div>
                    ) : null}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
