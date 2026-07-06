import { redirect } from 'next/navigation'

export default function LegacyGroupsListPage() {
  redirect('/members?tab=groups')
}
