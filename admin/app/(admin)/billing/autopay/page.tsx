import { redirect } from 'next/navigation'

export default function LegacyAutopayPage() {
  redirect('/billing?tab=autopay')
}
