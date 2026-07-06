import type { ReactNode } from 'react'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { SwimmerShell } from '@/components/swimmer/SwimmerShell'

export default function SwimmerLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard requiredRoles={['swimmer', 'admin']}>
      <SwimmerShell>{children}</SwimmerShell>
    </AuthGuard>
  )
}
