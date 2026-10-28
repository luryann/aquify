'use client'

import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { filterNavItems, type Role } from '@/lib/roles'
import { fetchHouseholds, fetchMeets, fetchSwimmers } from '@/lib/adminData'

export const OPEN_COMMAND_PALETTE_EVENT = 'aquify:open-command-palette'

type Item = { id: string; group: 'Go to' | 'Create' | 'Swimmers' | 'Meets' | 'Households'; label: string; meta?: string; href: string }

const MAX_PER_LIVE_GROUP = 5
const MAX_TOTAL = 20

const QUICK_ACTIONS: (Item & { roles: Role[] })[] = [
  { id: 'qa-invoice', group: 'Create', label: 'New invoice', href: '/billing?tab=invoices', roles: ['admin'] },
  { id: 'qa-meet', group: 'Create', label: 'New meet', href: '/meets/new', roles: ['admin', 'coach'] },
  { id: 'qa-swimmer', group: 'Create', label: 'New swimmer', href: '/members?tab=swimmers', roles: ['admin', 'coach'] },
]

type Live = { swimmers: Item[]; meets: Item[]; households: Item[] }

export function CommandPalette() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [active, setActive] = useState(0)
  const [live, setLive] = useState<Live | null>(null)
  const [liveFailed, setLiveFailed] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const returnRef = useRef<HTMLElement | null>(null)
  const uid = useId()
  const listId = `${uid}-list`

  const roles = useMemo<Role[]>(
    () => (session?.user.roles as Role[] | undefined) ?? (session?.user.role ? [session.user.role as Role] : []),
    [session?.user.roles, session?.user.role],
  )
  const orgId = session?.user.orgId

  const show = useCallback(() => {
    returnRef.current = document.activeElement as HTMLElement | null
    setOpen(true)
  }, [])
  const close = useCallback(() => {
    setOpen(false)
    setQuery('')
    setActive(0)
    returnRef.current?.focus?.()
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        if (open) close()
        else show()
      }
    }
    const onOpen = () => show()
    document.addEventListener('keydown', onKey)
    window.addEventListener(OPEN_COMMAND_PALETTE_EVENT, onOpen)
    return () => {
      document.removeEventListener('keydown', onKey)
      window.removeEventListener(OPEN_COMMAND_PALETTE_EVENT, onOpen)
    }
  }, [open, show, close])

  useEffect(() => {
    if (open) inputRef.current?.focus()
  }, [open])

  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(query.trim().toLowerCase()), 150)
    return () => window.clearTimeout(t)
  }, [query])

  // Load the searchable lists once, the first time the palette opens.
  useEffect(() => {
    if (!open || live || !orgId) return
    let cancelled = false
    const canMembers = roles.includes('admin') || roles.includes('coach')
    if (!canMembers) {
      setLive({ swimmers: [], meets: [], households: [] })
      return
    }
    void Promise.all([fetchSwimmers(api, orgId), fetchMeets(api, orgId), fetchHouseholds(api, orgId)])
      .then(([sw, mt, hh]) => {
        if (cancelled) return
        setLive({
          swimmers: sw.map((s) => ({ id: `s-${s.id}`, group: 'Swimmers', label: `${s.first_name} ${s.last_name}`, href: `/members/swimmers/${s.id}` })),
          meets: mt.map((m) => ({ id: `m-${m.id}`, group: 'Meets', label: m.name, meta: m.start_date ?? undefined, href: `/meets/${m.id}` })),
          households: hh.map((h) => ({ id: `h-${h.id}`, group: 'Households', label: h.household_name, href: `/members/households/${h.id}` })),
        })
      })
      .catch(() => {
        if (!cancelled) {
          setLiveFailed(true)
          setLive({ swimmers: [], meets: [], households: [] })
        }
      })
    return () => {
      cancelled = true
    }
  }, [open, live, orgId, api, roles])

  const results = useMemo<Item[]>(() => {
    const nav: Item[] = filterNavItems(roles).map((n) => ({ id: `n-${n.id}`, group: 'Go to', label: n.label, meta: n.section, href: n.path }))
    const actions: Item[] = QUICK_ACTIONS.filter((a) => a.roles.some((r) => roles.includes(r)))
    const match = (i: Item) => !debounced || i.label.toLowerCase().includes(debounced)
    const out: Item[] = [
      ...actions.filter(match),
      ...nav.filter(match).slice(0, debounced ? 8 : 6),
    ]
    if (debounced && live) {
      out.push(
        ...live.swimmers.filter(match).slice(0, MAX_PER_LIVE_GROUP),
        ...live.meets.filter(match).slice(0, MAX_PER_LIVE_GROUP),
        ...live.households.filter(match).slice(0, MAX_PER_LIVE_GROUP),
      )
    }
    return out.slice(0, MAX_TOTAL)
  }, [debounced, live, roles])

  useEffect(() => setActive(0), [debounced])

  const go = (item: Item) => {
    setOpen(false)
    setQuery('')
    router.push(item.href)
  }

  if (!open) return null

  const activeId = results[active] ? `${uid}-opt-${results[active].id}` : undefined
  let lastGroup = ''

  return (
    <div className="cmdk-backdrop" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) close() }}>
      <div className="cmdk" role="dialog" aria-modal="true" aria-label="Command palette">
        <input
          ref={inputRef}
          role="combobox"
          aria-expanded="true"
          aria-controls={listId}
          aria-activedescendant={activeId}
          aria-autocomplete="list"
          aria-label="Search pages, swimmers, meets and households"
          placeholder="Search or jump to…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') { e.preventDefault(); close() }
            else if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => Math.min(a + 1, results.length - 1)) }
            else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)) }
            else if (e.key === 'Enter' && results[active]) { e.preventDefault(); go(results[active]) }
            else if (e.key === 'Tab') { e.preventDefault() }
          }}
        />
        <ul id={listId} role="listbox" aria-label="Results">
          {results.length === 0 && (
            <li role="presentation" className="cmdk-group u-p-12px" >
              {live ? 'Nothing matches that search.' : 'Searching…'}
            </li>
          )}
          {results.map((item, i) => {
            const header = item.group !== lastGroup
            lastGroup = item.group
            return (
              <li key={item.id} role="presentation" className="u-p-0" style={{ display: 'block', cursor: 'default' }}>
                {header && <div className="cmdk-group" role="presentation">{item.group}</div>}
                <div
                  id={`${uid}-opt-${item.id}`}
                  role="option"
                  aria-selected={i === active}
                  className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', padding: '10px 12px', borderRadius: 8, cursor: 'pointer', background: i === active ? 'var(--accent-subtle)' : undefined }}
                  onMouseMove={() => setActive(i)}
                  onClick={() => go(item)}
                >
                  <span>{item.label}</span>
                  {item.meta && <span className="cmdk-meta">{item.meta}</span>}
                </div>
              </li>
            )
          })}
        </ul>
        {liveFailed && (
          <p className="cmdk-meta" style={{ padding: '0 16px 12px' }}>Swimmer, meet and household search is unavailable right now.</p>
        )}
      </div>
    </div>
  )
}
