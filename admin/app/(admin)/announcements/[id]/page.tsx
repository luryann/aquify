'use client'

import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useEffect, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { useStaffNames } from '@/lib/useStaffNames'
import {
  archiveAnnouncement,
  fetchAnnouncement,
  publishAnnouncement,
  type AnnouncementRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

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

export default function AnnouncementDetailPage() {
  const params = useParams<{ id: string }>()
  const announcementId = Array.isArray(params?.id) ? params.id[0] : params?.id
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const staffName = useStaffNames(api, session?.user.orgId)
  const [announcement, setAnnouncement] = useState<AnnouncementRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [working, setWorking] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId || !announcementId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const next = await fetchAnnouncement(api, session.user.orgId, announcementId)
        if (!cancelled) setAnnouncement(next)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load announcement. Try again.'))
          setAnnouncement(null)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [announcementId, api, session?.user.orgId])

  async function handlePublish() {
    if (!session?.user.orgId || !announcement) return
    setWorking(true)
    setError(null)
    try {
      const next = await publishAnnouncement(api, session.user.orgId, announcement.id)
      setAnnouncement(next)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t publish announcement. Try again.'))
    } finally {
      setWorking(false)
    }
  }

  async function handleArchive() {
    if (!session?.user.orgId || !announcement) return
    setWorking(true)
    setError(null)
    try {
      const next = await archiveAnnouncement(api, session.user.orgId, announcement.id)
      setAnnouncement(next)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t archive announcement. Try again.'))
    } finally {
      setWorking(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="megaphone" title="Sign in required" description="Authenticate to inspect live announcement details." /></div>
  }

  if (loading) {
    return <div style={{ padding: '32px' }}><EmptyState icon="megaphone" title="Loading announcement" description="Fetching the live announcement record." /></div>
  }

  if (!announcement) {
    return <div style={{ padding: '32px' }}><EmptyState icon="megaphone" title="Announcement not found" description={error ?? 'The requested announcement could not be loaded.'} /></div>
  }

  return (
    <div className="page">
      <div style={{ marginBottom: '20px' }}>
        <Link href="/announcements" style={{ fontSize: 'var(--fs-sm)', color: 'var(--accent)', textDecoration: 'none' }}>← Back to announcements</Link>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>
          {error}
        </div>
      ) : null}

      <div className="card" style={{ padding: '24px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: '16px', alignItems: 'flex-start', marginBottom: '16px', flexWrap: 'wrap' }}>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap', marginBottom: '4px' }}>
              <h1 className="page-title">
                {announcement.title}
              </h1>
              <span className="badge badge-default">{announcement.status}</span>
            </div>
            <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
              Audience: {announcement.audience} · Created {formatDateTime(announcement.created_at)} by {staffName(announcement.created_by)} · Updated {formatDateTime(announcement.updated_at)}
            </div>
          </div>
          <div style={{ display: 'flex', gap: '8px' }}>
            {announcement.status === 'draft' ? (
              <button onClick={() => void handlePublish()} disabled={working} className="btn btn-primary">
                Publish
              </button>
            ) : null}
            {announcement.status !== 'archived' ? (
              <button onClick={() => void handleArchive()} disabled={working} style={{ padding: '8px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: 'var(--fs-sm)', cursor: working ? 'not-allowed' : 'pointer' }}>
                Archive
              </button>
            ) : null}
          </div>
        </div>

        <div style={{ padding: '16px 20px', borderRadius: '12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', fontSize: 'var(--fs-md)', lineHeight: 1.7, color: 'var(--text-subtle)', whiteSpace: 'pre-wrap' }}>
          {announcement.body}
        </div>

        <div style={{ marginTop: '16px', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
          Published {formatDateTime(announcement.published_at)} · Archived {formatDateTime(announcement.archived_at)} · Group {announcement.group_id || 'All'}
        </div>
      </div>
    </div>
  )
}
