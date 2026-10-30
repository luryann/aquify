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
import './page.inline.css'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

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
    <main className="deck is-37e75c2" >
      {error && (
        <div role="alert" className="is-a14160d">
          {error}
          <button type="button" className="btn btn-secondary" onClick={load}>Retry</button>
        </div>
      )}
      {!workout && !error && <PageSkeleton variant="list" noHeader rows={4} />}
      {workout && (
        <>
          <header className="is-bca94cc">
            <h1 className="is-73c31ca">{workout.title}</h1>
            <div className="is-3d2f320">
              <p className="is-1cab63f">
                {formatDistance(workout.totals.distance, workout.totals.unit)}
              </p>
              <p className="is-cac0e17">
                {workout.course}, about {formatDuration(workout.totals.estimated_seconds)}
              </p>
            </div>
          </header>
          <WorkoutSheet workout={workout} variant="deck" />
          <button type="button" className="btn btn-secondary deck-print is-ffa21d3" onClick={() => window.print()} >
            Print this workout
          </button>
        </>
      )}
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
