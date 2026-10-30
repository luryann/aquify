'use client'

import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Icon } from '@/components/ui/icons'
import { useToast } from '@/components/ui/Toast'
import { useSession } from '@/lib/session'
import { setAnnouncementPublic } from '@/lib/publicSiteData'
import { useApiClient } from '@/lib/api'
import {
  archiveAnnouncement,
  createAnnouncement,
  fetchAnnouncements,
  publishAnnouncement,
  type AnnouncementRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { formatDateTime } from '@/lib/dates'
import { FIELD_INPUT_STYLE } from '@/lib/form-styles'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

export default function AnnouncementsPage() {
  const toast = useToast()
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
    public: false,
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
          setError(uiErrorMessage(err, 'Couldn’t load announcements. Try again.'))
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
        public: form.audience === 'all' && form.public,
      })
      setForm({ title: '', body: '', audience: 'all', public: false })
      await loadAnnouncements()
      setModalOpen(false)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t create announcement. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  async function runAction(id: string, action: (orgId: string) => Promise<unknown>, failureMessage: string) {
    if (!session?.user.orgId) return
    setActionId(id)
    setError(null)
    try {
      await action(session.user.orgId)
      await loadAnnouncements()
    } catch (err) {
      setError(uiErrorMessage(err, failureMessage))
    } finally {
      setActionId(null)
    }
  }

  const handlePublish = (id: string) => runAction(id, (orgId) => publishAnnouncement(api, orgId, id), 'Couldn’t publish announcement. Try again.')

  async function handleTogglePublic(id: string, next: boolean) {
    if (!session?.user.orgId) return
    setActionId(id)
    setError(null)
    try {
      const orgId = session.user.orgId
      await setAnnouncementPublic(api, orgId, id, next)
      await loadAnnouncements()
      toast.successWithAction(next ? 'Shown on the public website' : 'Hidden from the public website', {
        label: 'Undo',
        onClick: () => {
          void setAnnouncementPublic(api, orgId, id, !next).then(() => loadAnnouncements()).catch((err) => setError(uiErrorMessage(err, 'Couldn’t undo. Try again.')))
        },
      })
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t update website visibility. Try again.'))
    } finally {
      setActionId(null)
    }
  }

  const handleArchive = (id: string) => runAction(id, (orgId) => archiveAnnouncement(api, orgId, id), 'Couldn’t archive announcement. Try again.')

  if (!session) {
    return <div className="u-p-32px" ><EmptyState icon="megaphone" title="Sign in required" description="Authenticate to manage live announcements." /></div>
  }

  return (
    <div className="page">
      <div className="u-gap-16px u-mb-24px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <div>
          <h1 className="page-title">Announcements</h1>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary">
          <Icon name="plus" size={14} />
          New announcement
        </button>
      </div>

      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      <div className="u-gap-12px u-mb-20px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 220px', maxWidth: '320px' }}>
          <span style={{ position: 'absolute', left: '11px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
            <Icon name="search" size={14} />
          </span>
          <input aria-label="Search announcements" className="input-search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search announcements…"
            style={FIELD_INPUT_STYLE}
          />
        </div>
        <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)} style={{ ...FIELD_INPUT_STYLE, width: 'auto' }}>
          <option value="">All statuses</option>
          <option value="draft">Draft</option>
          <option value="published">Published</option>
          <option value="archived">Archived</option>
        </select>
        <div className="u-gap-8px u-ml-auto" style={{ display: 'flex', flexWrap: 'wrap' }}>
          <span className="badge badge-default">{stats.total} total</span>
          <span className="badge badge-success">{stats.published} published</span>
          <span className="badge badge-warning">{stats.draft} draft</span>
          <span className="badge badge-default">{stats.archived} archived</span>
        </div>
      </div>

      <div className="card" style={{ overflow: 'hidden' }}>
        {loading ? (
          <div className="u-p-20px" ><PageSkeleton variant="table" noHeader rows={4} /></div>
        ) : visibleAnnouncements.length === 0 ? (
          <div className="u-p-20px" >
            <EmptyState
              icon="megaphone"
              title={announcements.length === 0 ? 'No announcements yet' : 'No matches'}
              description={announcements.length === 0 ? 'Create one to let parents and swimmers know what\'s happening.' : 'Try a different search or filter.'}
            />
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {visibleAnnouncements.map((announcement, index) => (
              <div key={announcement.id} className="u-p-16px-20px" style={{ borderBottom: index < visibleAnnouncements.length - 1 ? '1px solid var(--border)' : 'none' }}>
                <div className="u-mb-8px" style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px 12px', alignItems: 'flex-start' }}>
                  <div style={{ flex: '1 1 260px', minWidth: 0 }}>
                    <div className="u-gap-8px u-mb-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
                      <Link href={`/announcements/${announcement.id}`} className="u-fs-fs-md u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}>
                        {announcement.title}
                      </Link>
                      <span className={`badge ${announcement.status === 'published' ? 'badge-success' : announcement.status === 'draft' ? 'badge-warning' : 'badge-default'}`}>
                        {announcement.status}
                      </span>
                      {announcement.public ? <span className="badge badge-info">On website</span> : null}
                    </div>
                    <div className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
                      Audience: {announcement.audience} · Created {formatDateTime(announcement.created_at)} · Updated {formatDateTime(announcement.updated_at)}
                    </div>
                  </div>
                  <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                    {announcement.status === 'draft' ? (
                      <button
                        onClick={() => void handlePublish(announcement.id)}
                        disabled={actionId === announcement.id}
                        className="btn btn-sm btn-primary"
                      >
                        Publish
                      </button>
                    ) : null}
                    {announcement.status !== 'archived' && announcement.audience === 'all' ? (
                      <button
                        onClick={() => void handleTogglePublic(announcement.id, !announcement.public)}
                        disabled={actionId === announcement.id}
                        className="btn btn-sm btn-secondary"
                      >
                        {announcement.public ? 'Remove from website' : 'Show on website'}
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
                <p className="u-fs-fs-base u-lh-1_6 u-m-0" style={{ color: 'var(--text-subtle)' }}>
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
        title="New announcement"
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
        <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
          <input aria-label="Title"
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
          {form.audience === 'all' ? (
            <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)' }}>
              <input type="checkbox" checked={form.public} onChange={(event) => setForm((current) => ({ ...current, public: event.target.checked }))} />
              Show on the public team website once published
            </label>
          ) : null}
          <textarea aria-label="Write the announcement body."
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
