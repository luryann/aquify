'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchSwimmerWellness } from '@/lib/engagementData'
import { localToday } from '@/lib/wellness-scale'
import { Icon } from '@/components/ui/icons'

/** Nudge on the swimmer dashboard until today's check-in exists. */
export function WellnessPromptCard({ api, orgId, swimmerId }: { api: ApiClient; orgId: string; swimmerId?: string }) {
  const [show, setShow] = useState(false)
  useEffect(() => {
    if (!orgId || !swimmerId) return
    let cancelled = false
    fetchSwimmerWellness(api, orgId, swimmerId, 2)
      .then((w) => { if (!cancelled) setShow(!w.checkins.some((c) => c.date === localToday())) })
      .catch(() => { /* the prompt is optional; stay hidden if the read fails */ })
    return () => { cancelled = true }
  }, [api, orgId, swimmerId])
  if (!show) return null
  return (
    <Link href="/swim/wellness" className="card" style={{
      display: 'flex', alignItems: 'center', gap: 16, padding: '16px 20px', marginBottom: 20, textDecoration: 'none',
      border: '1px solid var(--accent-border)', background: 'var(--accent-subtle)',
    }}>
      <Icon name="activity" size={20} style={{ color: 'var(--accent)', flexShrink: 0 }} />
      <span style={{ flex: 1, minWidth: 0 }}>
        <span style={{ display: 'block', fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)' }}>You haven’t checked in today</span>
        <span style={{ display: 'block', fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>Sleep, soreness, energy, stress and mood — about 20 seconds.</span>
      </span>
      <Icon name="chevron-right" size={16} style={{ color: 'var(--accent)' }} />
    </Link>
  )
}
