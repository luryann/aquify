import { redirect } from 'next/navigation'

export default function LegacyInvoicesPage() {
  redirect('/billing?tab=invoices')
}
