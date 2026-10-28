import type { ReactNode } from 'react'
import { InstallPrompt } from '@/components/pwa/InstallPrompt'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { PortalShell } from '@/components/portal/PortalShell'
import { DevRoleSwitcher } from '@/components/dev/DevRoleSwitcher'

export default function PortalLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard requiredRoles={['parent', 'admin']}>
      <PortalShell>{children}</PortalShell>
      <DevRoleSwitcher />
      <InstallPrompt />
    </AuthGuard>
  )
}
