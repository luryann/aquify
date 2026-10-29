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
import { formatDateTime } from '@/lib/dates'

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
    return <div className="u-p-32px" ><EmptyState icon="megaphone" title="Sign in required" description="Authenticate to inspect live announcement details." /></div>
  }

  if (loading) {
    return <div className="u-p-32px" ><EmptyState icon="megaphone" title="Loading announcement" description="Fetching the live announcement record." /></div>
  }

  if (!announcement) {
    return <div className="u-p-32px" ><EmptyState icon="megaphone" title="Announcement not found" description={error ?? 'The requested announcement could not be loaded.'} /></div>
  }

  return (
    <div className="page">
      <div className="u-mb-20px" >
        <Link href="/announcements" className="u-fs-fs-sm" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>← Back to announcements</Link>
      </div>

      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      <div className="card u-p-24px" >
        <div className="u-gap-16px u-mb-16px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap' }}>
          <div>
            <div className="u-gap-12px u-mb-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
              <h1 className="page-title">
                {announcement.title}
              </h1>
              <span className="badge badge-default">{announcement.status}</span>
            </div>
            <div className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
              Audience: {announcement.audience} · Created {formatDateTime(announcement.created_at)} by {staffName(announcement.created_by)} · Updated {formatDateTime(announcement.updated_at)}
            </div>
          </div>
          <div className="u-gap-8px" style={{ display: 'flex' }}>
            {announcement.status === 'draft' ? (
              <button type="button" onClick={() => void handlePublish()} disabled={working} className="btn btn-primary">
                Publish
              </button>
            ) : null}
            {announcement.status !== 'archived' ? (
              <button type="button" onClick={() => void handleArchive()} disabled={working} className="u-p-8px-12px u-fs-fs-sm" style={{ borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', cursor: working ? 'not-allowed' : 'pointer' }}>
                Archive
              </button>
            ) : null}
          </div>
        </div>

        <div className="u-p-16px-20px u-fs-fs-md u-lh-1_7" style={{ borderRadius: '12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', color: 'var(--text-subtle)', whiteSpace: 'pre-wrap' }}>
          {announcement.body}
        </div>

        <div className="u-mt-16px u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
          Published {formatDateTime(announcement.published_at)} · Archived {formatDateTime(announcement.archived_at)} · Group {announcement.group_id || 'All'}
        </div>
      </div>
    </div>
  )
}
