'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'
import Link from 'next/link'

// ─── Types ────────────────────────────────────────────────────────────────────

type Course = 'SCY' | 'SCM' | 'LCM'

type SwimmerStat = {
  id: string; name: string; age: number; gender: 'M' | 'F'; group: string
  bestTimes: Record<string, number> // event -> seconds
  improvement30d: number // percent
  eventsSwum: number
}

type EventStats = {
  event: string; gender: 'M' | 'F'
  swimmerCount: number; avgTime: number; fastestTime: number
  topSwimmer: string
}

type TeamTrend = { month: string; avgImprovement: number; meetCount: number; pbCount: number }

// ─── Mock Data ────────────────────────────────────────────────────────────────

const MOCK_SWIMMERS: SwimmerStat[] = [
  { id: 'sw1', name: 'Marcus Allen',    age: 14, gender: 'M', group: 'Senior A',  bestTimes: { '100 Free': 50.23, '200 Free': 110.45, '100 Back': 58.99, '200 IM': 118.32 }, improvement30d: 1.8, eventsSwum: 8 },
  { id: 'sw2', name: 'Sofia Reyes',     age: 12, gender: 'F', group: 'Junior A',  bestTimes: { '100 Fly': 65.88, '200 Fly': 143.21, '100 Free': 58.45 }, improvement30d: 2.4, eventsSwum: 6 },
  { id: 'sw3', name: 'Jordan Kim',      age: 16, gender: 'M', group: 'Senior A',  bestTimes: { '100 Free': 48.12, '200 Free': 107.33, '100 Back': 55.02, '50 Free': 22.88 }, improvement30d: 0.9, eventsSwum: 10 },
  { id: 'sw4', name: 'Amara Okafor',    age: 13, gender: 'F', group: 'Junior A',  bestTimes: { '100 Free': 59.99, '200 Free': 131.20, '100 Back': 67.45 }, improvement30d: 3.1, eventsSwum: 7 },
  { id: 'sw5', name: 'Liam Torres',     age: 11, gender: 'M', group: 'Age Group 1', bestTimes: { '100 Free': 63.22, '50 Free': 28.10, '100 Back': 72.55 }, improvement30d: 4.5, eventsSwum: 5 },
  { id: 'sw6', name: 'Zoe Chen',        age: 15, gender: 'F', group: 'Senior B',  bestTimes: { '200 IM': 136.44, '100 Breast': 76.33, '200 Breast': 162.90 }, improvement30d: 1.2, eventsSwum: 7 },
  { id: 'sw7', name: 'Caleb Henderson', age: 13, gender: 'M', group: 'Junior A',  bestTimes: { '100 Free': 55.80, '200 Free': 122.11, '50 Free': 25.44 }, improvement30d: 2.0, eventsSwum: 6 },
  { id: 'sw8', name: 'Jenna Park',      age: 14, gender: 'F', group: 'Junior A',  bestTimes: { '200 IM': 138.44, '100 Fly': 69.88, '100 Free': 62.30 }, improvement30d: 1.7, eventsSwum: 8 },
]

const MOCK_EVENT_STATS: EventStats[] = [
  { event: '100 Free', gender: 'M', swimmerCount: 12, avgTime: 55.4,  fastestTime: 48.12, topSwimmer: 'Jordan Kim' },
  { event: '100 Free', gender: 'F', swimmerCount: 14, avgTime: 61.2,  fastestTime: 58.45, topSwimmer: 'Sofia Reyes' },
  { event: '200 Free', gender: 'M', swimmerCount: 10, avgTime: 118.8, fastestTime: 107.33, topSwimmer: 'Jordan Kim' },
  { event: '200 Free', gender: 'F', swimmerCount: 9,  avgTime: 134.1, fastestTime: 131.20, topSwimmer: 'Amara Okafor' },
  { event: '50 Free',  gender: 'M', swimmerCount: 15, avgTime: 27.3,  fastestTime: 22.88, topSwimmer: 'Jordan Kim' },
  { event: '50 Free',  gender: 'F', swimmerCount: 13, avgTime: 30.1,  fastestTime: 28.90, topSwimmer: 'Mia Henderson' },
  { event: '100 Back', gender: 'M', swimmerCount: 8,  avgTime: 63.0,  fastestTime: 55.02, topSwimmer: 'Jordan Kim' },
  { event: '100 Back', gender: 'F', swimmerCount: 7,  avgTime: 70.2,  fastestTime: 67.45, topSwimmer: 'Amara Okafor' },
  { event: '100 Fly',  gender: 'F', swimmerCount: 6,  avgTime: 70.1,  fastestTime: 65.88, topSwimmer: 'Sofia Reyes' },
  { event: '200 IM',   gender: 'M', swimmerCount: 7,  avgTime: 122.5, fastestTime: 118.32, topSwimmer: 'Marcus Allen' },
  { event: '200 IM',   gender: 'F', swimmerCount: 8,  avgTime: 140.2, fastestTime: 136.44, topSwimmer: 'Zoe Chen' },
  { event: '100 Breast', gender: 'F', swimmerCount: 5, avgTime: 80.3, fastestTime: 76.33, topSwimmer: 'Zoe Chen' },
]

const MOCK_TRENDS: TeamTrend[] = [
  { month: 'Sep 25', avgImprovement: 0.8, meetCount: 1, pbCount: 14 },
  { month: 'Oct 25', avgImprovement: 1.2, meetCount: 2, pbCount: 22 },
  { month: 'Nov 25', avgImprovement: 0.5, meetCount: 1, pbCount: 9 },
  { month: 'Dec 25', avgImprovement: 1.8, meetCount: 2, pbCount: 31 },
  { month: 'Jan 26', avgImprovement: 2.1, meetCount: 3, pbCount: 47 },
  { month: 'Feb 26', avgImprovement: 1.5, meetCount: 2, pbCount: 28 },
  { month: 'Mar 26', avgImprovement: 1.9, meetCount: 1, pbCount: 17 },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmtTime(sec: number): string {
  if (sec < 60) return sec.toFixed(2)
  const m = Math.floor(sec / 60)
  const s = (sec % 60).toFixed(2).padStart(5, '0')
  return `${m}:${s}`
}

const GROUPS = ['All Groups', 'Senior A', 'Senior B', 'Junior A', 'Junior B', 'Age Group 1', 'Age Group 2']
const COURSE_ADJUSTMENT: Record<Course, number> = {
  SCY: 1,
  SCM: 1.03,
  LCM: 1.08,
}

// ─── Team Improvement Bar Chart ───────────────────────────────────────────────

function TrendBar({ trend, maxPb }: { trend: TeamTrend; maxPb: number }) {
  const width = (trend.pbCount / maxPb) * 100
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '60px 1fr 80px', gap: '12px', alignItems: 'center', marginBottom: '10px' }}>
      <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textAlign: 'right' }}>{trend.month}</span>
      <div style={{ position: 'relative', height: '28px', background: 'var(--bg-elevated)', borderRadius: '4px', overflow: 'hidden' }}>
        <div style={{
          height: '100%', width: `${width}%`,
          background: 'linear-gradient(90deg, var(--accent), var(--accent)88)',
          borderRadius: '4px',
          transition: 'width 0.5s ease',
          display: 'flex', alignItems: 'center', paddingLeft: '8px',
        }}>
          {width > 20 && <span style={{ fontSize: '11px', color: '#fff', fontFamily: 'var(--font-mono)', fontWeight: 700 }}>{trend.pbCount} PBs</span>}
        </div>
      </div>
      <div style={{ textAlign: 'right' }}>
        <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: trend.avgImprovement >= 2 ? 'var(--success)' : trend.avgImprovement >= 1 ? 'var(--warning)' : 'var(--text-muted)' }}>
          {trend.avgImprovement >= 0 ? '+' : ''}{trend.avgImprovement.toFixed(1)}%
        </span>
      </div>
    </div>
  )
}

// ─── Swimmer Comparison ───────────────────────────────────────────────────────

function SwimmerRow({ swimmer, event, maxSec }: { swimmer: SwimmerStat; event: string; maxSec: number }) {
  const time = swimmer.bestTimes[event]
  if (time === undefined) return null
  const pct = (1 - time / maxSec) * 100

  return (
    <div style={{ display: 'grid', gridTemplateColumns: '160px 80px 1fr 80px', gap: '12px', alignItems: 'center', marginBottom: '10px' }}>
      <Link href={`/members/swimmers/${swimmer.id}`} style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)', textDecoration: 'none', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
        {swimmer.name}
      </Link>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: 700, color: 'var(--accent)', textAlign: 'right' }}>{fmtTime(time)}</span>
      <div style={{ height: '8px', background: 'var(--bg-elevated)', borderRadius: '4px', overflow: 'hidden' }}>
        <div style={{ height: '100%', width: `${Math.max(10, pct)}%`, background: 'var(--accent)', borderRadius: '4px' }} />
      </div>
      <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textAlign: 'right' }}>
        {swimmer.group}
      </span>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function PerformanceVisualizationPage() {
  const [course, setCourse] = useState<Course>('SCY')
  const [groupFilter, setGroupFilter] = useState('All Groups')
  const [genderFilter, setGenderFilter] = useState<'all' | 'M' | 'F'>('all')
  const [selectedEvent, setSelectedEvent] = useState('100 Free')
  const [activeView, setActiveView] = useState<'overview' | 'comparison' | 'trends' | 'event'>('overview')

  const adjustedSwimmers = MOCK_SWIMMERS.map((swimmer) => ({
    ...swimmer,
    bestTimes: Object.fromEntries(
      Object.entries(swimmer.bestTimes).map(([event, time]) => [event, Number((time * COURSE_ADJUSTMENT[course]).toFixed(2))]),
    ),
  }))

  const swimmers = adjustedSwimmers
    .filter((s) => groupFilter === 'All Groups' || s.group === groupFilter)
    .filter((s) => genderFilter === 'all' || s.gender === genderFilter)

  const eventStats = MOCK_EVENT_STATS
    .map((event) => ({
      ...event,
      avgTime: Number((event.avgTime * COURSE_ADJUSTMENT[course]).toFixed(2)),
      fastestTime: Number((event.fastestTime * COURSE_ADJUSTMENT[course]).toFixed(2)),
    }))
    .filter((e) => genderFilter === 'all' || e.gender === genderFilter)

  const allEvents = [...new Set(adjustedSwimmers.flatMap((s) => Object.keys(s.bestTimes)))]
  const maxPb = Math.max(...MOCK_TRENDS.map((t) => t.pbCount))

  // For comparison chart: get all swimmers who have the selected event
  const compSwimmers = adjustedSwimmers
    .filter((s) => s.bestTimes[selectedEvent] !== undefined)
    .filter((s) => genderFilter === 'all' || s.gender === genderFilter)
    .sort((a, b) => (a.bestTimes[selectedEvent] ?? 999) - (b.bestTimes[selectedEvent] ?? 999))
  const maxTime = Math.max(...compSwimmers.map((s) => s.bestTimes[selectedEvent] ?? 0)) * 1.1

  const VIEWS = ['overview', 'comparison', 'trends', 'event'] as const

  return (
    <div style={{ padding: '24px', maxWidth: '1300px' }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '28px' }}>
        <div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>Performance Visualization</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Team and swimmer performance analytics across events, groups, and time.</p>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          {(['SCY', 'SCM', 'LCM'] as Course[]).map((c) => (
            <button
              key={c}
              onClick={() => setCourse(c)}
              style={{
                fontSize: '12px', padding: '5px 14px', borderRadius: '20px', cursor: 'pointer',
                background: course === c ? 'var(--accent)' : 'var(--bg-elevated)',
                color: course === c ? '#fff' : 'var(--text-subtle)',
                border: `1px solid ${course === c ? 'var(--accent)' : 'var(--border)'}`,
                fontFamily: 'var(--font-mono)', fontWeight: 600, transition: 'all var(--dur-fast)',
              }}
            >{c}</button>
          ))}
        </div>
      </div>

      {/* Team stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Active Swimmers', value: swimmers.length, color: 'var(--text)', sub: `with ${course} times` },
          { label: 'Avg Improvement', value: (swimmers.reduce((a, s) => a + s.improvement30d, 0) / Math.max(swimmers.length, 1)).toFixed(1) + '%', color: 'var(--success)', sub: 'last 30 days' },
          { label: 'PBs This Month', value: 17, color: 'var(--accent)', sub: 'Mar 2026' },
          { label: 'Events Tracked', value: allEvents.length, color: 'var(--info)', sub: `across ${swimmers.length} swimmers` },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '16px 18px' }}>
            <div style={{ fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '26px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color, marginBottom: '2px' }}>{s.value}</div>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>{s.sub}</div>
          </div>
        ))}
      </div>

      {/* Filters + View tabs */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px', gap: '12px', flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
          <select value={groupFilter} onChange={(e) => setGroupFilter(e.target.value)} className="input" style={{ fontSize: '12px', padding: '5px 10px', minWidth: '130px' }}>
            {GROUPS.map((g) => <option key={g} value={g}>{g}</option>)}
          </select>
          {(['all', 'M', 'F'] as const).map((g) => (
            <button
              key={g}
              onClick={() => setGenderFilter(g)}
              style={{
                fontSize: '12px', padding: '5px 14px', borderRadius: '20px', cursor: 'pointer',
                background: genderFilter === g ? 'var(--accent)' : 'var(--bg-elevated)',
                color: genderFilter === g ? '#fff' : 'var(--text-subtle)',
                border: `1px solid ${genderFilter === g ? 'var(--accent)' : 'var(--border)'}`,
                transition: 'all var(--dur-fast)',
              }}
            >{g === 'all' ? 'All' : g === 'M' ? 'Male' : 'Female'}</button>
          ))}
        </div>
        <div style={{ display: 'flex', gap: '2px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', padding: '3px' }}>
          {VIEWS.map((v) => (
            <button
              key={v}
              onClick={() => setActiveView(v)}
              style={{
                padding: '5px 14px', borderRadius: 'calc(var(--r-md) - 2px)',
                fontSize: '12px', cursor: 'pointer',
                background: activeView === v ? 'var(--accent)' : 'transparent',
                color: activeView === v ? '#fff' : 'var(--text-subtle)',
                border: 'none', transition: 'all var(--dur-fast)',
                textTransform: 'capitalize',
              }}
            >{v}</button>
          ))}
        </div>
      </div>

      {/* ── Overview ── */}
      {activeView === 'overview' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          {/* Top improvers */}
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: '16px' }}>Top Improvers (30 Days)</h3>
            {[...swimmers].sort((a, b) => b.improvement30d - a.improvement30d).slice(0, 6).map((s, i) => (
              <div key={s.id} style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '12px', paddingBottom: '12px', borderBottom: '1px solid var(--border)' }}>
                <div style={{ width: '22px', height: '22px', borderRadius: '50%', background: i === 0 ? 'var(--accent)' : 'var(--bg-elevated)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                  <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: i === 0 ? '#fff' : 'var(--text-muted)' }}>{i + 1}</span>
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <Link href={`/members/swimmers/${s.id}`} style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text)', textDecoration: 'none', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                    onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--accent)')}
                    onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text)')}>
                    {s.name}
                  </Link>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>{s.group} · {s.eventsSwum} events</span>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '4px', flexShrink: 0 }}>
                  <Icon name="arrow-up" size={12} style={{ color: 'var(--success)' }} />
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--success)' }}>+{s.improvement30d.toFixed(1)}%</span>
                </div>
              </div>
            ))}
          </div>

          {/* Event coverage heatmap */}
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: '16px' }}>Event Coverage by Group</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {eventStats.map((e) => {
                const maxCount = 15
                const fill = (e.swimmerCount / maxCount) * 100
                return (
                  <div key={`${e.event}-${e.gender}`} style={{ display: 'grid', gridTemplateColumns: '110px 24px 1fr 80px', gap: '8px', alignItems: 'center' }}>
                    <span style={{ fontSize: '12px', color: 'var(--text-subtle)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{e.event}</span>
                    <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: e.gender === 'F' ? 'var(--accent)' : 'var(--info)', fontWeight: 700 }}>{e.gender}</span>
                    <div style={{ height: '6px', background: 'var(--bg-elevated)', borderRadius: '3px', overflow: 'hidden' }}>
                      <div style={{ height: '100%', width: `${fill}%`, background: e.gender === 'F' ? 'var(--accent)' : 'var(--info)', borderRadius: '3px' }} />
                    </div>
                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textAlign: 'right' }}>{e.swimmerCount} swimmers</span>
                  </div>
                )
              })}
            </div>
          </div>

          {/* Recent PBs */}
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: '16px' }}>Recent Personal Bests</h3>
            {[
              { swimmer: 'Marcus Allen', event: '100 Free', time: '50.23', date: '2026-02-14', prev: '51.10' },
              { swimmer: 'Sofia Reyes', event: '100 Fly', time: '1:05.88', date: '2026-01-20', prev: '1:07.21' },
              { swimmer: 'Liam Torres', event: '50 Free', time: '28.10', date: '2026-03-12', prev: '28.99' },
              { swimmer: 'Amara Okafor', event: '100 Back', time: '1:07.45', date: '2026-03-08', prev: '1:09.12' },
              { swimmer: 'Zoe Chen', event: '200 IM', time: '2:16.44', date: '2026-03-05', prev: '2:18.90' },
            ].map((pb, i) => (
              <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '10px', paddingBottom: '10px', borderBottom: '1px solid var(--border)' }}>
                <div style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                  <Icon name="zap" size={14} style={{ color: 'var(--accent)' }} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontWeight: 600, fontSize: '12px', color: 'var(--text)' }}>{pb.swimmer}</div>
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>{pb.event} · {pb.date}</div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0 }}>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', fontWeight: 700, color: 'var(--accent)' }}>{pb.time}</div>
                  <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textDecoration: 'line-through' }}>{pb.prev}</div>
                </div>
              </div>
            ))}
          </div>

          {/* Group comparison */}
          <div className="card" style={{ padding: '20px' }}>
            <h3 style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', marginBottom: '16px' }}>Group Activity</h3>
            {[
              { group: 'Senior A',    swimmers: 8,  practicesWeek: 10, meets: 3, avgImprovement: 1.4 },
              { group: 'Senior B',    swimmers: 6,  practicesWeek: 8,  meets: 2, avgImprovement: 1.1 },
              { group: 'Junior A',    swimmers: 10, practicesWeek: 8,  meets: 3, avgImprovement: 2.3 },
              { group: 'Junior B',    swimmers: 8,  practicesWeek: 6,  meets: 2, avgImprovement: 2.8 },
              { group: 'Age Group 1', swimmers: 12, practicesWeek: 5,  meets: 1, avgImprovement: 3.5 },
            ].map((g) => (
              <div key={g.group} style={{ display: 'grid', gridTemplateColumns: '120px 1fr 60px', gap: '12px', alignItems: 'center', marginBottom: '12px', paddingBottom: '12px', borderBottom: '1px solid var(--border)' }}>
                <span style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>{g.group}</span>
                <div>
                  <div style={{ fontSize: '10px', color: 'var(--text-muted)', marginBottom: '4px' }}>{g.swimmers} swimmers · {g.practicesWeek} practices/wk · {g.meets} meets</div>
                  <div style={{ height: '4px', background: 'var(--bg-elevated)', borderRadius: '2px', overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${(g.avgImprovement / 4) * 100}%`, background: 'var(--success)', borderRadius: '2px' }} />
                  </div>
                </div>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', fontWeight: 700, color: 'var(--success)', textAlign: 'right' }}>+{g.avgImprovement}%</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── Comparison ── */}
      {activeView === 'comparison' && (
        <div className="card" style={{ padding: '24px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600, color: 'var(--text)' }}>Swimmer Comparison by Event</h3>
            <select value={selectedEvent} onChange={(e) => setSelectedEvent(e.target.value)} className="input" style={{ fontSize: '13px', padding: '6px 12px', minWidth: '160px' }}>
              {allEvents.map((ev) => <option key={ev} value={ev}>{ev}</option>)}
            </select>
          </div>
          <div style={{ marginBottom: '12px', padding: '8px 12px', background: 'var(--bg-surface)', borderRadius: 'var(--r-sm)', border: '1px solid var(--border)' }}>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
              Showing {compSwimmers.length} swimmers with times in <strong style={{ color: 'var(--text)' }}>{selectedEvent} ({course})</strong>. Bar width = relative performance.
            </span>
          </div>
          {compSwimmers.length === 0 && (
            <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>No times recorded for this event in the filtered set.</div>
          )}
          {compSwimmers.map((s) => (
            <SwimmerRow key={s.id} swimmer={s} event={selectedEvent} maxSec={maxTime} />
          ))}
        </div>
      )}

      {/* ── Trends ── */}
      {activeView === 'trends' && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
          <div className="card" style={{ padding: '24px' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600, color: 'var(--text)', marginBottom: '20px' }}>Monthly PBs & Improvement</h3>
            {MOCK_TRENDS.map((t) => <TrendBar key={t.month} trend={t} maxPb={maxPb} />)}
            <div style={{ marginTop: '16px', padding: '10px 12px', background: 'var(--bg-surface)', borderRadius: 'var(--r-sm)', border: '1px solid var(--border)', fontSize: '11px', color: 'var(--text-muted)' }}>
              Bar height = # of personal bests set · Color = avg improvement percentage
            </div>
          </div>
          <div className="card" style={{ padding: '24px' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600, color: 'var(--text)', marginBottom: '20px' }}>Meet Participation</h3>
            {MOCK_TRENDS.map((t) => (
              <div key={t.month} style={{ display: 'grid', gridTemplateColumns: '60px 1fr 50px', gap: '12px', alignItems: 'center', marginBottom: '10px' }}>
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textAlign: 'right' }}>{t.month}</span>
                <div style={{ display: 'flex', gap: '4px' }}>
                  {Array.from({ length: t.meetCount }).map((_, i) => (
                    <div key={i} style={{ flex: 1, height: '28px', background: 'var(--info)', borderRadius: '4px', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                      <span style={{ fontSize: '10px', color: '#fff', fontFamily: 'var(--font-mono)', fontWeight: 700 }}>M</span>
                    </div>
                  ))}
                  {Array.from({ length: Math.max(0, 3 - t.meetCount) }).map((_, i) => (
                    <div key={`empty-${i}`} style={{ flex: 1, height: '28px', background: 'var(--bg-elevated)', borderRadius: '4px' }} />
                  ))}
                </div>
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textAlign: 'right' }}>{t.meetCount}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── Event Analytics ── */}
      {activeView === 'event' && (
        <div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '20px' }}>
            {eventStats.slice(0, 6).map((e) => (
              <div key={`${e.event}-${e.gender}`} className="card" style={{ padding: '18px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '12px' }}>
                  <div>
                    <div style={{ fontWeight: 700, fontSize: '14px', color: 'var(--text)', marginBottom: '2px' }}>{e.event}</div>
                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', padding: '1px 6px', borderRadius: '6px', background: e.gender === 'F' ? 'var(--accent-subtle)' : 'var(--info-bg)', color: e.gender === 'F' ? 'var(--accent)' : 'var(--info)', fontWeight: 700 }}>{e.gender === 'F' ? 'Female' : 'Male'}</span>
                  </div>
                  <span style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)', marginTop: '2px' }}>{e.swimmerCount}</span>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Fastest</span>
                    <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--accent)' }}>{fmtTime(e.fastestTime)}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Average</span>
                    <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>{fmtTime(e.avgTime)}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Top swimmer</span>
                    <span style={{ fontWeight: 600, color: 'var(--text)' }}>{e.topSwimmer}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
          {/* Fastest 100 Free breakdown */}
          <div className="card" style={{ padding: '24px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
              <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 600, color: 'var(--text)' }}>All Events — Top Times</h3>
              <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>{course} · filtered by selection above</p>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {eventStats.map((e) => (
                <div key={`${e.event}-${e.gender}`} style={{ display: 'grid', gridTemplateColumns: '110px 24px 120px 1fr 120px', gap: '12px', alignItems: 'center', padding: '10px 12px', background: 'var(--bg-surface)', borderRadius: 'var(--r-sm)', border: '1px solid var(--border)' }}>
                  <span style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text)' }}>{e.event}</span>
                  <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: e.gender === 'F' ? 'var(--accent)' : 'var(--info)', fontWeight: 700 }}>{e.gender}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontWeight: 700, fontSize: '14px', color: 'var(--accent)' }}>{fmtTime(e.fastestTime)}</span>
                  <span style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>{e.topSwimmer}</span>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)', textAlign: 'right' }}>avg {fmtTime(e.avgTime)} · {e.swimmerCount} swimmers</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
