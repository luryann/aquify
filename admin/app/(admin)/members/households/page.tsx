import { redirect } from 'next/navigation'

export default function LegacyHouseholdsListPage() {
  redirect('/members?tab=households')
}
