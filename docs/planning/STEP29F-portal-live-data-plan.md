# STEP 29F: Parent & Swimmer Portal Live-Data Conversion

## Goal
Replace all hardcoded mock/stub data in parent portal and swimmer portal pages with live
API-backed queries, preserving existing UI designs exactly.

## Session Data Available
- `session.user.orgId` — org scope for all requests
- `session.user.householdId` — parent's household (optional, may be undefined for non-parent roles)
- `session.token` — bearer token

## Implementation Pattern (per page)
```tsx
const { session } = useSession()
const api = useApiClient(session?.token)
const orgId = session?.user.orgId ?? ''
const householdId = session?.user.householdId ?? ''
const [data, setData] = useState<T[]>([])
const [loading, setLoading] = useState(true)
useEffect(() => {
  if (!orgId) { setLoading(false); return }
  fetchXxx(api, orgId).then(setData).catch(console.error).finally(() => setLoading(false))
}, [api, orgId])
```

## Pages to Convert

### Parent Portal (`admin/app/(portal)/portal/`)
| Page | API Sources | Status |
|------|-------------|--------|
| `dashboard/page.tsx` | fetchSwimmers (filter by householdId), fetchMeets, fetchMeetCommitments, fetchInvoicesForHousehold | ✅ Done |
| `meets/page.tsx` | fetchMeets, fetchMeetCommitments per meet | ✅ Done |
| `meets/[id]/page.tsx` | fetchMeet, fetchMeetSessions, fetchMeetEvents, fetchMeetCommitments; POST respond | ✅ Done |
| `billing/page.tsx` | fetchInvoicesForHousehold, fetchInvoicePayments | ✅ Done |
| `billing/invoices/[id]/page.tsx` | fetchInvoice, fetchInvoiceItems, fetchInvoicePayments | ✅ Done |
| `registrations/page.tsx` | fetchRegistrations, fetchSwimmers, fetchGroups | ✅ Done |
| `communications/page.tsx` | fetchAnnouncements('published') | ✅ Done |
| `calendar/page.tsx` | fetchMeets (convert to calendar events) | ✅ Done |
| `household/page.tsx` | fetchHousehold, fetchHouseholdContacts, fetchSwimmers | ✅ Done |
| `household/swimmers/[id]/page.tsx` | fetchSwimmer, fetchSwimmerBestTimes, fetchSwimmerGroups | ✅ Done |

### Swimmer Portal (`admin/app/(swimmer)/swim/`)
| Page | API Sources | Status |
|------|-------------|--------|
| `page.tsx` | fetchSwimmers (first for household), fetchMeets, fetchMeetCommitments, fetchSwimmerBestTimes, fetchAnnouncements | ✅ Done |
| `performance/page.tsx` | fetchSwimmerBestTimes | ✅ Done |
| `schedule/page.tsx` | fetchMeets (convert to schedule events) | ✅ Done |
| `meets/[id]/page.tsx` | fetchMeet, fetchMeetEvents, fetchMeetCommitments; POST respond | ✅ Done |
| `profile/page.tsx` | fetchSwimmer, fetchSwimmerGroups, fetchGroups | ✅ Done |

## Key Data Mappings

### AnnouncementRecord → Display
- `published_at` → publishedAt
- `audience` → category (approximate)
- `created_by` → author

### BestTimeRecord → Display
- `event_name` → event
- `course` → SCY/SCM/LCM
- `best_time_ms` → formatted time string

### Time Format Helper
```ts
function fmtTime(ms: number): string {
  const s = ms / 1000
  const m = Math.floor(s / 60)
  const rem = s % 60
  return m > 0 ? `${m}:${rem.toFixed(2).padStart(5, '0')}` : rem.toFixed(2)
}
```

### Commitment Status Mapping
Backend statuses: `invited`, `committed_yes`, `committed_no`, `coach_reviewed`, `locked`
Display mappings:
- `invited` → 'pending'
- `committed_yes` → 'yes'
- `committed_no` → 'no'
- `coach_reviewed` / `locked` → 'closed'
- (no commitment record) → 'open'

## Regression Coverage
A Node-based test helper at `admin/__tests__/portal-regression.test.ts` validates:
1. Parent portal dashboard loads with live session data
2. Parent can view meets and commitment status
3. Parent billing overview loads household invoices
4. Swimmer portal dashboard resolves swimmer from householdId
5. Swimmer performance page loads best times
6. Parent workflows complete without swimmer login

## Notes
- Swimmer portal: no `swimmerId` in session — resolve by fetching swimmers for householdId and using first match
- Calendar: backend has no "practice" or "holiday" record type yet (29H scope), so calendar shows meets + deadline events only
- Document center: wired to `/portal/documents` which remains mostly static (29G scope)
- Notification preferences: stored client-side only (no backend endpoint yet)
