'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ApiClient } from './api'
import { fetchStaff } from './adminData'

/**
 * Resolves staff user IDs (created_by, recorded-by, …) to display names via the
 * admin/coach staff directory. Unknown IDs fall back to a neutral label rather
 * than exposing raw UUIDs.
 */
export function useStaffNames(api: ApiClient, orgId: string | undefined): (userId?: string | null) => string {
  const [names, setNames] = useState<Record<string, string>>({})

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    fetchStaff(api, orgId)
      .then((staff) => { if (!cancelled) setNames(Object.fromEntries(staff.map((s) => [s.id, s.name]))) })
      .catch(() => { /* names are cosmetic; keep the fallback label */ })
    return () => { cancelled = true }
  }, [api, orgId])

  return useCallback((userId?: string | null) => {
    if (!userId) return 'System'
    return names[userId] ?? (userId === 'system' || userId.startsWith('system') ? 'System' : 'A staff member')
  }, [names])
}
