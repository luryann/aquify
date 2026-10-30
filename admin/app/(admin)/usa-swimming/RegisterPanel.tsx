'use client'

import { useState } from 'react'
import { Notice } from '@/components/ui/Notice'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchUSASwimmingRegistrationStatus,
  submitUSASwimmingRegistration,
  type SwimmerRecord,
  type USASwimmingRegistrationInput,
  type USASwimmingRegistrationResult,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'


const EMPTY: USASwimmingRegistrationInput = {
  member_id: '', first_name: '', last_name: '', date_of_birth: '', competition_category: 'FEMALE',
  phone: '', phone_type: 'Mobile', postal_code: '', offering_id: '',
}

function categoryFor(gender: string): 'FEMALE' | 'MALE' {
  return gender.trim().toLowerCase().startsWith('m') ? 'MALE' : 'FEMALE'
}

export function RegisterPanel({ swimmers }: { swimmers: SwimmerRecord[] }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [form, setForm] = useState<USASwimmingRegistrationInput>(EMPTY)
  const [busy, setBusy] = useState<'submit' | 'status' | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [result, setResult] = useState<USASwimmingRegistrationResult | null>(null)
  const [requestId, setRequestId] = useState('')
  const [status, setStatus] = useState<string | null>(null)

  const set = (k: keyof USASwimmingRegistrationInput) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))

  function prefill(id: string) {
    const s = swimmers.find((x) => x.id === id)
    if (!s) return setForm(EMPTY)
    setForm({ ...EMPTY, member_id: (s.usa_swimming_id ?? '').replace(/[^A-Za-z0-9]/g, ''), first_name: s.first_name, last_name: s.last_name, date_of_birth: s.date_of_birth, competition_category: categoryFor(s.gender) })
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!session) return
    setBusy('submit')
    setErr(null)
    setResult(null)
    try {
      const body: USASwimmingRegistrationInput = { ...form }
      for (const k of ['member_id', 'phone', 'postal_code', 'offering_id'] as const) if (!body[k]) delete body[k]
      const res = await submitUSASwimmingRegistration(api, session.user.orgId, body)
      setResult(res)
      if (res.request_id) {
        setRequestId(res.request_id)
        setStatus(null)
      }
    } catch (e2) {
      setErr(uiErrorMessage(e2, 'Couldn’t submit the registration. Registering members needs Tier 2 access from USA Swimming.'))
    } finally {
      setBusy(null)
    }
  }

  async function checkStatus() {
    if (!session || !requestId.trim()) return
    setBusy('status')
    setErr(null)
    try {
      setStatus(await fetchUSASwimmingRegistrationStatus(api, session.user.orgId, requestId.trim()))
    } catch (e) {
      setErr(uiErrorMessage(e, 'Couldn’t check that request. Confirm the request ID and try again.'))
    } finally {
      setBusy(null)
    }
  }

  const field = { width: '100%' } as const

  return (
    <div className="stack-lg">
      {err ? <Notice tone="err">{err}</Notice> : null}

      <section className="card u-p-20px">
        <h2 className="card-title">Register or renew a member</h2>
        <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-muted)', maxWidth: '65ch' }}>
          Submits a membership to USA Swimming for your club. Leave the USA Swimming ID blank for a new member. Fill the form from an Aquify swimmer, then adjust.
        </p>
        <form onSubmit={(e) => void submit(e)} className="u-mt-16px u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', alignItems: 'end' }}>
          <div>
            <label className="form-label" htmlFor="rg-swimmer">Start from swimmer</label>
            <select id="rg-swimmer" style={field} defaultValue="" onChange={(e) => prefill(e.target.value)}>
              <option value="">Enter details manually</option>
              {swimmers.map((s) => <option key={s.id} value={s.id}>{s.first_name} {s.last_name}</option>)}
            </select>
          </div>
          <div>
            <label className="form-label" htmlFor="rg-id">USA Swimming ID</label>
            <input id="rg-id" className="tabular" style={field} value={form.member_id ?? ''} onChange={set('member_id')} maxLength={14} pattern="[A-Za-z0-9]{14}|" title="14 letters and digits, or blank for a new member" />
          </div>
          <div>
            <label className="form-label" htmlFor="rg-first">First name</label>
            <input id="rg-first" style={field} value={form.first_name} onChange={set('first_name')} required />
          </div>
          <div>
            <label className="form-label" htmlFor="rg-last">Last name</label>
            <input id="rg-last" style={field} value={form.last_name} onChange={set('last_name')} required />
          </div>
          <div>
            <label className="form-label" htmlFor="rg-dob">Date of birth</label>
            <input id="rg-dob" type="date" style={field} value={form.date_of_birth} onChange={set('date_of_birth')} required />
          </div>
          <div>
            <label className="form-label" htmlFor="rg-cat">Competition category</label>
            <select id="rg-cat" style={field} value={form.competition_category} onChange={set('competition_category')}>
              <option value="FEMALE">Female</option>
              <option value="MALE">Male</option>
            </select>
          </div>
          <div>
            <label className="form-label" htmlFor="rg-phone">Phone</label>
            <input id="rg-phone" inputMode="numeric" style={field} value={form.phone ?? ''} onChange={set('phone')} placeholder="15551234567" pattern="(1|52)\d{10}|" title="1 or 52 followed by 10 digits" />
          </div>
          <div>
            <label className="form-label" htmlFor="rg-ptype">Phone type</label>
            <select id="rg-ptype" style={field} value={form.phone_type} onChange={set('phone_type')}>
              {['Mobile', 'Work', 'Home', 'Other'].map((t) => <option key={t}>{t}</option>)}
            </select>
          </div>
          <div>
            <label className="form-label" htmlFor="rg-zip">Postal code</label>
            <input id="rg-zip" style={field} value={form.postal_code ?? ''} onChange={set('postal_code')} pattern="\d{5}(-?\d{4})?|" title="5 digits, with an optional 4-digit extension" />
          </div>
          <div>
            <label className="form-label" htmlFor="rg-offer">Offering ID</label>
            <input id="rg-offer" style={field} value={form.offering_id ?? ''} onChange={set('offering_id')} />
          </div>
          <div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button type="button" className="btn btn-secondary" onClick={() => setForm(EMPTY)} disabled={busy !== null}>Clear</button>
            <button type="submit" className="btn btn-primary" disabled={busy !== null}>{busy === 'submit' ? 'Submitting…' : 'Submit registration'}</button>
          </div>
        </form>
        {result ? (
          <Notice tone={result.ok ? 'ok' : 'err'} className="u-mt-16px">
            {result.ok
              ? `Submitted. Status: ${result.status}.${result.request_id ? ` Request ID ${result.request_id}.` : ''}`
              : `USA Swimming rejected the registration: ${result.error ?? 'no reason given'}.`}
          </Notice>
        ) : null}
      </section>

      <section className="card u-p-20px">
        <h2 className="card-title">Check a request</h2>
        <div className="u-mt-12px u-gap-12px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'end' }}>
          <div style={{ flex: '1 1 260px' }}>
            <label className="form-label" htmlFor="rg-req">Request ID</label>
            <input id="rg-req" className="tabular" style={field} value={requestId} onChange={(e) => setRequestId(e.target.value)} />
          </div>
          <button type="button" className="btn btn-secondary" onClick={() => void checkStatus()} disabled={busy !== null || !requestId.trim()}>
            {busy === 'status' ? 'Checking…' : 'Check status'}
          </button>
        </div>
        {status ? <p role="status" className="u-mt-12px u-m-0">Status: <strong>{status}</strong></p> : null}
      </section>
    </div>
  )
}
