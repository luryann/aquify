'use client'

import { Fragment, useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import {
  CLUB_USERS_PAGE_SIZE, SUPPORT_ACTIONS, fetchClubUsers, isValidReason, runSupportAction, supportActionsFor, userDisplayName,
  type ClubUser, type SupportAction,
} from '@/lib/tenancyData'
import { ReasonField } from './shared'

type Props = {
  api: ApiClient
  platformOrgId: string
  orgId: string
  /** The first page of the club's people, loaded with the page. Search and role filters query the server. */
  users: ClubUser[]
  onChanged: () => Promise<void>
  onNotice: (message: string) => void
}

const ROLE_ORDER = ['admin', 'coach', 'parent', 'swimmer']
const ROLE_LABEL: Record<string, string> = { admin: 'Admin', coach: 'Coach', parent: 'Parent', swimmer: 'Swimmer' }

/** A club's people with the account-recovery actions support is asked for most. */
export function ClubPeople({ api, platformOrgId, orgId, users, onChanged, onNotice }: Props) {
  const [query, setQuery] = useState('')
  const [role, setRole] = useState('')
  const [openId, setOpenId] = useState<string | null>(null)
  const [action, setAction] = useState<SupportAction | null>(null)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Search and the role filter run on the server so a club with hundreds of accounts is fully searchable.
  const [shown, setShown] = useState<ClubUser[]>(users)
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)
  const filtered = query.trim() !== '' || role !== ''
  // A full page means there may be more people behind it.
  const [hasMore, setHasMore] = useState(users.length >= CLUB_USERS_PAGE_SIZE)
  const [loadingMore, setLoadingMore] = useState(false)

  useEffect(() => {
    if (!filtered) {
      setShown(users)
      setHasMore(users.length >= CLUB_USERS_PAGE_SIZE)
      setSearchError(null)
      return
    }
    let cancelled = false
    setSearching(true)
    const timer = setTimeout(() => {
      fetchClubUsers(api, platformOrgId, orgId, { search: query, role })
        .then((found) => { if (!cancelled) { setShown(found); setHasMore(found.length >= CLUB_USERS_PAGE_SIZE); setSearchError(null) } })
        .catch((e) => { if (!cancelled) setSearchError(uiErrorMessage(e, 'Couldn’t search this club’s people. Try again.')) })
        .finally(() => { if (!cancelled) setSearching(false) })
    }, 250)
    return () => { cancelled = true; clearTimeout(timer) }
  }, [api, platformOrgId, orgId, query, role, filtered, users])

  async function loadMore() {
    setLoadingMore(true)
    try {
      const next = await fetchClubUsers(api, platformOrgId, orgId, { search: query, role, offset: shown.length })
      setShown((current) => [...current, ...next.filter((u) => !current.some((c) => c.id === u.id))])
      setHasMore(next.length >= CLUB_USERS_PAGE_SIZE)
      setSearchError(null)
    } catch (e) {
      setSearchError(uiErrorMessage(e, 'Couldn’t load more people. Try again.'))
    } finally {
      setLoadingMore(false)
    }
  }

  function toggle(id: string) {
    setOpenId(openId === id ? null : id)
    setAction(null)
    setReason('')
    setError(null)
  }

  async function confirm(u: ClubUser) {
    if (!action || !isValidReason(reason)) return
    setBusy(true)
    setError(null)
    try {
      await runSupportAction(api, platformOrgId, orgId, u.id, action, reason.trim())
      onNotice(`${SUPPORT_ACTIONS[action].done} for ${userDisplayName(u)}.`)
      setOpenId(null)
      setAction(null)
      setReason('')
      await onChanged()
    } catch (e) {
      setError(uiErrorMessage(e, 'That didn’t go through. Nothing was changed. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="card u-p-0" aria-labelledby="people-title">
      <div className="u-p-20px" style={{ display: 'flex', gap: '16px', flexWrap: 'wrap', alignItems: 'end', justifyContent: 'space-between' }}>
        <div>
          <h2 className="card-title" id="people-title">People</h2>
          <p className="u-fs-fs-sm u-m-0" style={{ color: 'var(--text-muted)' }}>Account details only. Swimmers, billing and other club records are not shown here.</p>
        </div>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <input type="search" aria-label="Search people" placeholder="Search name or email" value={query} onChange={(e) => setQuery(e.target.value)} style={{ minWidth: '220px' }} />
          <select aria-label="Filter by role" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="">All roles</option>
            {ROLE_ORDER.map((r) => <option key={r} value={r}>{ROLE_LABEL[r]}</option>)}
          </select>
        </div>
      </div>

      {searchError ? <Notice tone="err" className="u-mb-16px">{searchError}</Notice> : null}
      {users.length === 0 && !filtered ? <p className="u-p-20px u-m-0 u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>No one has an account at this club yet.</p> : null}
      {shown.length === 0 && (filtered || users.length > 0) && !searching ? <p className="u-p-20px u-m-0 u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>No one matches that search.</p> : null}
      {shown.length > 0 ? (
        <div className="table-scroll">
          <table style={{ width: '100%' }}>
            <thead>
              <tr><th align="left">Person</th><th align="left">Roles</th><th align="left">Account</th><th /></tr>
            </thead>
            <tbody>
              {shown.map((u) => {
                const open = openId === u.id
                const actions = supportActionsFor(u)
                const chosen = action ? SUPPORT_ACTIONS[action] : null
                return (
                  <Fragment key={u.id}>
                    <tr>
                      <td>
                        <div>{userDisplayName(u)}</div>
                        <div className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{u.email}</div>
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                          {u.roles.length === 0 ? <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>None</span> : null}
                          {u.roles.map((r) => <span key={r} className="badge badge-default">{ROLE_LABEL[r] ?? r}</span>)}
                        </div>
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                          {!u.enabled ? <span className="badge badge-error">Disabled</span> : <span className="badge badge-success">Active</span>}
                          {!u.email_verified ? <span className="badge badge-warning">Email not verified</span> : null}
                          {u.mfa_enrolled ? <span className="badge badge-info">MFA on</span> : null}
                        </div>
                      </td>
                      <td align="right">
                        <button type="button" className="btn btn-ghost btn-sm" aria-expanded={open} aria-controls={`help-${u.id}`} onClick={() => toggle(u.id)}>
                          {open ? 'Close' : 'Help with account'}
                        </button>
                      </td>
                    </tr>
                    {open ? (
                      <tr>
                        <td colSpan={4} id={`help-${u.id}`} style={{ background: 'var(--bg-surface)' }}>
                          <div style={{ maxWidth: '620px' }}>
                            <div role="group" aria-label={`Actions for ${userDisplayName(u)}`} style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                              {actions.map((a) => (
                                <button key={a} type="button" aria-pressed={action === a}
                                  className={action === a ? 'btn btn-secondary btn-sm' : 'btn btn-ghost btn-sm'}
                                  onClick={() => { setAction(a); setError(null) }}>
                                  {SUPPORT_ACTIONS[a].label}
                                </button>
                              ))}
                            </div>
                            {chosen && action ? (
                              <div className="u-mt-12px" style={{ display: 'grid', gap: '12px' }}>
                                <p className="u-m-0 u-fs-fs-sm"><strong>{chosen.label}</strong> for {userDisplayName(u)}. {chosen.consequence}</p>
                                <ReasonField id={`reason-${u.id}`} value={reason} onChange={setReason} />
                                <div style={{ display: 'flex', gap: '8px' }}>
                                  <button type="button" className={chosen.danger ? 'btn btn-danger' : 'btn btn-primary'}
                                    disabled={busy || !isValidReason(reason)} onClick={() => void confirm(u)}>
                                    {busy ? 'Working…' : chosen.label}
                                  </button>
                                  <button type="button" className="btn btn-ghost" onClick={() => toggle(u.id)}>Cancel</button>
                                </div>
                              </div>
                            ) : null}
                            {error ? <Notice tone="err" className="u-mt-12px">{error}</Notice> : null}
                          </div>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
      {hasMore && shown.length > 0 ? (
        <div className="u-p-20px">
          <button type="button" className="btn btn-secondary" onClick={() => void loadMore()} disabled={loadingMore || searching}>
            {loadingMore ? 'Loading…' : 'Show more people'}
          </button>
        </div>
      ) : null}
    </section>
  )
}
