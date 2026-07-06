import { redirect } from 'next/navigation'

export default function LegacyDocumentCenterPage() {
  redirect('/members?tab=documents')
}
