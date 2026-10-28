import type { AnnouncementRecord } from '@/lib/adminData'
import type { ApiClient } from '@/lib/api'

export type PublicClub = {
  name: string
  slug: string
  about?: string
  contact_email?: string
  contact_phone?: string
  join_url: string
}
export type PublicAnnouncement = { id: string; title: string; body: string; published_at?: string | null }
export type PublicMeet = {
  id: string
  name: string
  start_date?: string
  end_date?: string
  location?: string
  host_org?: string
  course: string
}
export type PublicEvent = {
  id: string
  title: string
  category: string
  description?: string
  location?: string
  starts_at: string
  ends_at?: string | null
  all_day: boolean
}
export type PublicRecord = {
  label: string
  course: string
  gender: string
  age_group: string
  distance: number
  stroke: string
  time_ms: number
  swimmer_name: string
  swim_date: string
  meet_name?: string
}
export type PublicCoach = { name: string; role: string }
export type PublicSite = {
  club: PublicClub
  announcements: PublicAnnouncement[]
  meets: PublicMeet[]
  events: PublicEvent[]
  records: PublicRecord[]
  coaches: PublicCoach[]
}

/** Server-side fetch of the public team site; null when the site is disabled or unknown. */
export async function fetchPublicSite(slug: string): Promise<PublicSite | null> {
  const base = process.env.API_INTERNAL_URL ?? process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
  try {
    const res = await fetch(`${base}/api/v1/public/orgs/${encodeURIComponent(slug)}/site`, { next: { revalidate: 60 } })
    if (res.status === 404) return null
    if (!res.ok) throw new Error(`public site ${res.status}`)
    return (await res.json()) as PublicSite
  } catch (err) {
    if (err instanceof Error && err.message.startsWith('public site')) throw err
    throw new Error('public site unreachable')
  }
}

export async function setAnnouncementPublic(api: ApiClient, orgId: string, announcementId: string, isPublic: boolean): Promise<AnnouncementRecord> {
  return api.patch<AnnouncementRecord>(`/api/v1/announcements/${announcementId}`, { public: isPublic }, { 'X-Org-Id': orgId })
}

/** "1:02.45" style swim time from milliseconds. */
export function formatSwimTime(ms: number): string {
  const totalCs = Math.round(ms / 10)
  const cs = totalCs % 100
  const totalSec = Math.floor(totalCs / 100)
  const sec = totalSec % 60
  const min = Math.floor(totalSec / 60)
  const tail = `${String(cs).padStart(2, '0')}`
  return min > 0 ? `${min}:${String(sec).padStart(2, '0')}.${tail}` : `${sec}.${tail}`
}
