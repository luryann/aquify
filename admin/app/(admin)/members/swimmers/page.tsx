import { redirect } from 'next/navigation'

export default function LegacySwimmersListPage() {
  redirect('/members?tab=swimmers')
}
