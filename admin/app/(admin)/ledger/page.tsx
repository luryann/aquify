import { redirect } from 'next/navigation'

export default function LegacyLedgerPage() {
  redirect('/billing?tab=ledger')
}
