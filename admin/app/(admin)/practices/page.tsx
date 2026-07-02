import { redirect } from 'next/navigation'

import { legacyPracticesRedirectTarget } from '@/lib/practices-route'

export default function PracticesPage() {
  redirect(legacyPracticesRedirectTarget())
}
