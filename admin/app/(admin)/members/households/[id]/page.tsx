'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  addHouseholdContact,
  fetchGroups,
  fetchHousehold,
  fetchHouseholdContacts,
  fetchInvoicesForHousehold,
  fetchSwimmerGroups,
  fetchSwimmers,
  type ContactRecord,
  type GroupRecord,
  type HouseholdRecord,
  type InvoiceRecord,
  type SwimmerGroupLinkRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { parseBackendDate } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type SwimmerRow = {
  id: string
  name: string
  ussNumber: string
  dob: string
  gender: string
  groups: string[]
  status: string
}

type ContactRow = {
  id: string
  name: string
  role: string
  email: string
  phone: string
  isPrimary: boolean
}

type InvoiceRow = {
  id: string
  invoiceNumber: string
  amountCents: number
  status: string
  issuedAt: string
  dueAt: string
}

function formatCurrency(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100)
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  const parsed = parseBackendDate(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function StatusBadge({ status }: { status: string }) {
  const cfg: Record<string, { color: string; bg: string }> = {
    active: { color: 'var(--success)', bg: 'var(--success-bg)' },
    inactive: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    pending: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    suspended: { color: 'var(--error)', bg: 'var(--error-bg)' },
    archived: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    draft: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    issued: { color: 'var(--info)', bg: 'var(--info-bg)' },
    partially_paid: { color: 'var(--warning)', bg: 'var(--warning-bg)' },
    paid: { color: 'var(--success)', bg: 'var(--success-bg)' },
    refunded_partial: { color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
    refunded_full: { color: 'var(--accent-text, var(--accent))', bg: 'var(--accent-subtle)' },
    voided: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    written_off: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  }
  const colors = cfg[status] ?? cfg.active
  return (
    <span className="badge" style={{ background: colors.bg, color: colors.color }}>
      {status.replaceAll('_', ' ')}
    </span>
  )
}

const SWIMMER_COLS: Column<SwimmerRow>[] = [
  {
    key: 'name',
    label: 'Swimmer',
    sortable: true,
    render: (row) => (
      <div>
        <Link href={`/members/swimmers/${row.id}`} className="u-fw-600 u-fs-fs-base" style={{ color: 'var(--text)', textDecoration: 'none' }}>
          {row.name}
        </Link>
        <div className="u-fs-fs-xs u-ff-font-body u-mt-2px" style={{ color: 'var(--text-muted)' }}>{row.ussNumber || 'No external ID'}</div>
      </div>
    ),
  },
  { key: 'dob', label: 'DOB', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDate(row.dob)}</span> },
  { key: 'gender', label: 'Gender', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{row.gender}</span> },
  {
    key: 'groups',
    label: 'Groups',
    render: (row) => (
      <div className="u-gap-4px" style={{ display: 'flex', flexWrap: 'wrap' }}>
        {row.groups.length ? row.groups.map((group) => (
          <span key={group} className="u-fs-fs-xs u-p-2px-8px u-ff-font-body" style={{ borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))' }}>
            {group}
          </span>
        )) : <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Unassigned</span>}
      </div>
    ),
  },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
]

const CONTACT_COLS: Column<ContactRow>[] = [
  {
    key: 'name',
    label: 'Contact',
    sortable: true,
    render: (row) => (
      <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
        <div className="u-fs-fs-sm u-fw-700" style={{ width: '30px', height: '30px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--accent-text, var(--accent))', flexShrink: 0 }}>
          {row.name.split(' ').map((part) => part[0]).join('').slice(0, 2)}
        </div>
        <div>
          <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>
            {row.name}
            {row.isPrimary ? <span className="u-fs-fs-2xs u-ff-font-body u-p-1px-6px u-fw-600" style={{ background: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))', borderRadius: '8px', marginLeft: '6px' }}>PRIMARY</span> : null}
          </div>
          <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>{row.role}</div>
        </div>
      </div>
    ),
  },
  { key: 'email', label: 'Email', render: (row) => <span className="u-fs-fs-base u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>{row.email || '—'}</span> },
  { key: 'phone', label: 'Phone', render: (row) => <span className="u-fs-fs-base u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>{row.phone || '—'}</span> },
]

const INVOICE_COLS: Column<InvoiceRow>[] = [
  {
    key: 'invoiceNumber',
    label: 'Invoice',
    render: (row) => (
      <Link href={`/invoices/${row.id}`} className="u-ff-font-body u-fs-fs-sm u-fw-600" style={{ color: 'var(--text)', textDecoration: 'none' }}>
        {row.invoiceNumber}
      </Link>
    ),
  },
  { key: 'amountCents', label: 'Amount', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>{formatCurrency(row.amountCents)}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'dueAt', label: 'Due', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDate(row.dueAt)}</span> },
]

function buildSwimmerRows(swimmers: SwimmerRecord[], groups: GroupRecord[], links: SwimmerGroupLinkRecord[]): SwimmerRow[] {
  const groupNameByCode = new Map(groups.map((group) => [group.code, group.name]))
  const groupsBySwimmer = links.reduce<Record<string, string[]>>((acc, link) => {
    const groupName = groupNameByCode.get(link.group_code) ?? link.group_code
    acc[link.swimmer_id] = [...(acc[link.swimmer_id] ?? []), groupName]
    return acc
  }, {})

  return swimmers
    .map((swimmer) => ({
      id: swimmer.id,
      name: `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim(),
      ussNumber: swimmer.swimmer_external_id || '',
      dob: swimmer.date_of_birth,
      gender: swimmer.gender,
      groups: (groupsBySwimmer[swimmer.id] ?? []).sort(),
      status: swimmer.registration_status,
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

function buildContactRows(contacts: ContactRecord[]): ContactRow[] {
  return contacts.map((contact) => ({
    id: contact.id,
    name: `${contact.first_name} ${contact.last_name}`.trim(),
    role: contact.relationship.replaceAll('_', ' '),
    email: contact.email || '',
    phone: contact.phone_e164 || '',
    isPrimary: contact.is_primary,
  }))
}

function buildInvoiceRows(invoices: InvoiceRecord[]): InvoiceRow[] {
  return invoices
    .map((invoice) => ({
      id: invoice.id,
      invoiceNumber: invoice.invoice_number,
      amountCents: invoice.total_cents,
      status: invoice.status,
      issuedAt: invoice.issued_at || invoice.created_at,
      dueAt: invoice.due_date || '',
    }))
    .sort((a, b) => b.issuedAt.localeCompare(a.issuedAt))
}

export default function HouseholdDetailPage() {
  const params = useParams<{ id: string }>()
  const householdId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [activeTab, setActiveTab] = useState('overview')
  const [household, setHousehold] = useState<HouseholdRecord | null>(null)
  const [swimmers, setSwimmers] = useState<SwimmerRow[]>([])
  const [contacts, setContacts] = useState<ContactRow[]>([])
  const [invoices, setInvoices] = useState<InvoiceRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Add-contact form (Contacts tab empty state)
  const [showAddContact, setShowAddContact] = useState(false)
  const [contactForm, setContactForm] = useState({ relationship: 'parent', first_name: '', last_name: '', email: '', phone_e164: '', is_primary: false })
  const [addingContact, setAddingContact] = useState(false)
  const [addContactError, setAddContactError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId || !householdId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const [nextHousehold, allSwimmers, allGroups, allLinks, nextContacts, nextInvoices] = await Promise.all([
          fetchHousehold(api, session.user.orgId, householdId),
          fetchSwimmers(api, session.user.orgId),
          fetchGroups(api, session.user.orgId),
          fetchSwimmerGroups(api, session.user.orgId),
          fetchHouseholdContacts(api, session.user.orgId, householdId),
          fetchInvoicesForHousehold(api, session.user.orgId, householdId),
        ])

        if (cancelled) return

        const householdSwimmers = allSwimmers.filter((swimmer) => swimmer.household_id === householdId)
        const swimmerIds = new Set(householdSwimmers.map((swimmer) => swimmer.id))
        setHousehold(nextHousehold)
        setSwimmers(buildSwimmerRows(householdSwimmers, allGroups, allLinks.filter((link) => swimmerIds.has(link.swimmer_id))))
        setContacts(buildContactRows(nextContacts))
        setInvoices(buildInvoiceRows(nextInvoices))
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load household details. Try again.'))
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, householdId, session?.user.orgId])

  async function handleAddContact(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId || !householdId) return
    setAddingContact(true)
    setAddContactError(null)
    try {
      await addHouseholdContact(api, session.user.orgId, householdId, contactForm)
      const nextContacts = await fetchHouseholdContacts(api, session.user.orgId, householdId)
      setContacts(buildContactRows(nextContacts))
      setShowAddContact(false)
      setContactForm({ relationship: 'parent', first_name: '', last_name: '', email: '', phone_e164: '', is_primary: false })
    } catch (err) {
      setAddContactError(uiErrorMessage(err, 'Couldn’t add contact. Try again.'))
    } finally {
      setAddingContact(false)
    }
  }

  const tabs = useMemo(() => [
    { id: 'overview', label: 'Overview' },
    { id: 'swimmers', label: 'Swimmers', count: swimmers.length },
    { id: 'contacts', label: 'Additional contacts', count: contacts.length },
    { id: 'invoices', label: 'Invoices', count: invoices.length },
  ], [contacts.length, invoices.length, swimmers.length])

  const totalBalanceCents = useMemo(
    () => invoices.reduce((sum, invoice) => sum + invoice.amountCents, 0),
    [invoices],
  )
  const outstandingCount = useMemo(
    () => invoices.filter((invoice) => ['issued', 'partially_paid'].includes(invoice.status)).length,
    [invoices],
  )
  const primaryContact = contacts.find((contact) => contact.isPrimary) ?? contacts[0]

  if (loading) {
    return <div className="page"><EmptyState icon="users" title="Loading household" description="Fetching live household detail data from the API." /></div>
  }

  if (error || !household) {
    return <div className="page"><EmptyState icon="alert-circle" title="Unable to load household" description={error ?? 'Household not found.'} /></div>
  }

  return (
    <div className="page">
      <Link href="/members?tab=households" className="u-gap-4px u-fs-fs-base u-mb-20px" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)', textDecoration: 'none' }}>
        <Icon name="chevron-left" size={14} />
        Households
      </Link>

      <div className="page-header">
        <div>
          <div className="u-gap-6px-12px u-mb-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
            <h1 className="page-title">
              {household.household_name}
            </h1>
            <StatusBadge status={household.status || 'active'} />
          </div>
          <div className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
            {household.primary_email || household.primary_phone_e164 || 'No primary contact method'}
          </div>
        </div>
      </div>

      <div className="cols-4 u-gap-12px u-mb-24px" >
        {[
          { label: 'Invoice total', value: formatCurrency(totalBalanceCents), color: 'var(--text)' },
          { label: 'Swimmers', value: swimmers.length.toString(), color: 'var(--text)' },
          { label: 'Additional contacts', value: contacts.length.toString(), color: 'var(--text)' },
          { label: 'Member since', value: formatDate(household.created_at), color: 'var(--text-subtle)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-xl u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview' ? (
        <div className="cols-2 u-gap-16px" >
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >
              Account
            </h3>
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              {[
                { label: 'Primary email', value: household.primary_email || '—' },
                { label: 'Primary phone', value: household.primary_phone_e164 || '—' },
                { label: 'Open invoices', value: `${outstandingCount}` },
                { label: 'Created', value: formatDate(household.created_at) },
              ].map((row) => (
                <div key={row.label} className="u-fs-fs-base u-pb-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span className="u-ff-font-body u-fw-600" style={{ color: 'var(--text)' }}>{row.value}</span>
                </div>
              ))}
              <div className="u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: 'var(--text-muted)' }}>Household ID</span>
                <button type="button"
                  onClick={() => { void navigator.clipboard.writeText(household.id) }}
                  title="Copy household ID"
                  className="u-gap-4px u-p-0 u-ff-font-body u-fw-600 u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}
                >
                  {household.id.slice(0, 8)}…
                  <Icon name="copy" size={12} style={{ color: 'var(--text-muted)' }} />
                </button>
              </div>
            </div>
          </div>
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >
              Notes &amp; contacts
            </h3>
            <p className="u-fs-fs-base u-lh-1_7 u-mb-16px" style={{ color: 'var(--text-subtle)' }}>
              {household.notes || 'No household notes recorded yet.'}
            </p>
            <div className="u-pt-16px" style={{ borderTop: '1px solid var(--border)' }}>
              <div className="u-fs-fs-sm u-ff-font-body u-mb-8px" style={{ color: 'var(--text-muted)' }}>Additional contact</div>
              {primaryContact ? (
                <>
                  <div className="u-fs-fs-md u-fw-600" style={{ color: 'var(--text)' }}>{primaryContact.name}</div>
                  <div className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>{primaryContact.email || 'No email on file'}</div>
                  <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{primaryContact.phone || 'No phone on file'}</div>
                </>
              ) : (
                <div>
                  <div className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No additional contacts on file. The household&apos;s primary email and phone are shown under Account Info.</div>
                  <button type="button"
                    onClick={() => setActiveTab('contacts')}
                    className="u-mt-8px u-fs-fs-sm u-p-0 u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center' }}
                  >
                    Add a contact <Icon name="chevron-right" size={12} />
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}

      {activeTab === 'swimmers' ? (
        <Table columns={SWIMMER_COLS} data={swimmers} emptyTitle="No swimmers" emptyDescription="No swimmers are assigned to this household yet." />
      ) : null}

      {activeTab === 'contacts' ? (
        <div>
          <div className="u-mb-16px" >
            {!showAddContact ? (
              <button type="button" className="btn btn-primary u-gap-6px" style={{ display: 'flex', alignItems: 'center' }} onClick={() => setShowAddContact(true)}>
                <Icon name="plus" size={14} /> Add contact
              </button>
            ) : (
              <form onSubmit={handleAddContact} className="card u-p-16px u-gap-12px" style={{ display: 'flex', flexDirection: 'column', maxWidth: '480px' }}>
                {addContactError && (
                  <div className="u-fs-fs-sm" style={{ color: 'var(--error)' }}>{addContactError}</div>
                )}
                <div className="cols-2 u-gap-12px" >
                  <input aria-label="First name" required placeholder="First name" value={contactForm.first_name} onChange={(e) => setContactForm((c) => ({ ...c, first_name: e.target.value }))} className="u-p-8px-12px"  />
                  <input aria-label="Last name" required placeholder="Last name" value={contactForm.last_name} onChange={(e) => setContactForm((c) => ({ ...c, last_name: e.target.value }))} className="u-p-8px-12px"  />
                </div>
                <select value={contactForm.relationship} onChange={(e) => setContactForm((c) => ({ ...c, relationship: e.target.value }))} className="u-p-8px-12px" >
                  <option value="parent">Parent</option>
                  <option value="guardian">Guardian</option>
                  <option value="emergency">Emergency contact</option>
                  <option value="other">Other</option>
                </select>
                <input aria-label="Email" placeholder="Email" type="email" value={contactForm.email} onChange={(e) => setContactForm((c) => ({ ...c, email: e.target.value }))} className="u-p-8px-12px"  />
                <input aria-label="Phone" placeholder="Phone" value={contactForm.phone_e164} onChange={(e) => setContactForm((c) => ({ ...c, phone_e164: e.target.value }))} className="u-p-8px-12px"  />
                <label className="u-gap-8px u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
                  <input type="checkbox" checked={contactForm.is_primary} onChange={(e) => setContactForm((c) => ({ ...c, is_primary: e.target.checked }))} />
                  Make this the primary contact
                </label>
                <div className="u-gap-8px" style={{ display: 'flex' }}>
                  <button type="submit" className="btn btn-primary" disabled={addingContact}>{addingContact ? 'Adding…' : 'Add contact'}</button>
                  <button type="button" className="btn" onClick={() => setShowAddContact(false)}>Cancel</button>
                </div>
              </form>
            )}
          </div>
          <Table columns={CONTACT_COLS} data={contacts} emptyTitle="No contacts" emptyDescription="No contacts are stored for this household yet." />
        </div>
      ) : null}

      {activeTab === 'invoices' ? (
        <Table columns={INVOICE_COLS} data={invoices} emptyTitle="No invoices" emptyDescription="No invoices have been issued for this household." />
      ) : null}
    </div>
  )
}
