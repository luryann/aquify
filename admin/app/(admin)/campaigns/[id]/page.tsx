'use client'

import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  fetchCampaign,
  fetchCampaignDeliveries,
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

function campaignPreviewDocument(bodyHTML: string) {
  return `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <base target="_blank">
  <style>
    body { margin: 0; color: #334155; font: 14px/1.7 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    img { max-width: 100%; height: auto; }
    table { max-width: 100%; border-collapse: collapse; }
    a { color: #2563eb; }
  </style>
</head>
<body>${bodyHTML}</body>
</html>`
}

export default function CampaignDetailPage() {
  const params = useParams<{ id: string }>()
  const campaignId = Array.isArray(params?.id) ? params.id[0] : params?.id
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [campaign, setCampaign] = useState<EmailCampaignRecord | null>(null)
  const [deliveries, setDeliveries] = useState<EmailDeliveryRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  async function loadCampaignState(orgId: string, id: string) {
    const [nextCampaign, nextDeliveries] = await Promise.all([
      fetchCampaign(api, orgId, id),
      fetchCampaignDeliveries(api, orgId, id).catch(() => []),
    ])
    setCampaign(nextCampaign)
    setDeliveries(nextDeliveries)
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId || !campaignId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [nextCampaign, nextDeliveries] = await Promise.all([
          fetchCampaign(api, session.user.orgId, campaignId),
          fetchCampaignDeliveries(api, session.user.orgId, campaignId).catch(() => []),
        ])
        if (cancelled) return
        setCampaign(nextCampaign)
        setDeliveries(nextDeliveries)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load campaign.')
          setCampaign(null)
          setDeliveries([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, campaignId, session?.user.orgId])

  async function handleSend() {
    if (!session?.user.orgId || !campaign) return

    setSending(true)
    setError(null)
    setNotice(null)
    try {
      const sentCampaign = await sendCampaign(api, session.user.orgId, campaign.id)
      await loadCampaignState(session.user.orgId, campaign.id)
      setNotice(`Sent campaign to ${sentCampaign.total_recipients} eligible recipient${sentCampaign.total_recipients === 1 ? '' : 's'}.`)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to send campaign.')
    } finally {
      setSending(false)
    }
  }

  const deliverySummary = useMemo(() => ({
    delivered: deliveries.filter((delivery) => delivery.status === 'delivered').length,
    failed: deliveries.filter((delivery) => delivery.status === 'failed').length,
    suppressed: deliveries.filter((delivery) => delivery.status === 'suppressed').length,
  }), [deliveries])

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="mail" title="Sign in required" description="Authenticate to inspect live campaign details." /></div>
  }

  if (loading) {
    return <div style={{ padding: '32px' }}><EmptyState icon="mail" title="Loading campaign" description="Fetching the live campaign and delivery rows." /></div>
  }

  if (!campaign) {
    return <div style={{ padding: '32px' }}><EmptyState icon="mail" title="Campaign not found" description={error ?? 'The requested campaign could not be loaded.'} /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1080, margin: '0 auto' }}>
      <div style={{ marginBottom: '20px' }}>
        <Link href="/campaigns" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none' }}>← Back to campaigns</Link>
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

      <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
        <section className="card" style={{ padding: '24px' }}>
          <div style={{ marginBottom: '16px' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '8px' }}>
              {campaign.status} · {campaign.audience}
            </div>
            <h1 className="page-title">
              {campaign.name}
            </h1>
            <div style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '4px' }}>{campaign.subject}</div>
            <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
              Created {formatDateTime(campaign.created_at)} by {campaign.created_by} · Sent {formatDateTime(campaign.sent_at)}
            </div>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '18px' }}>
            {[
              { label: 'Recipients', value: String(campaign.total_recipients) },
              { label: 'Sent', value: String(campaign.sent_count) },
              { label: 'Failed', value: String(campaign.failed_count) },
              { label: 'Suppressed', value: String(campaign.suppressed_count) },
            ].map((stat) => (
              <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
                <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</div>
                <div style={{ fontSize: '22px', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>{stat.value}</div>
              </div>
            ))}
          </div>

          <div style={{ padding: '16px 18px', borderRadius: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
            <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '8px' }}>HTML Preview</div>
            <iframe
              title={`${campaign.name} HTML preview`}
              sandbox=""
              srcDoc={campaignPreviewDocument(campaign.body_html)}
              style={{ width: '100%', minHeight: 240, border: '1px solid var(--border)', borderRadius: '8px', background: '#fff' }}
            />
          </div>

          {campaign.status === 'draft' ? (
            <div style={{ padding: '16px 18px', borderRadius: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
              <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '8px' }}>Send Draft</div>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                <span style={{ fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1.5 }}>
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
            </div>
          ) : null}
        </section>

        <section className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Deliveries</h2>
            <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
              Delivered {deliverySummary.delivered} · Failed {deliverySummary.failed} · Suppressed {deliverySummary.suppressed}
            </span>
          </div>
          {deliveries.length === 0 ? (
            <div style={{ padding: '20px' }}><EmptyState icon="mail" title="No deliveries recorded" description="This campaign has not produced delivery rows yet." /></div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Recipient', 'Status', 'Sent', 'Delivered', 'Message'].map((header) => (
                    <th key={header} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {deliveries.map((delivery) => (
                  <tr key={delivery.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td style={{ padding: '11px 14px', fontSize: '13px', color: 'var(--text)' }}>{delivery.recipient_email}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-body)', fontSize: '11px', color: delivery.status === 'delivered' ? 'var(--success)' : delivery.status === 'failed' ? 'var(--error)' : 'var(--text-subtle)' }}>{delivery.status}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDateTime(delivery.sent_at)}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDateTime(delivery.delivered_at)}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: delivery.error_message ? 'var(--error)' : 'var(--text-muted)' }}>{delivery.provider_message_id || delivery.error_message || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      </div>
    </div>
  )
}
