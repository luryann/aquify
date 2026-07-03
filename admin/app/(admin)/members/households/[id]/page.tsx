'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
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
import { canWrite } from '@/lib/roles'

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
  const parsed = new Date(value)
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
    refunded_partial: { color: 'var(--accent)', bg: 'var(--accent-subtle)' },
    refunded_full: { color: 'var(--accent)', bg: 'var(--accent-subtle)' },
    voided: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
    written_off: { color: 'var(--text-muted)', bg: 'var(--bg-elevated)' },
  }
  const colors = cfg[status] ?? cfg.active
  return (
    <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: colors.bg, color: colors.color, fontFamily: 'var(--font-mono)', letterSpacing: '0.02em', textTransform: 'uppercase' }}>
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
        <Link href={`/members/swimmers/${row.id}`} style={{ color: 'var(--text)', fontWeight: 600, textDecoration: 'none', fontSize: '13px' }}>
          {row.name}
        </Link>
        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '2px' }}>{row.ussNumber || 'No external ID'}</div>
      </div>
    ),
  },
  { key: 'dob', label: 'DOB', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDate(row.dob)}</span> },
  { key: 'gender', label: 'Gender', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{row.gender}</span> },
  {
    key: 'groups',
    label: 'Groups',
    render: (row) => (
      <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
        {row.groups.length ? row.groups.map((group) => (
          <span key={group} style={{ fontSize: '11px', padding: '2px 7px', borderRadius: '8px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)' }}>
            {group}
          </span>
        )) : <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Unassigned</span>}
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
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        <div style={{ width: '30px', height: '30px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '12px', fontWeight: 700, color: 'var(--accent)', flexShrink: 0 }}>
          {row.name.split(' ').map((part) => part[0]).join('').slice(0, 2)}
        </div>
        <div>
          <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>
            {row.name}
            {row.isPrimary ? <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', background: 'var(--accent-subtle)', color: 'var(--accent)', padding: '1px 6px', borderRadius: '8px', marginLeft: '6px', fontWeight: 600 }}>PRIMARY</span> : null}
          </div>
          <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>{row.role}</div>
        </div>
      </div>
    ),
  },
  { key: 'email', label: 'Email', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{row.email || '—'}</span> },
  { key: 'phone', label: 'Phone', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{row.phone || '—'}</span> },
]

const INVOICE_COLS: Column<InvoiceRow>[] = [
  {
    key: 'invoiceNumber',
    label: 'Invoice',
    render: (row) => (
      <Link href={`/invoices/${row.id}`} style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text)', textDecoration: 'none', fontWeight: 600 }}>
        {row.invoiceNumber}
      </Link>
    ),
  },
  { key: 'amountCents', label: 'Amount', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{formatCurrency(row.amountCents)}</span> },
  { key: 'status', label: 'Status', render: (row) => <StatusBadge status={row.status} /> },
  { key: 'dueAt', label: 'Due', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDate(row.dueAt)}</span> },
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
  const allowWrite = canWrite(session?.user.roles ?? [])
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
          setError(err instanceof ApiError ? err.message : 'Failed to load household details.')
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
      setAddContactError(err instanceof ApiError ? err.message : 'Failed to add contact.')
    } finally {
      setAddingContact(false)
    }
  }

  const tabs = useMemo(() => [
    { id: 'overview', label: 'Overview' },
    { id: 'swimmers', label: 'Swimmers', count: swimmers.length },
    { id: 'contacts', label: 'Contacts', count: contacts.length },
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
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="users" title="Loading household" description="Fetching live household detail data from the API." /></div>
  }

  if (error || !household) {
    return <div style={{ padding: '24px', maxWidth: '1200px' }}><EmptyState icon="alert-circle" title="Unable to load household" description={error ?? 'Household not found.'} /></div>
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <Link href="/members/households" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} />
        Households
      </Link>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>
              {household.household_name}
            </h1>
            <StatusBadge status={household.status || 'active'} />
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            {household.primary_email || household.primary_phone_e164 || 'No primary contact method'}
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Invoice Total', value: formatCurrency(totalBalanceCents), color: 'var(--text)' },
          { label: 'Swimmers', value: swimmers.length.toString(), color: 'var(--text)' },
          { label: 'Contacts', value: contacts.length.toString(), color: 'var(--text)' },
          { label: 'Member Since', value: formatDate(household.created_at), color: 'var(--text-subtle)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'overview' ? (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>
              Account Info
            </h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                <span style={{ color: 'var(--text-muted)' }}>Household ID</span>
                <button
                  onClick={() => { void navigator.clipboard.writeText(household.id) }}
                  title="Copy household ID"
                  style={{ display: 'flex', alignItems: 'center', gap: '5px', background: 'none', border: 'none', cursor: 'pointer', padding: 0, fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600, fontSize: '13px' }}
                >
                  {household.id.slice(0, 8)}…
                  <Icon name="copy" size={12} style={{ color: 'var(--text-muted)' }} />
                </button>
              </div>
              {[
                { label: 'Primary Email', value: household.primary_email || '—' },
                { label: 'Primary Phone', value: household.primary_phone_e164 || '—' },
                { label: 'Open Invoices', value: `${outstandingCount}` },
                { label: 'Created', value: formatDate(household.created_at) },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>
              Notes & Contacts
            </h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7, marginBottom: '16px' }}>
              {household.notes || 'No household notes recorded yet.'}
            </p>
            <div style={{ paddingTop: '16px', borderTop: '1px solid var(--border)' }}>
              <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '8px' }}>Primary Contact</div>
              {primaryContact ? (
                <>
                  <div style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>{primaryContact.name}</div>
                  <div style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '4px' }}>{primaryContact.email || 'No email on file'}</div>
                  <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{primaryContact.phone || 'No phone on file'}</div>
                </>
              ) : (
                <div>
                  <div style={{ fontSize: '13px', color: 'var(--text-muted)' }}>No contact on file for this household.</div>
                  <button
                    onClick={() => setActiveTab('contacts')}
                    style={{ marginTop: '8px', fontSize: '12px', color: 'var(--accent)', background: 'none', border: 'none', cursor: 'pointer', padding: 0, display: 'flex', alignItems: 'center', gap: '4px' }}
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
          {allowWrite && (
            <div style={{ marginBottom: '16px' }}>
              {!showAddContact ? (
                <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => setShowAddContact(true)}>
                  <Icon name="plus" size={14} /> Add Contact
                </button>
              ) : (
                <form onSubmit={handleAddContact} className="card" style={{ padding: '16px', display: 'flex', flexDirection: 'column', gap: '10px', maxWidth: '480px' }}>
                  {addContactError && (
                    <div style={{ fontSize: '12px', color: 'var(--error)' }}>{addContactError}</div>
                  )}
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                    <input required placeholder="First name" value={contactForm.first_name} onChange={(e) => setContactForm((c) => ({ ...c, first_name: e.target.value }))} style={{ padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }} />
                    <input required placeholder="Last name" value={contactForm.last_name} onChange={(e) => setContactForm((c) => ({ ...c, last_name: e.target.value }))} style={{ padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }} />
                  </div>
                  <select value={contactForm.relationship} onChange={(e) => setContactForm((c) => ({ ...c, relationship: e.target.value }))} style={{ padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }}>
                    <option value="parent">Parent</option>
                    <option value="guardian">Guardian</option>
                    <option value="emergency">Emergency contact</option>
                    <option value="other">Other</option>
                  </select>
                  <input placeholder="Email" type="email" value={contactForm.email} onChange={(e) => setContactForm((c) => ({ ...c, email: e.target.value }))} style={{ padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }} />
                  <input placeholder="Phone" value={contactForm.phone_e164} onChange={(e) => setContactForm((c) => ({ ...c, phone_e164: e.target.value }))} style={{ padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)' }} />
                  <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                    <input type="checkbox" checked={contactForm.is_primary} onChange={(e) => setContactForm((c) => ({ ...c, is_primary: e.target.checked }))} />
                    Make this the primary contact
                  </label>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <button type="submit" className="btn btn-primary" disabled={addingContact}>{addingContact ? 'Adding…' : 'Add Contact'}</button>
                    <button type="button" className="btn" onClick={() => setShowAddContact(false)}>Cancel</button>
                  </div>
                </form>
              )}
            </div>
          )}
          <Table columns={CONTACT_COLS} data={contacts} emptyTitle="No contacts" emptyDescription="No contacts are stored for this household yet." />
        </div>
      ) : null}

      {activeTab === 'invoices' ? (
        <Table columns={INVOICE_COLS} data={invoices} emptyTitle="No invoices" emptyDescription="No invoices have been issued for this household." />
      ) : null}
    </div>
  )
}
