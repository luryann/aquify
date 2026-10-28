'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { Table, type Column } from '@/components/ui/Table'
import { EmptyState } from '@/components/ui/EmptyState'
import { useToast } from '@/components/ui/Toast'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  assignSwimmerToGroup,
  attachGroupEligibilityRule,
  fetchGroup,
  fetchGroupEligibilityRules,
  fetchStaff,
  fetchSwimmerGroups,
  fetchSwimmers,
  updateGroup,
  type EligibilityRuleRecord,
  type GroupRecord,
  type StaffRecord,
  type SwimmerGroupLinkRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { parseBackendDate } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

const FIELD_STYLE: React.CSSProperties = { width: '100%' }
const FIELD_LABEL_STYLE: React.CSSProperties = { display: 'block', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '6px', fontFamily: 'var(--font-body)' }

const GROUP_TYPES = ['competitive', 'recreational', 'developmental', 'masters', 'other'] as const
const RULE_GENDERS = [
  { value: 'female', label: 'Female' },
  { value: 'male', label: 'Male' },
  { value: 'non_binary', label: 'Non-binary' },
  { value: 'undisclosed', label: 'Undisclosed' },
]
const RULE_REGISTRATION_STATUSES = ['pending', 'active', 'inactive', 'archived']

type SettingsForm = {
  name: string
  type: string
  minAge: string
  maxAge: string
  coachUserId: string
  description: string
  isActive: boolean
}

function settingsFormFromGroup(group: GroupRecord): SettingsForm {
  return {
    name: group.name,
    type: group.type,
    minAge: group.min_age != null ? String(group.min_age) : '',
    maxAge: group.max_age != null ? String(group.max_age) : '',
    coachUserId: group.coach_user_id ?? '',
    description: group.description ?? '',
    isActive: group.is_active,
  }
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

function formatAgeRange(minAge: number | null | undefined, maxAge: number | null | undefined) {
  if (minAge == null && maxAge == null) return 'All ages'
  if (minAge == null) return `Up to ${maxAge}`
  if (maxAge == null) return `${minAge}+`
  return `${minAge}-${maxAge}`
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  const parsed = parseBackendDate(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function computeAge(dob?: string) {
  if (!dob) return '—'
  const birth = parseBackendDate(dob)
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
        <Link href={`/members/swimmers/${row.id}`} className="u-fw-600 u-fs-fs-base" style={{ color: 'var(--text)', textDecoration: 'none' }}>
          {row.name}
        </Link>
        <div className="u-fs-fs-xs u-ff-font-body u-mt-2px" style={{ color: 'var(--text-muted)' }}>{row.ussNumber || 'No external ID'}</div>
      </div>
    ),
  },
  { key: 'age', label: 'Age', align: 'right', render: (row) => <span className="u-ff-font-body u-fs-fs-base" style={{ color: 'var(--text)' }}>{row.age}</span> },
  { key: 'gender', label: 'Gender', render: (row) => <span className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>{row.gender}</span> },
  { key: 'isPrimary', label: 'Primary group?', render: (row) => <span className="badge" style={{ background: row.isPrimary ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: row.isPrimary ? 'var(--accent)' : 'var(--text-muted)' }}>{row.isPrimary ? 'Yes' : 'No'}</span> },
  { key: 'status', label: 'Status', render: (row) => <span className="badge" style={{ background: row.status === 'active' ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.status === 'active' ? 'var(--success)' : 'var(--text-muted)' }}>{row.status}</span> },
  { key: 'joinedGroup', label: 'Since', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatDate(row.joinedGroup)}</span> },
]

const RULE_COLS: Column<RuleRow>[] = [
  { key: 'ruleType', label: 'Type', render: (row) => <span className="u-ff-font-body u-fs-fs-sm u-p-2px-8px" style={{ borderRadius: '8px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', color: 'var(--text-subtle)' }}>{row.ruleType}</span> },
  { key: 'description', label: 'Description', render: (row) => <span className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>{row.description}</span> },
  { key: 'createdAt', label: 'Created', render: (row) => <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatDate(row.createdAt)}</span> },
  { key: 'active', label: 'Status', render: (row) => <span className="badge" style={{ background: row.active ? 'var(--success-bg)' : 'var(--bg-elevated)', color: row.active ? 'var(--success)' : 'var(--text-muted)' }}>{row.active ? 'active' : 'disabled'}</span> },
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
  const toast = useToast()
  const params = useParams<{ id: string }>()
  const groupId = typeof params?.id === 'string' ? params.id : ''
  const { session } = useSession()
  const api = useApiClient(session?.token)
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
  const [staff, setStaff] = useState<StaffRecord[]>([])
  const [settingsForm, setSettingsForm] = useState<SettingsForm | null>(null)
  const [savingSettings, setSavingSettings] = useState(false)
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [settingsSuccess, setSettingsSuccess] = useState<string | null>(null)
  const [ruleForm, setRuleForm] = useState({ ruleType: 'age_range', minAge: '', maxAge: '', allowed: [] as string[] })
  const [addingRule, setAddingRule] = useState(false)
  const [ruleError, setRuleError] = useState<string | null>(null)
  const isAdmin = (session?.user.roles ?? (session?.user.role ? [session.user.role] : [])).includes('admin')

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
        const [nextGroup, nextSwimmers, swimmerGroups, eligibilityRules, nextStaff] = await Promise.all([
          fetchGroup(api, session.user.orgId, groupId),
          fetchSwimmers(api, session.user.orgId),
          fetchSwimmerGroups(api, session.user.orgId),
          fetchGroupEligibilityRules(api, session.user.orgId, groupId),
          fetchStaff(api, session.user.orgId).catch(() => [] as StaffRecord[]),
        ])

        if (cancelled) return

        const nextMemberLinks = swimmerGroups.filter((link) => link.group_code === nextGroup.code)
        setGroup(nextGroup)
        setSwimmers(nextSwimmers)
        setMemberLinks(nextMemberLinks)
        setMembers(buildSwimmerRows(nextSwimmers, nextMemberLinks))
        setRules(buildRuleRows(eligibilityRules))
        setStaff(nextStaff)
        setSettingsForm(settingsFormFromGroup(nextGroup))
        setCoachName(coachLabel(nextGroup, nextStaff))
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load group details. Try again.'))
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
    { id: 'eligibility', label: 'Eligibility rules', count: rules.length },
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

  function coachLabel(target: GroupRecord, directory: StaffRecord[]): string | null {
    if (!target.coach_user_id) return null
    return directory.find((member) => member.id === target.coach_user_id)?.name ?? 'Former staff member'
  }

  async function handleSaveSettings(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId || !group || !settingsForm) return
    const name = settingsForm.name.trim()
    if (!name) {
      setSettingsError('Group name is required.')
      return
    }
    const minAge = settingsForm.minAge.trim() === '' ? undefined : Number(settingsForm.minAge)
    const maxAge = settingsForm.maxAge.trim() === '' ? undefined : Number(settingsForm.maxAge)
    if (minAge != null && maxAge != null && minAge > maxAge) {
      setSettingsError('Min age cannot be greater than max age.')
      return
    }
    setSavingSettings(true)
    setSettingsError(null)
    setSettingsSuccess(null)
    try {
      const prev = group
      const orgId = session.user.orgId
      const updated = await updateGroup(api, orgId, group.id, {
        name,
        type: settingsForm.type,
        description: settingsForm.description.trim(),
        min_age: minAge,
        max_age: maxAge,
        coach_user_id: settingsForm.coachUserId,
        is_active: settingsForm.isActive,
      })
      setGroup(updated)
      setSettingsForm(settingsFormFromGroup(updated))
      setCoachName(coachLabel(updated, staff))
      setSettingsSuccess('Group settings saved.')
      toast.successWithAction('Group settings saved', {
        label: 'Undo',
        onClick: () => {
          void updateGroup(api, orgId, prev.id, {
            name: prev.name, type: prev.type, description: prev.description ?? '',
            min_age: prev.min_age, max_age: prev.max_age,
            coach_user_id: prev.coach_user_id, is_active: prev.is_active,
          }).then((g) => {
            setGroup(g)
            setSettingsForm(settingsFormFromGroup(g))
            setCoachName(coachLabel(g, staff))
          }).catch((err) => setSettingsError(uiErrorMessage(err, 'Couldn’t undo. Try again.')))
        },
      })
    } catch (err) {
      setSettingsError(uiErrorMessage(err, 'Couldn’t save group settings. Try again.'))
    } finally {
      setSavingSettings(false)
    }
  }

  async function handleAddRule(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId || !group) return
    let parameters: Record<string, unknown>
    if (ruleForm.ruleType === 'age_range') {
      if (ruleForm.minAge.trim() === '' && ruleForm.maxAge.trim() === '') {
        setRuleError('Enter a minimum and/or maximum age.')
        return
      }
      parameters = {}
      if (ruleForm.minAge.trim() !== '') parameters.min_age = Number(ruleForm.minAge)
      if (ruleForm.maxAge.trim() !== '') parameters.max_age = Number(ruleForm.maxAge)
    } else {
      if (ruleForm.allowed.length === 0) {
        setRuleError('Select at least one allowed value.')
        return
      }
      parameters = { allowed: ruleForm.allowed }
    }
    setAddingRule(true)
    setRuleError(null)
    try {
      await attachGroupEligibilityRule(api, session.user.orgId, group.id, {
        rule_type: ruleForm.ruleType as 'age_range' | 'gender' | 'registration_status',
        parameters,
      })
      const nextRules = await fetchGroupEligibilityRules(api, session.user.orgId, group.id)
      setRules(buildRuleRows(nextRules))
      setRuleForm({ ruleType: ruleForm.ruleType, minAge: '', maxAge: '', allowed: [] })
    } catch (err) {
      setRuleError(uiErrorMessage(err, 'Couldn’t add eligibility rule. Try again.'))
    } finally {
      setAddingRule(false)
    }
  }

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
      setAssignError(uiErrorMessage(err, 'Couldn’t assign swimmer to group. Try again.'))
    } finally {
      setAssigning(false)
    }
  }

  if (loading) {
    return <div className="page"><EmptyState icon="layers" title="Loading group" description="Fetching live group detail data from the API." /></div>
  }

  if (error || !group) {
    return <div className="page"><EmptyState icon="alert-circle" title="Unable to load group" description={error ?? 'Group not found.'} /></div>
  }

  return (
    <div className="page">
      <Link href="/members?tab=groups" className="u-gap-4px u-fs-fs-base u-mb-20px" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)', textDecoration: 'none' }}>
        <Icon name="chevron-left" size={14} /> Groups
      </Link>

      <div className="page-header">
        <div>
          <div className="u-gap-6px-12px u-mb-4px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
            <h1 className="page-title">{group.name}</h1>
            <span className="badge" style={{ background: 'var(--accent-subtle)', color: 'var(--accent-text, var(--accent))' }}>
              {group.type}
            </span>
            <span className="badge" style={{ background: group.is_active ? 'var(--success-bg)' : 'var(--bg-elevated)', color: group.is_active ? 'var(--success)' : 'var(--text-muted)' }}>
              {group.is_active ? 'active' : 'archived'}
            </span>
          </div>
          <div className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
            {group.code}
          </div>
        </div>
      </div>

      <div className="cols-4 u-gap-12px u-mb-24px" >
        {[
          { label: 'Swimmers', value: members.length.toString(), color: 'var(--text)' },
          { label: 'Coach', value: coachName || 'Unassigned', color: 'var(--text-subtle)' },
          { label: 'Age range', value: formatAgeRange(group.min_age, group.max_age), color: 'var(--text)' },
          { label: 'Rules', value: rules.length.toString(), color: 'var(--accent-text, var(--accent))' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-lg u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />

      {activeTab === 'members' ? (
        <div>
          <div className="split-aside u-gap-16px u-mb-16px" >
            <div className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>
              {group.description || 'No group description recorded yet.'}
            </div>
            <form className="card u-p-16px u-gap-12px" onSubmit={handleAssignSwimmer} style={{ display: 'flex', flexDirection: 'column' }}>
                <div>
                  <h3 className="card-title u-mb-4px" >Add swimmer to group</h3>
                  <p className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>Assign a swimmer to this training group so they appear on the roster immediately.</p>
                </div>
                <select
                  value={assignForm.swimmerId}
                  onChange={(e) => setAssignForm((current) => ({ ...current, swimmerId: e.target.value }))}
                  className="u-p-8px-12px u-fs-fs-base" style={{ width: '100%' }}
                  disabled={assigning || assignableSwimmers.length === 0}
                >
                  <option value="">{assignableSwimmers.length === 0 ? 'All swimmers already assigned' : 'Select swimmer'}</option>
                  {assignableSwimmers.map((swimmer) => (
                    <option key={swimmer.id} value={swimmer.id}>
                      {`${swimmer.preferred_name || swimmer.first_name} ${swimmer.last_name}`.trim()}
                    </option>
                  ))}
                </select>
                <label className="u-gap-8px u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
                  <input
                    type="checkbox"
                    checked={assignForm.isPrimary}
                    onChange={(e) => setAssignForm((current) => ({ ...current, isPrimary: e.target.checked }))}
                    disabled={assigning}
                  />
                  Make this the swimmer&apos;s primary group
                </label>
                {assignError ? <div className="u-fs-fs-sm" style={{ color: 'var(--danger)' }}>{assignError}</div> : null}
                {assignSuccess ? <div className="u-fs-fs-sm" style={{ color: 'var(--success)' }}>{assignSuccess}</div> : null}
                <button className="btn btn-primary" type="submit" disabled={assigning || !assignForm.swimmerId}>
                  {assigning ? 'Adding...' : 'Add swimmer'}
                </button>
            </form>
          </div>
          <Table columns={SWIMMER_COLS} data={members} emptyTitle="No members" emptyDescription="No swimmers are currently assigned to this group." />
        </div>
      ) : null}

      {activeTab === 'eligibility' ? (
        <div>
          <div className={isAdmin ? 'split-aside' : undefined} style={{ display: 'grid', gap: '16px', alignItems: 'start' }}>
            <Table columns={RULE_COLS} data={rules} emptyTitle="No rules" emptyDescription="No eligibility rules are configured for this group." />
            {isAdmin ? (
              <form className="card u-p-16px u-gap-12px" onSubmit={handleAddRule} style={{ display: 'flex', flexDirection: 'column' }}>
                <div>
                  <h3 className="card-title u-mb-4px" >Add eligibility rule</h3>
                  <p className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>Rules are checked when swimmers are evaluated for this group.</p>
                </div>
                <select
                  aria-label="Rule type"
                  value={ruleForm.ruleType}
                  onChange={(e) => setRuleForm({ ruleType: e.target.value, minAge: '', maxAge: '', allowed: [] })}
                  style={FIELD_STYLE}
                  disabled={addingRule}
                >
                  <option value="age_range">Age range</option>
                  <option value="gender">Gender</option>
                  <option value="registration_status">Registration status</option>
                </select>
                {ruleForm.ruleType === 'age_range' ? (
                  <div className="cols-2 u-gap-8px" >
                    <input aria-label="Rule minimum age" type="number" min="0" max="99" placeholder="Min age" value={ruleForm.minAge} onChange={(e) => setRuleForm((f) => ({ ...f, minAge: e.target.value }))} style={FIELD_STYLE} />
                    <input aria-label="Rule maximum age" type="number" min="0" max="99" placeholder="Max age" value={ruleForm.maxAge} onChange={(e) => setRuleForm((f) => ({ ...f, maxAge: e.target.value }))} style={FIELD_STYLE} />
                  </div>
                ) : (
                  <div className="u-gap-6px" style={{ display: 'flex', flexDirection: 'column' }}>
                    {(ruleForm.ruleType === 'gender'
                      ? RULE_GENDERS
                      : RULE_REGISTRATION_STATUSES.map((value) => ({ value, label: value.charAt(0).toUpperCase() + value.slice(1) }))
                    ).map((option) => (
                      <label key={option.value} className="u-gap-8px u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
                        <input
                          type="checkbox"
                          checked={ruleForm.allowed.includes(option.value)}
                          onChange={(e) => setRuleForm((f) => ({
                            ...f,
                            allowed: e.target.checked ? [...f.allowed, option.value] : f.allowed.filter((v) => v !== option.value),
                          }))}
                        />
                        {option.label}
                      </label>
                    ))}
                  </div>
                )}
                {ruleError ? <div className="u-fs-fs-sm" style={{ color: 'var(--danger)' }}>{ruleError}</div> : null}
                <button className="btn btn-primary" type="submit" disabled={addingRule}>
                  {addingRule ? 'Adding...' : 'Add Rule'}
                </button>
              </form>
            ) : null}
          </div>
        </div>
      ) : null}

      {activeTab === 'settings' && settingsForm ? (
        <div className="split-pair u-gap-16px" >
          <form className="card u-p-20px u-gap-16px" onSubmit={handleSaveSettings} style={{ display: 'flex', flexDirection: 'column' }}>
            <h3 className="card-title">Edit group</h3>
            <div className="cols-2 u-gap-12px" >
              <div>
                <label htmlFor="group-name" style={FIELD_LABEL_STYLE}>Name</label>
                <input id="group-name" required value={settingsForm.name} onChange={(e) => setSettingsForm((f) => f && ({ ...f, name: e.target.value }))} style={FIELD_STYLE} />
              </div>
              <div>
                <label htmlFor="group-type" style={FIELD_LABEL_STYLE}>Type</label>
                <select id="group-type" value={settingsForm.type} onChange={(e) => setSettingsForm((f) => f && ({ ...f, type: e.target.value }))} style={FIELD_STYLE}>
                  {GROUP_TYPES.map((type) => (
                    <option key={type} value={type}>{type.charAt(0).toUpperCase() + type.slice(1)}</option>
                  ))}
                </select>
              </div>
              <div>
                <label htmlFor="group-min-age" style={FIELD_LABEL_STYLE}>Min age</label>
                <input id="group-min-age" type="number" min="0" max="99" value={settingsForm.minAge} onChange={(e) => setSettingsForm((f) => f && ({ ...f, minAge: e.target.value }))} style={FIELD_STYLE} />
              </div>
              <div>
                <label htmlFor="group-max-age" style={FIELD_LABEL_STYLE}>Max age</label>
                <input id="group-max-age" type="number" min="0" max="99" value={settingsForm.maxAge} onChange={(e) => setSettingsForm((f) => f && ({ ...f, maxAge: e.target.value }))} style={FIELD_STYLE} />
              </div>
              <div>
                <label htmlFor="group-coach" style={FIELD_LABEL_STYLE}>Coach</label>
                <select id="group-coach" value={settingsForm.coachUserId} onChange={(e) => setSettingsForm((f) => f && ({ ...f, coachUserId: e.target.value }))} style={FIELD_STYLE}>
                  <option value="">Unassigned</option>
                  {settingsForm.coachUserId && !staff.some((member) => member.id === settingsForm.coachUserId) ? (
                    <option value={settingsForm.coachUserId}>Former staff member</option>
                  ) : null}
                  {staff.map((member) => (
                    <option key={member.id} value={member.id}>{member.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <span style={FIELD_LABEL_STYLE}>Status</span>
                <label className="u-gap-8px u-fs-fs-base u-p-8px-0" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
                  <input type="checkbox" checked={settingsForm.isActive} onChange={(e) => setSettingsForm((f) => f && ({ ...f, isActive: e.target.checked }))} />
                  Active (uncheck to archive)
                </label>
              </div>
            </div>
            <div>
              <label htmlFor="group-description" style={FIELD_LABEL_STYLE}>Description</label>
              <textarea id="group-description" rows={3} value={settingsForm.description} onChange={(e) => setSettingsForm((f) => f && ({ ...f, description: e.target.value }))} style={{ ...FIELD_STYLE, resize: 'vertical' }} />
            </div>
            {settingsError ? <div className="u-fs-fs-sm" style={{ color: 'var(--danger)' }}>{settingsError}</div> : null}
            {settingsSuccess ? <div className="u-fs-fs-sm" style={{ color: 'var(--success)' }}>{settingsSuccess}</div> : null}
            <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button type="button" className="btn" disabled={savingSettings} onClick={() => { setSettingsForm(settingsFormFromGroup(group)); setSettingsError(null); setSettingsSuccess(null) }}>Reset</button>
              <button type="submit" className="btn btn-primary" disabled={savingSettings}>{savingSettings ? 'Saving…' : 'Save Changes'}</button>
            </div>
          </form>
          <div className="card u-p-20px" >
            <h3 className="card-title u-mb-16px" >Identifiers</h3>
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              {[
                { label: 'Code', value: group.code },
                { label: 'Group ID', value: group.id },
                { label: 'Created', value: formatDate(group.created_at) },
              ].map((row) => (
                <div key={row.label} className="u-gap-12px u-fs-fs-base u-pb-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
                  <span style={{ color: 'var(--text-muted)' }}>{row.label}</span>
                  <span className="u-ff-font-body u-fw-600" style={{ color: 'var(--text)', overflowWrap: 'anywhere', textAlign: 'right' }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}
