import { redirect } from 'next/navigation'

// Root route — redirect to login.
// The temporary API dev console has been retired.
export default function RootPage() {
  redirect('/login')
}
