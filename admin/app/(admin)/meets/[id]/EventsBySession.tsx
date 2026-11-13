'use client'

import { useMemo, useState, type ReactNode } from 'react'
import { Icon } from '@/components/ui/icons'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { formatDateOnly } from '@/lib/dates'

export type SessionGroupEvent = {
  id: string
  sessionId: string
  eventNumber: string
  name: string
  gender: string
  ageGroup: string
  entries: number
}

export type SessionGroupMeta = {
  id: string
  name: string
  date: string
  startTime: string
}

const UNSCHEDULED = '__unscheduled'

const COLS: Column<SessionGroupEvent>[] = [
  { key: 'eventNumber', label: 'Event', width: '90px', render: (row) => <span className="u-ff-font-body u-fs-fs-sm u-fw-600">{row.eventNumber}</span> },
  { key: 'name', label: 'Name', render: (row) => <span className="u-fs-fs-base">{row.name}</span> },
  { key: 'ageGroup', label: 'Age / gender', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{row.gender} {row.ageGroup}</span> },
  { key: 'entries', label: 'Entries', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: row.entries > 0 ? 'var(--accent-text, var(--accent))' : 'var(--text-muted)' }}>{row.entries}</span> },
]

/** Events grouped under their session; each group collapses so hundreds of events stay navigable. */
export function EventsBySession({ events, sessions, formatTime, toolbarExtra }: {
  events: SessionGroupEvent[]
  sessions: SessionGroupMeta[]
  formatTime: (v?: string | null) => string
  toolbarExtra?: ReactNode
}) {
  const groups = useMemo(() => {
    const known = new Set(sessions.map((s) => s.id))
    const list = sessions.map((s) => ({ key: s.id, title: s.name, sub: [s.date ? formatDateOnly(s.date) : '', s.startTime ? `starts ${formatTime(s.startTime)}` : ''].filter(Boolean).join(' · '), events: events.filter((e) => e.sessionId === s.id) }))
    const orphans = events.filter((e) => !known.has(e.sessionId))
    if (orphans.length > 0) list.push({ key: UNSCHEDULED, title: 'Unscheduled', sub: '', events: orphans })
    return list.filter((g) => g.events.length > 0)
  }, [events, sessions, formatTime])

  const [open, setOpen] = useState<Set<string>>(() => new Set())
  const allOpen = groups.length > 0 && groups.every((g) => open.has(g.key))

  function toggle(key: string) {
    setOpen((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  if (groups.length === 0) return <EmptyState icon="calendar" title="No events" />

  return (
    <div>
      <div className="md-toolbar" style={{ justifyContent: 'space-between' }}>
        <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{events.length} events in {groups.length} session{groups.length === 1 ? '' : 's'}</span>
        <div className="md-actions">
          {toolbarExtra}
          <button type="button" className="btn btn-ghost u-fs-fs-sm" onClick={() => setOpen(allOpen ? new Set() : new Set(groups.map((g) => g.key)))}>
            {allOpen ? 'Collapse all' : 'Expand all'}
          </button>
        </div>
      </div>
      <div className="stack" style={{ gap: 'var(--space-3)' }}>
        {groups.map((g) => {
          const isOpen = open.has(g.key)
          const entryTotal = g.events.reduce((n, e) => n + e.entries, 0)
          return (
            <section key={g.key} className="md-group" data-open={isOpen}>
              <button type="button" className="md-group-head" aria-expanded={isOpen} aria-controls={`md-group-${g.key}`} onClick={() => toggle(g.key)}>
                <Icon name="chevron-right" size={14} className="md-group-chevron" />
                <span className="md-group-title">{g.title}</span>
                {g.sub ? <span className="md-group-sub plain-digits">{g.sub}</span> : null}
                <span className="md-group-count">{g.events.length} event{g.events.length === 1 ? '' : 's'}{entryTotal > 0 ? ` · ${entryTotal} entries` : ''}</span>
              </button>
              {isOpen ? (
                <div id={`md-group-${g.key}`}>
                  <Table columns={COLS} data={g.events} keyField="id" style={{ border: 0, borderRadius: 0 }} />
                </div>
              ) : null}
            </section>
          )
        })}
      </div>
    </div>
  )
}
