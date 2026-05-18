'use client'

import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  archiveAnnouncement,
  createAnnouncement,
  fetchAnnouncements,
  publishAnnouncement,
  type AnnouncementRecord,
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

export default function AnnouncementsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [announcements, setAnnouncements] = useState<AnnouncementRecord[]>([])
  const [statusFilter, setStatusFilter] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [actionId, setActionId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [form, setForm] = useState({
    title: '',
    body: '',
    audience: 'all',
  })

  async function loadAnnouncements(selectedStatus = statusFilter) {
    if (!session?.user.orgId) return
    const next = await fetchAnnouncements(api, session.user.orgId, selectedStatus || undefined)
    setAnnouncements(next)
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
        const next = await fetchAnnouncements(api, session.user.orgId, statusFilter || undefined)
        if (!cancelled) setAnnouncements(next)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load announcements.')
          setAnnouncements([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId, statusFilter])

  const stats = useMemo(() => ({
    total: announcements.length,
    published: announcements.filter((announcement) => announcement.status === 'published').length,
    draft: announcements.filter((announcement) => announcement.status === 'draft').length,
    archived: announcements.filter((announcement) => announcement.status === 'archived').length,
  }), [announcements])

  async function handleCreate() {
    if (!session?.user.orgId || !form.title.trim() || !form.body.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      await createAnnouncement(api, session.user.orgId, {
        title: form.title.trim(),
        body: form.body.trim(),
        audience: form.audience,
      })
      setForm({ title: '', body: '', audience: 'all' })
      await loadAnnouncements()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to create announcement.')
    } finally {
      setSubmitting(false)
    }
  }

  async function handlePublish(id: string) {
    if (!session?.user.orgId) return
    setActionId(id)
    setError(null)
    try {
      await publishAnnouncement(api, session.user.orgId, id)
      await loadAnnouncements()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to publish announcement.')
    } finally {
      setActionId(null)
    }
  }

  async function handleArchive(id: string) {
    if (!session?.user.orgId) return
    setActionId(id)
    setError(null)
    try {
      await archiveAnnouncement(api, session.user.orgId, id)
      await loadAnnouncements()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to archive announcement.')
    } finally {
      setActionId(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="megaphone" title="Sign in required" description="Authenticate to manage live announcements." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1180, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Communications
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Announcements
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Draft, publish, and archive announcements using the live communications service.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total', value: String(stats.total) },
          { label: 'Published', value: String(stats.published) },
          { label: 'Drafts', value: String(stats.draft) },
          { label: 'Archived', value: String(stats.archived) },
        ].map((stat) => (
          <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '340px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '18px' }}>
          <h2 style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '14px' }}>New Announcement</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <input
              value={form.title}
              onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
              placeholder="Title"
              style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px' }}
            />
            <select
              value={form.audience}
              onChange={(event) => setForm((current) => ({ ...current, audience: event.target.value }))}
              style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px' }}
            >
              <option value="all">All members</option>
              <option value="group">Specific group</option>
            </select>
            <textarea
              value={form.body}
              onChange={(event) => setForm((current) => ({ ...current, body: event.target.value }))}
              rows={8}
              placeholder="Write the announcement body."
              style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', resize: 'vertical' }}
            />
            <button
              onClick={() => void handleCreate()}
              disabled={submitting || !form.title.trim() || !form.body.trim()}
              style={{ padding: '9px 14px', borderRadius: '8px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '12px', fontWeight: 600, cursor: submitting ? 'not-allowed' : 'pointer' }}
            >
              {submitting ? 'Creating…' : 'Create Draft'}
            </button>
          </div>
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Live Announcement Feed</h2>
            <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)} style={{ padding: '8px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '12px' }}>
              <option value="">All statuses</option>
              <option value="draft">Draft</option>
              <option value="published">Published</option>
              <option value="archived">Archived</option>
            </select>
          </div>
          {loading ? (
            <div style={{ padding: '20px' }}><EmptyState icon="megaphone" title="Loading announcements" description="Fetching the live communications feed." /></div>
          ) : announcements.length === 0 ? (
            <div style={{ padding: '20px' }}><EmptyState icon="megaphone" title="No announcements yet" description="Create a draft to populate this feed." /></div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column' }}>
              {announcements.map((announcement, index) => (
                <div key={announcement.id} style={{ padding: '16px 18px', borderBottom: index < announcements.length - 1 ? '1px solid var(--border)' : 'none' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'flex-start', marginBottom: '8px' }}>
                    <div>
                      <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap', marginBottom: '4px' }}>
                        <Link href={`/announcements/${announcement.id}`} style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>
                          {announcement.title}
                        </Link>
                        <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: announcement.status === 'published' ? 'var(--success)' : announcement.status === 'draft' ? 'var(--warning)' : 'var(--text-muted)', background: 'var(--bg-surface)', border: '1px solid var(--border)', padding: '2px 8px', borderRadius: '999px' }}>
                          {announcement.status}
                        </span>
                      </div>
                      <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                        Audience: {announcement.audience} · Created {formatDateTime(announcement.created_at)} · Updated {formatDateTime(announcement.updated_at)}
                      </div>
                    </div>
                    <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                      {announcement.status === 'draft' ? (
                        <button
                          onClick={() => void handlePublish(announcement.id)}
                          disabled={actionId === announcement.id}
                          style={{ padding: '6px 10px', borderRadius: '6px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '12px', cursor: actionId === announcement.id ? 'not-allowed' : 'pointer' }}
                        >
                          Publish
                        </button>
                      ) : null}
                      {announcement.status !== 'archived' ? (
                        <button
                          onClick={() => void handleArchive(announcement.id)}
                          disabled={actionId === announcement.id}
                          style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '12px', cursor: actionId === announcement.id ? 'not-allowed' : 'pointer' }}
                        >
                          Archive
                        </button>
                      ) : null}
                    </div>
                  </div>
                  <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.6, margin: 0 }}>
                    {announcement.body}
                  </p>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
