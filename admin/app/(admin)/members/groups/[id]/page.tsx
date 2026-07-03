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
import { canWrite } from '@/lib/roles'
import {
  assignSwimmerToGroup,
  fetchGroup,
  fetchGroupEligibilityRules,
  fetchSwimmerGroups,
  fetchSwimmers,
  fetchUsers,
  type EligibilityRuleRecord,
  type GroupRecord,
  type SwimmerGroupLinkRecord,
  type SwimmerRecord,
  type UserRecord,
} from '@/lib/adminData'

function displayNameForUser(user: UserRecord): string {
  const name = `${user.firstName ?? ''} ${user.lastName ?? ''}`.trim()
  return name || user.email || user.id
}

type SwimmerRow = {
  id: string
  name: string
  ussNumber: string
  age: string
  gender: string
  status: string
  joinedGroup: string
  isPrimary: boolean
}

type RuleRow = {
  id: string
  ruleType: string
  description: string
  active: boolean
  createdAt: string
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function computeAge(dob?: string) {
  if (!dob) return '—'
  const birth = new Date(dob)
  if (Number.isNaN(birth.getTime())) return '—'
  const now = new Date()
  let age = now.getFullYear() - birth.getFullYear()
  const birthdayPassed = now.getMonth() > birth.getMonth() || (now.getMonth() === birth.getMonth() && now.getDate() >= birth.getDate())
  if (!birthdayPassed) age -= 1
  return `${age}`
}

function summarizeRule(rule: EligibilityRuleRecord) {
  const params = rule.parameters ?? {}
  if (rule.rule_type === 'age_range') {
    const min = typeof params.min_age === 'number' ? params.min_age : '—'
    const max = typeof params.max_age === 'number' ? params.max_age : '—'
    return `Age ${min} to ${max}`
  }
  if (rule.rule_type === 'gender') {
    const allowed = Array.isArray(params.allowed) ? params.allowed.join(', ') : 'configured values'
    return `Allowed genders: ${allowed}`
  }
  if (rule.rule_type === 'registration_status') {
    const allowed = Array.isArray(params.allowed) ? params.allowed.join(', ') : 'configured values'
    return `Registration status must be ${allowed}`
  }
  return 'Custom eligibility rule'
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
  { key: 'age', label: 'Age', align: 'right', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>{row.age}</span> },
  { key: 'gender', label: 'Gender', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{row.gender}</span> },
  { key: 'isPrimary', label: 'Role', render: (row) => <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: row.isPrimary ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: row.isPrimary ? 'var(--accent)' : 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' }}>{row.isPrimary ? 'primary' : 'secondary'}</span> },
  { key: 'status', label: 'Status', render: (row) => <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: row.status === 'active' ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.status === 'active' ? 'var(--success)' : 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' }}>{row.status}</span> },
  { key: 'joinedGroup', label: 'Since', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.joinedGroup)}</span> },
]

const RULE_COLS: Column<RuleRow>[] = [
  { key: 'ruleType', label: 'Type', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', padding: '2px 8px', borderRadius: '8px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', color: 'var(--text-subtle)' }}>{row.ruleType}</span> },
  { key: 'description', label: 'Description', render: (row) => <span style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>{row.description}</span> },
  { key: 'createdAt', label: 'Created', render: (row) => <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDate(row.createdAt)}</span> },
  { key: 'active', label: 'Status', render: (row) => <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: row.active ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.active ? 'var(--success)' : 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' }}>{row.active ? 'active' : 'disabled'}</span> },
]

function buildSwimmerRows(swimmers: SwimmerRecord[], links: SwimmerGroupLinkRecord[]): SwimmerRow[] {
  const swimmerById = new Map(swimmers.map((swimmer) => [swimmer.id, swimmer]))
  return links
    .map((link) => {
      const swimmer = swimmerById.get(link.swimmer_id)
      if (!swimmer) return null
      return {
        id: swimmer.id,
        name: `${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim(),
        ussNumber: swimmer.swimmer_external_id || '',
        age: computeAge(swimmer.date_of_birth),
        gender: swimmer.gender,
        status: swimmer.registration_status,
        joinedGroup: link.assigned_at,
        isPrimary: link.is_primary,
      }
    })
    .filter((row): row is SwimmerRow => Boolean(row))
    .sort((a, b) => {
      if (a.isPrimary !== b.isPrimary) return a.isPrimary ? -1 : 1
      return a.name.localeCompare(b.name)
    })
}

function buildRuleRows(rules: EligibilityRuleRecord[]): RuleRow[] {
  return rules.map((rule) => ({
    id: rule.id,
    ruleType: rule.rule_type.replaceAll('_', ' '),
    description: summarizeRule(rule),
    active: rule.is_active,
    createdAt: rule.created_at,
  }))
}

export default function GroupDetailPage() {
  const params = useParams<{ id: string }>()
  const groupId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const allowWrite = canWrite(session?.user.roles ?? [])
  const [activeTab, setActiveTab] = useState('members')
  const [group, setGroup] = useState<GroupRecord | null>(null)
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [memberLinks, setMemberLinks] = useState<SwimmerGroupLinkRecord[]>([])
  const [members, setMembers] = useState<SwimmerRow[]>([])
  const [rules, setRules] = useState<RuleRow[]>([])
  const [coachName, setCoachName] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [assigning, setAssigning] = useState(false)
  const [assignError, setAssignError] = useState<string | null>(null)
  const [assignSuccess, setAssignSuccess] = useState<string | null>(null)
  const [assignForm, setAssignForm] = useState({ swimmerId: '', isPrimary: false })

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!session?.user.orgId || !groupId) {
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const [nextGroup, nextSwimmers, swimmerGroups, eligibilityRules] = await Promise.all([
          fetchGroup(api, session.user.orgId, groupId),
          fetchSwimmers(api, session.user.orgId),
          fetchSwimmerGroups(api, session.user.orgId),
          fetchGroupEligibilityRules(api, session.user.orgId, groupId),
        ])

        if (cancelled) return

        const nextMemberLinks = swimmerGroups.filter((link) => link.group_code === nextGroup.code)
        setGroup(nextGroup)
        setSwimmers(nextSwimmers)
        setMemberLinks(nextMemberLinks)
        setMembers(buildSwimmerRows(nextSwimmers, nextMemberLinks))
        setRules(buildRuleRows(eligibilityRules))

        if (nextGroup.coach_user_id) {
          try {
            const { users } = await fetchUsers(api, session.user.orgId, { limit: 200 })
            if (!cancelled) {
              const match = users.find((u) => u.id === nextGroup.coach_user_id)
              setCoachName(match ? displayNameForUser(match) : nextGroup.coach_user_id)
            }
          } catch {
            if (!cancelled) setCoachName(nextGroup.coach_user_id)
          }
        } else if (!cancelled) {
          setCoachName(null)
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Failed to load group details.')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [api, groupId, session?.user.orgId])

  const tabs = useMemo(() => [
    { id: 'members', label: 'Members', count: members.length },
    { id: 'eligibility', label: 'Eligibility Rules', count: rules.length },
    { id: 'settings', label: 'Settings' },
  ], [members.length, rules.length])

  const assignableSwimmers = useMemo(() => {
    const assignedIds = new Set(memberLinks.map((link) => link.swimmer_id))
    return swimmers
      .filter((swimmer) => !assignedIds.has(swimmer.id))
      .sort((a, b) => {
        const nameA = `${a.preferred_name || a.first_name} ${a.last_name}`.trim()
        const nameB = `${b.preferred_name || b.first_name} ${b.last_name}`.trim()
        return nameA.localeCompare(nameB)
      })
  }, [memberLinks, swimmers])

  async function handleAssignSwimmer(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId || !group || !assignForm.swimmerId) return

    setAssigning(true)
    setAssignError(null)
    setAssignSuccess(null)
    try {
      await assignSwimmerToGroup(api, session.user.orgId, assignForm.swimmerId, {
        group_code: group.code,
        is_primary: assignForm.isPrimary,
      })

      const updatedLinks = await fetchSwimmerGroups(api, session.user.orgId)
      const nextMemberLinks = updatedLinks.filter((link) => link.group_code === group.code)
      setMemberLinks(nextMemberLinks)
      setMembers(buildSwimmerRows(swimmers, nextMemberLinks))
      setAssignSuccess('Swimmer added to this training group.')
      setAssignForm({ swimmerId: '', isPrimary: false })
    } catch (err) {
      setAssignError(err instanceof ApiError ? err.message : 'Failed to assign swimmer to group.')
    } finally {
      setAssigning(false)
    }
  }

  if (loading) {
    return <div style={{ padding: '24px', maxWidth: '1100px' }}><EmptyState icon="layers" title="Loading group" description="Fetching live group detail data from the API." /></div>
  }

  if (error || !group) {
    return <div style={{ padding: '24px', maxWidth: '1100px' }}><EmptyState icon="alert-circle" title="Unable to load group" description={error ?? 'Group not found.'} /></div>
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>
      <Link href="/members/groups" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '13px', color: 'var(--text-subtle)', textDecoration: 'none', marginBottom: '20px' }}>
        <Icon name="chevron-left" size={14} /> Groups
      </Link>

      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
            <h1 style={{ fontSize: '24px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2 }}>{group.name}</h1>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: 'var(--accent-subtle)', color: 'var(--accent)', fontFamily: 'var(--font-mono)', letterSpacing: '0.02em' }}>
              {group.type}
            </span>
            <span style={{ fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '10px', background: group.is_active ? 'var(--success-bg)' : 'var(--bg-elevated)', color: group.is_active ? 'var(--success)' : 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' }}>
              {group.is_active ? 'active' : 'archived'}
            </span>
          </div>
          <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
            {group.id} · {group.code}
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '28px' }}>
        {[
          { label: 'Swimmers', value: members.length.toString(), color: 'var(--text)' },
          { label: 'Coach', value: coachName || 'Unassigned', color: 'var(--text-subtle)' },
          { label: 'Age Range', value: `${group.min_age ?? '—'}-${group.max_age ?? '+'}`, color: 'var(--text)' },
          { label: 'Rules', value: rules.length.toString(), color: 'var(--accent)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '16px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'members' ? (
        <div>
          <div style={{ display: 'grid', gridTemplateColumns: allowWrite ? 'minmax(0, 1fr) 320px' : '1fr', gap: '16px', alignItems: 'start', marginBottom: '16px' }}>
            <div style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
              {group.description || 'No group description recorded yet.'}
            </div>
            {allowWrite && (
              <form className="card" onSubmit={handleAssignSwimmer} style={{ padding: '16px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <div>
                  <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>Add Swimmer To Group</h3>
                  <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.5 }}>Assign a swimmer to this training group so they appear on the live roster immediately.</p>
                </div>
                <select
                  value={assignForm.swimmerId}
                  onChange={(e) => setAssignForm((current) => ({ ...current, swimmerId: e.target.value }))}
                  style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }}
                  disabled={assigning || assignableSwimmers.length === 0}
                >
                  <option value="">{assignableSwimmers.length === 0 ? 'All swimmers already assigned' : 'Select swimmer'}</option>
                  {assignableSwimmers.map((swimmer) => (
                    <option key={swimmer.id} value={swimmer.id}>
                      {`${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim()}
                    </option>
                  ))}
                </select>
                <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                  <input
                    type="checkbox"
                    checked={assignForm.isPrimary}
                    onChange={(e) => setAssignForm((current) => ({ ...current, isPrimary: e.target.checked }))}
                    disabled={assigning}
                  />
                  Make this the swimmer&apos;s primary group
                </label>
                {assignError ? <div style={{ fontSize: '12px', color: 'var(--danger)' }}>{assignError}</div> : null}
                {assignSuccess ? <div style={{ fontSize: '12px', color: 'var(--success)' }}>{assignSuccess}</div> : null}
                <button className="btn btn-primary" type="submit" disabled={assigning || !assignForm.swimmerId}>
                  {assigning ? 'Adding...' : 'Add Swimmer'}
                </button>
              </form>
            )}
          </div>
          <Table columns={SWIMMER_COLS} data={members} emptyTitle="No members" emptyDescription="No swimmers are currently assigned to this group." />
        </div>
      ) : null}

      {activeTab === 'eligibility' ? (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Rules below are loaded from the persisted eligibility-rule store.</p>
          </div>
          <Table columns={RULE_COLS} data={rules} emptyTitle="No rules" emptyDescription="No eligibility rules are configured for this group." />
        </div>
      ) : null}

      {activeTab === 'settings' ? (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Group Settings</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[
                { label: 'Group ID', value: group.id },
                { label: 'Code', value: group.code },
                { label: 'Type', value: group.type },
                { label: 'Coach', value: coachName || '—' },
                { label: 'Min Age', value: group.min_age?.toString() || '—' },
                { label: 'Max Age', value: group.max_age?.toString() || 'No limit' },
                { label: 'Created', value: formatDate(group.created_at) },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px', borderBottom: '1px solid var(--border)', paddingBottom: '10px' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text)', fontWeight: 600 }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-subtle)', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '16px', fontFamily: 'var(--font-mono)' }}>Description</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.7 }}>{group.description || 'No description recorded yet.'}</p>
          </div>
        </div>
      ) : null}
    </div>
  )
}
