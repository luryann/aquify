'use client'

import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Icon } from '@/components/ui/icons'
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

const FIELD_INPUT_STYLE: React.CSSProperties = {
  width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)',
  background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', boxSizing: 'border-box',
}

export default function AnnouncementsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [announcements, setAnnouncements] = useState<AnnouncementRecord[]>([])
  const [statusFilter, setStatusFilter] = useState('')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [actionId, setActionId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
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

  const visibleAnnouncements = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return announcements
    return announcements.filter((a) => `${a.title} ${a.body}`.toLowerCase().includes(q))
  }, [announcements, search])

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
      setModalOpen(false)
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
    <div style={{ padding: '32px 36px 80px', maxWidth: 1240, margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', marginBottom: '24px', flexWrap: 'wrap' }}>
        <div>
          <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
            Communications
          </p>
          <h1 className="page-title">Announcements</h1>
          <p className="page-subtitle">Draft, publish, and archive announcements for your club.</p>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary">
          <Icon name="plus" size={14} />
          New Announcement
        </button>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '20px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 220px', maxWidth: '320px' }}>
          <span style={{ position: 'absolute', left: '11px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
            <Icon name="search" size={14} />
          </span>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search announcements…"
            style={{ ...FIELD_INPUT_STYLE, paddingLeft: '32px' }}
          />
        </div>
        <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)} style={{ ...FIELD_INPUT_STYLE, width: 'auto' }}>
          <option value="">All statuses</option>
          <option value="draft">Draft</option>
          <option value="published">Published</option>
          <option value="archived">Archived</option>
        </select>
        <div style={{ display: 'flex', gap: '8px', marginLeft: 'auto', flexWrap: 'wrap' }}>
          <span className="badge badge-default">{stats.total} total</span>
          <span className="badge badge-success">{stats.published} published</span>
          <span className="badge badge-warning">{stats.draft} draft</span>
          <span className="badge badge-default">{stats.archived} archived</span>
        </div>
      </div>

      <div className="card" style={{ overflow: 'hidden' }}>
        {loading ? (
          <div style={{ padding: '20px' }}><EmptyState icon="megaphone" title="Loading announcements" description="Fetching the club's announcement feed." /></div>
        ) : visibleAnnouncements.length === 0 ? (
          <div style={{ padding: '20px' }}>
            <EmptyState
              icon="megaphone"
              title={announcements.length === 0 ? 'No announcements yet' : 'No matches'}
              description={announcements.length === 0 ? 'Create one to let parents and swimmers know what\'s happening.' : 'Try a different search or filter.'}
            />
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {visibleAnnouncements.map((announcement, index) => (
              <div key={announcement.id} style={{ padding: '16px 18px', borderBottom: index < visibleAnnouncements.length - 1 ? '1px solid var(--border)' : 'none' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'flex-start', marginBottom: '8px' }}>
                  <div>
                    <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap', marginBottom: '4px' }}>
                      <Link href={`/announcements/${announcement.id}`} style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none' }}>
                        {announcement.title}
                      </Link>
                      <span className={`badge ${announcement.status === 'published' ? 'badge-success' : announcement.status === 'draft' ? 'badge-warning' : 'badge-default'}`}>
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
                        className="btn btn-sm btn-primary"
                      >
                        Publish
                      </button>
                    ) : null}
                    {announcement.status !== 'archived' ? (
                      <button
                        onClick={() => void handleArchive(announcement.id)}
                        disabled={actionId === announcement.id}
                        className="btn btn-sm btn-secondary"
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

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="New Announcement"
        description="Announcements post as drafts first — publish when you're ready for members to see them."
        footer={
          <>
            <button onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button
              onClick={() => void handleCreate()}
              disabled={submitting || !form.title.trim() || !form.body.trim()}
              className="btn btn-primary"
            >
              {submitting ? 'Creating…' : 'Create Draft'}
            </button>
          </>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          <input
            value={form.title}
            onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
            placeholder="Title"
            style={FIELD_INPUT_STYLE}
          />
          <select
            value={form.audience}
            onChange={(event) => setForm((current) => ({ ...current, audience: event.target.value }))}
            style={FIELD_INPUT_STYLE}
          >
            <option value="all">All members</option>
            <option value="group">Specific group</option>
          </select>
          <textarea
            value={form.body}
            onChange={(event) => setForm((current) => ({ ...current, body: event.target.value }))}
            rows={8}
            placeholder="Write the announcement body."
            style={{ ...FIELD_INPUT_STYLE, resize: 'vertical' }}
          />
        </div>
      </Modal>
    </div>
  )
}
