import type { ActivityEvent, CompanyStats, ProjectSummary } from '../lib/api'
import { Link } from 'react-router-dom'

const KIND_META: Record<string, { label: string; className: string }> = {
  import: { label: 'Import', className: 'kind-import' },
  sample: { label: 'Sample', className: 'kind-sample' },
  project: { label: 'Project', className: 'kind-project' },
  task: { label: 'Task', className: 'kind-task' },
}

/** Relative time, falling back to a date for older entries. */
export function relativeTime(iso: string): string {
  const then = new Date(iso.includes('T') ? iso : iso.replace(' ', 'T'))
  if (Number.isNaN(then.getTime())) return iso
  const diff = Date.now() - then.getTime()
  const mins = Math.round(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.round(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.round(hours / 24)
  if (days < 30) return `${days}d ago`
  return then.toLocaleDateString()
}

export function MetricCard({
  label,
  value,
  accent,
  to,
}: {
  label: string
  value: number | string
  accent?: 'green' | 'blue' | 'exceed'
  to?: string
}) {
  const body = (
    <>
      <div className="card-label">{label}</div>
      <div className={accent ? `card-value ${accent}` : 'card-value'}>{value}</div>
    </>
  )
  return to ? (
    <Link to={to} className="card card-link">
      {body}
    </Link>
  ) : (
    <div className="card">{body}</div>
  )
}

export function ProjectCard({ project }: { project: ProjectSummary }) {
  return (
    <Link to={`/projects?project=${project.id}`} className="project-card">
      <div className="project-card-head">
        <span className="mono project-code">{project.code}</span>
        <span className="project-arrow" aria-hidden="true">
          →
        </span>
      </div>
      <div className="project-name">{project.name}</div>
      <div className="project-stats">
        <span>
          <strong>{project.locations}</strong> locations
        </span>
        <span>
          <strong>{project.samples}</strong> samples
        </span>
        <span>
          <strong>{project.results}</strong> results
        </span>
      </div>
      {project.last_sample_date && (
        <div className="project-meta">Last sampled {project.last_sample_date}</div>
      )}
    </Link>
  )
}

export function ActivityFeed({ events }: { events: ActivityEvent[] }) {
  if (events.length === 0) {
    return <p className="muted">No activity yet. Import an EDD or create a project.</p>
  }
  return (
    <ul className="feed">
      {events.map((e) => {
        const meta = KIND_META[e.kind] ?? { label: e.kind, className: '' }
        return (
          <li key={`${e.kind}-${e.id}-${e.at}`} className="feed-item">
            <span className={`feed-kind ${meta.className}`}>{meta.label}</span>
            <div className="feed-body">
              <div className="feed-title">{e.title || '—'}</div>
              {e.detail && <div className="feed-detail">{e.detail}</div>}
            </div>
            <time className="feed-time" dateTime={e.at}>
              {relativeTime(e.at)}
            </time>
          </li>
        )
      })}
    </ul>
  )
}

/** Compact stat strip used at the top of a project workspace. */
export function StatStrip({
  items,
}: {
  items: { label: string; value: number | string; accent?: 'green' | 'blue' | 'exceed' }[]
}) {
  return (
    <div className="stat-strip">
      {items.map((i) => (
        <div key={i.label} className="stat">
          <div className="stat-value" data-accent={i.accent ?? ''}>
            {i.value}
          </div>
          <div className="stat-label">{i.label}</div>
        </div>
      ))}
    </div>
  )
}

export function statsOrEmpty(s: CompanyStats | undefined): CompanyStats {
  return (
    s ?? {
      projects: 0,
      tasks: 0,
      locations: 0,
      samples: 0,
      tests: 0,
      results: 0,
      analytes: 0,
      imports: 0,
      project_detail: [],
    }
  )
}
