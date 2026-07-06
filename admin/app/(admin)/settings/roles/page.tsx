import { redirect } from 'next/navigation'

export default function LegacyRoleManagementPage() {
  redirect('/users?tab=roles')
}
