'use client'

import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { fetchPlatformOrgs, isValidSlug, provisionOrg, suggestSlug, type PlatformOrg } from '@/lib/tenancyData'
import { usePlatformMock, MOCK_ORGS, MOCK_PLATFORM_ORG_ID } from '@/lib/platform-mock'
import { ClubRow } from './ClubRow'

type NewClub = { name: string; slug: string; admin_email: string; admin_first_name: string; admin_last_name: string; team_code: string }
const EMPTY: NewClub = { name: '', slug: '', admin_email: '', admin_first_name: '', admin_last_name: '', team_code: '' }

export default function PlatformPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  // Operators belong to the dedicated platform org; every platform call is scoped to it.
  const mock = usePlatformMock()
  const platformOrgId = mock ? MOCK_PLATFORM_ORG_ID : (session?.user.orgId ?? '')
  const [orgs, setOrgs] = useState<PlatformOrg[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [form, setForm] = useState<NewClub>(EMPTY)
  const [slugEdited, setSlugEdited] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (mock === null) return
    if (mock) { setOrgs(MOCK_ORGS); return }
    if (!platformOrgId) return
    setLoadError(null)
    try {
      setOrgs(await fetchPlatformOrgs(api, platformOrgId))
    } catch (error) {
      setLoadError(uiErrorMessage(error, 'Couldn’t load clubs. Try again.'))
    }
  }, [api, platformOrgId, mock])

  useEffect(() => { void load() }, [load])

  const slugOk = isValidSlug(form.slug)
  const canCreate = form.name.trim() !== '' && slugOk && form.admin_email.includes('@') && form.admin_first_name.trim() !== '' && form.admin_last_name.trim() !== ''

  async function create(e: React.FormEvent) {
    e.preventDefault()
    if (!canCreate) return
    setCreating(true)
    setCreateError(null)
    setNotice(null)
    try {
      const res = await provisionOrg(api, platformOrgId, { ...form, team_code: form.team_code.trim() || undefined })
      setNotice(`${res.org.name} is ready. We emailed ${form.admin_email} to set a password.`)
      setForm(EMPTY)
      setSlugEdited(false)
      await load()
    } catch (error) {
      setCreateError(uiErrorMessage(error, 'Couldn’t create the club. If it was set up halfway, submit the same details again to finish.'))
    } finally {
      setCreating(false)
    }
  }

  return (
    <>
      <div className="page-header">
        <div>
          <h1 className="page-title">Clubs</h1>
          <p className="page-subtitle">Create a club, pause one, or give it its own web address.</p>
        </div>
      </div>

      {notice ? <Notice tone="ok" className="u-mb-16px">{notice}</Notice> : null}
      {actionError ? <Notice tone="err" className="u-mb-16px">{actionError}</Notice> : null}

      <section className="card u-mb-20px" aria-labelledby="new-club-title">
        <h2 className="card-title" id="new-club-title">Add a club</h2>
        <form onSubmit={create}>
          <div className="club-form-group">
            <div>
              <h3 className="club-form-group-title">Club</h3>
              <p className="club-form-group-hint">The name members see and the handle used in the club’s links.</p>
            </div>
            <div className="club-form-fields">
              <div>
                <label className="club-field-label" htmlFor="club-name">Club name</label>
                <input id="club-name" value={form.name} onChange={(e) => {
                  const name = e.target.value
                  setForm((f) => ({ ...f, name, slug: slugEdited ? f.slug : suggestSlug(name) }))
                }} />
              </div>
              <div>
                <label className="club-field-label" htmlFor="club-slug">Web handle</label>
                <input id="club-slug" value={form.slug} aria-invalid={form.slug !== '' && !slugOk} aria-describedby="club-slug-hint"
                  onChange={(e) => { setSlugEdited(true); setForm((f) => ({ ...f, slug: e.target.value.toLowerCase() })) }} />
                <p id="club-slug-hint" className={form.slug !== '' && !slugOk ? 'field-error' : 'field-hint'}>
                  3 to 40 letters, numbers or hyphens. Used in the club’s sign-up link.
                </p>
              </div>
              <div>
                <label className="club-field-label" htmlFor="club-team-code">Team code <span className="club-field-optional">(optional)</span></label>
                <input id="club-team-code" value={form.team_code} placeholder="USA Swimming" onChange={(e) => setForm((f) => ({ ...f, team_code: e.target.value }))} />
              </div>
            </div>
          </div>

          <div className="club-form-group">
            <div>
              <h3 className="club-form-group-title">First administrator</h3>
              <p className="club-form-group-hint">They get an email to set a password and can then invite everyone else.</p>
            </div>
            <div className="club-form-fields">
              <div>
                <label className="club-field-label" htmlFor="admin-first">First name</label>
                <input id="admin-first" value={form.admin_first_name} onChange={(e) => setForm((f) => ({ ...f, admin_first_name: e.target.value }))} />
              </div>
              <div>
                <label className="club-field-label" htmlFor="admin-last">Last name</label>
                <input id="admin-last" value={form.admin_last_name} onChange={(e) => setForm((f) => ({ ...f, admin_last_name: e.target.value }))} />
              </div>
              <div>
                <label className="club-field-label" htmlFor="admin-email">Email</label>
                <input id="admin-email" type="email" value={form.admin_email} onChange={(e) => setForm((f) => ({ ...f, admin_email: e.target.value }))} />
              </div>
            </div>
          </div>

          <div className="club-form-footer">
            <p className="club-form-footnote">Name, handle and administrator are required.</p>
            <button type="submit" className="btn btn-primary" disabled={!canCreate || creating}>{creating ? 'Creating club…' : 'Create club'}</button>
          </div>
        </form>
        {createError ? <Notice tone="err" className="u-mt-12px">{createError}</Notice> : null}
      </section>

      {loadError ? <LoadErrorBanner message={loadError} onRetry={() => void load()} /> : null}
      {orgs === null && !loadError ? <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Loading clubs…</p> : null}
      {orgs && orgs.length === 0 ? <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>No clubs yet. Add the first one above.</p> : null}
      {orgs && orgs.length > 0 ? (
        <div className="card u-p-0">
          <div className="table-scroll">
            <table className="club-table">
              <thead>
                <tr><th>Club</th><th>Handle</th><th>Status</th><th>Created</th><th><span className="sr-only">Actions</span></th></tr>
              </thead>
              <tbody>
                {orgs.map((org) => (
                  <ClubRow key={org.id} org={org} isPlatform={org.id === platformOrgId} api={api} platformOrgId={platformOrgId}
                    onChanged={load} onNotice={setNotice} onError={setActionError} />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </>
  )
}
