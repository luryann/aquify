'use client'

import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchWorkout, type Workout } from '@/lib/trainingData'
import { formatDistance, formatDuration } from '@/lib/workout-format'
import { WorkoutSheet } from '@/components/training/WorkoutSheet'
import { errorText } from '@/app/(admin)/training/WorkoutComposer'

// Pool-deck view: one workout in very large type for a tablet on the deck or
// a printed sheet in a page protector. No app chrome.
function DeckView() {
  const { id } = useParams<{ id: string }>()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [workout, setWorkout] = useState<Workout | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!orgId || !id) return
    setError(null)
    try {
      setWorkout((await fetchWorkout(api, orgId, id)).workout)
    } catch (err) {
      setError(errorText(err, 'Couldn’t load this workout. Try again.'))
    }
  }, [api, orgId, id])

  useEffect(() => { void load() }, [load])

  return (
    <main className="deck" style={{ minHeight: '100vh', background: 'var(--bg)', color: 'var(--text)', padding: 'clamp(16px, 4vw, 48px)' }}>
      {error && (
        <div role="alert" style={{ fontSize: 'var(--fs-xl)', color: 'var(--error)', display: 'flex', gap: '12px', alignItems: 'center' }}>
          {error}
          <button type="button" className="btn btn-secondary" onClick={load}>Retry</button>
        </div>
      )}
      {!workout && !error && <p style={{ fontSize: 'var(--fs-xl)', color: 'var(--text-muted)' }}>Loading workout…</p>}
      {workout && (
        <>
          <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: '24px', flexWrap: 'wrap', marginBottom: '32px', borderBottom: '3px solid var(--accent)', paddingBottom: '20px' }}>
            <h1 style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'clamp(40px, 7vw, 76px)', lineHeight: 1, margin: 0, overflowWrap: 'anywhere' }}>{workout.title}</h1>
            <div style={{ textAlign: 'right', marginLeft: 'auto' }}>
              <p style={{ fontFamily: 'var(--font-mono)', fontSize: 'clamp(26px, 4vw, 40px)', fontWeight: 600, margin: 0 }}>
                {formatDistance(workout.totals.distance, workout.totals.unit)}
              </p>
              <p style={{ fontSize: 'var(--fs-xl)', color: 'var(--text-subtle)', margin: '4px 0 0' }}>
                {workout.course}, about {formatDuration(workout.totals.estimated_seconds)}
              </p>
            </div>
          </header>
          <WorkoutSheet workout={workout} variant="deck" />
          <button type="button" className="btn btn-secondary deck-print" onClick={() => window.print()} style={{ marginTop: '40px', fontSize: 'var(--fs-lg)' }}>
            Print this workout
          </button>
        </>
      )}
      <style>{`
        @media print {
          .deck { padding: 0 !important; background: #fff !important; color: #000 !important; }
          .deck-print { display: none !important; }
          .deck section { break-inside: avoid; }
        }
      `}</style>
    </main>
  )
}

export default function DeckWorkoutPage() {
  return (
    <AuthGuard requiredRoles={['admin', 'coach']}>
      <DeckView />
    </AuthGuard>
  )
}
