import { redirect } from 'next/navigation'

export default function LegacySuppressionPage() {
  redirect('/campaigns?tab=suppression')
}
