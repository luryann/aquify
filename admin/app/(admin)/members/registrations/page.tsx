import { redirect } from 'next/navigation'

export default function LegacyRegistrationsListPage() {
  redirect('/members?tab=registrations')
}
