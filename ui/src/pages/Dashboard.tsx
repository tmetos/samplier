import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import {
  ActivityFeed,
  MetricCard,
  ProjectCard,
  statsOrEmpty,
} from '../components/Widgets'

export function Dashboard() {
  const stats = useQuery({ queryKey: ['stats'], queryFn: api.stats })
  const activity = useQuery({
    queryKey: ['activity', 12],
    queryFn: () => api.activity(12),
  })

  const s = statsOrEmpty(stats.data)
  const projects = s.project_detail ?? []
  const events = activity.data?.items ?? []

  if (stats.error) {
    return <p className="error">Failed to load dashboard: {String(stats.error)}</p>
  }

  return (
    <>
      <h1>Dashboard</h1>
      <p className="sub">
        Company-wide view · {s.projects} project{s.projects === 1 ? '' : 's'} ·{' '}
        {s.samples} samples · {s.results} results
      </p>

      <div className="cards">
        <MetricCard label="Projects" value={s.projects} to="/projects" />
        <MetricCard label="Sampling tasks" value={s.tasks} accent="blue" to="/projects" />
        <MetricCard label="Locations" value={s.locations} accent="green" to="/map" />
        <MetricCard label="Samples" value={s.samples} accent="blue" to="/projects" />
        <MetricCard label="Results" value={s.results} to="/results" />
        <MetricCard label="EDD imports" value={s.imports} to="/import" />
      </div>

      <div className="section-head">
        <h2>Projects</h2>
        <span className="count">{projects.length}</span>
        <div className="spacer" />
        <Link to="/projects" className="button secondary">
          All projects
        </Link>
      </div>

      <div className="project-grid" style={{ marginBottom: 22 }}>
        {projects.length === 0 ? (
          <p className="empty">
            No projects yet. <Link to="/projects">Create your first project</Link>.
          </p>
        ) : (
          projects.map((p) => <ProjectCard key={p.id} project={p} />)
        )}
      </div>

      <div className="split">
        <div className="panel">
          <div className="section-head">
            <h2>Quick actions</h2>
          </div>
          <div className="quick-grid" style={{ gridTemplateColumns: '1fr' }}>
            <Link to="/projects" className="quick">
              <div className="quick-title">Browse projects &amp; sampling events</div>
              <div className="quick-desc">Tasks, locations and samples per project</div>
            </Link>
            <Link to="/map" className="quick">
              <div className="quick-title">View site boundaries on the map</div>
              <div className="quick-desc">Project areas, tasks, locations, samples</div>
            </Link>
            <Link to="/import" className="quick">
              <div className="quick-title">Import a lab EDD</div>
              <div className="quick-desc">EFWEDD 4-file, validated before commit</div>
            </Link>
            <Link to="/results" className="quick">
              <div className="quick-title">Check exceedances</div>
              <div className="quick-desc">Results against EPA / state / local limits</div>
            </Link>
          </div>
        </div>

        <div className="panel">
          <div className="section-head">
            <h2>Recent activity</h2>
            <span className="count">{events.length}</span>
          </div>
          <ActivityFeed events={events} />
        </div>
      </div>
    </>
  )
}
