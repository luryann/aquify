import type { ReactNode } from 'react'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { AdminShell } from '@/components/layout/AdminShell'
import { DevRoleSwitcher } from '@/components/dev/DevRoleSwitcher'

export default function AdminLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard requiredRoles={['admin', 'coach']}>
      <AdminShell>{children}</AdminShell>
      <DevRoleSwitcher />
    </AuthGuard>
  )
}
