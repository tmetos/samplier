import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { api, formatDay, ogcFeature, type Project } from '../lib/api'
import { BoundaryMap } from '../components/BoundaryMap'
import { StatStrip } from '../components/Widgets'

type Tab = 'overview' | 'tasks' | 'locations' | 'samples'

/** A project picker on the left; the selected project's data on the right. */
export function ProjectsPage() {
  const [params, setParams] = useSearchParams()
  const [newProjectOpen, setNewProjectOpen] = useState(false)

  const projects = useQuery({
    queryKey: ['projects'],
    queryFn: async () => (await api.listProjects()).items ?? [],
  })

  const list = projects.data ?? []
  const selectedFromUrl = params.get('project') ?? ''
  const selected: Project | undefined =
    list.find((p) => p.id === selectedFromUrl) ?? list[0]

  // Keep the URL in step with the effective selection so links are shareable.
  useEffect(() => {
    if (selected && selected.id !== selectedFromUrl) {
      setParams({ project: selected.id }, { replace: true })
    }
  }, [selected, selectedFromUrl, setParams])

  const select = (id: string) => setParams({ project: id })

  if (projects.error) {
    return <p className="error">Failed to load projects: {String(projects.error)}</p>
  }

  return (
    <>
      <h1>Projects &amp; sampling events</h1>
      <p className="sub">
        Select a project to see only its tasks, locations and samples.
      </p>

      <div className="split">
        <div className="panel" style={{ margin: 0 }}>
          <div className="section-head">
            <h2>Projects</h2>
            <span className="count">{list.length}</span>
            <div className="spacer" />
            <button
              type="button"
              className="secondary"
              onClick={() => setNewProjectOpen((v) => !v)}
            >
              {newProjectOpen ? 'Close' : 'New'}
            </button>
          </div>

          {newProjectOpen && (
            <NewProjectForm onDone={() => setNewProjectOpen(false)} />
          )}

          <div className="picker" role="listbox" aria-label="Projects">
            {projects.isLoading && <p className="empty">Loading…</p>}
            {!projects.isLoading && list.length === 0 && (
              <p className="empty">No projects yet — create one above.</p>
            )}
            {list.map((p) => (
              <button
                key={p.id}
                type="button"
                role="option"
                aria-selected={p.id === selected?.id}
                className="picker-item"
                onClick={() => select(p.id)}
              >
                <span className="picker-code mono">{p.code}</span>
                <span className="picker-name">{p.name}</span>
                <span className="picker-meta">SRID {p.srid}</span>
              </button>
            ))}
          </div>
        </div>

        <div>
          {!selected ? (
            <div className="panel">
              <p className="empty">Select or create a project to begin.</p>
            </div>
          ) : (
            <ProjectWorkspace project={selected} />
          )}
        </div>
      </div>
    </>
  )
}

function NewProjectForm({ onDone }: { onDone: () => void }) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState('')
  const [name, setName] = useState('')

  const create = useMutation({
    mutationFn: () => api.createProject({ code, name }),
    onSuccess: () => {
      setCode('')
      setName('')
      void queryClient.invalidateQueries({ queryKey: ['projects'] })
      void queryClient.invalidateQueries({ queryKey: ['stats'] })
      onDone()
    },
  })

  return (
    <form
      className="row"
      style={{ marginBottom: 12 }}
      onSubmit={(e) => {
        e.preventDefault()
        if (code && name) create.mutate()
      }}
    >
      <input
        placeholder="Code"
        value={code}
        onChange={(e) => setCode(e.target.value)}
        style={{ width: 90 }}
      />
      <input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
      <button type="submit" disabled={!code || !name || create.isPending}>
        Create
      </button>
      {create.error && <span className="error">{String(create.error)}</span>}
    </form>
  )
}

function ProjectWorkspace({ project }: { project: Project }) {
  const [tab, setTab] = useState<Tab>('overview')

  const tasks = useQuery({
    queryKey: ['tasks', project.id],
    queryFn: () => api.listTasks(project.id),
  })
  const locations = useQuery({
    queryKey: ['locations', project.id],
    queryFn: () => api.listLocations(project.id),
  })
  const samples = useQuery({
    queryKey: ['samples', project.id],
    queryFn: () => api.listSamples({ projectId: project.id }),
  })

  const taskList = useMemo(() => tasks.data?.items ?? [], [tasks.data])
  const locationList = useMemo(() => locations.data?.items ?? [], [locations.data])
  const sampleList = useMemo(() => samples.data?.items ?? [], [samples.data])

  const locationName = useMemo(() => {
    const m = new Map(locationList.map((l) => [l.id, l.name]))
    return (id: string) => m.get(id) ?? id.slice(0, 8)
  }, [locationList])

  const taskCode = useMemo(() => {
    const m = new Map(taskList.map((t) => [t.id, t.code]))
    return (id: string) => m.get(id) ?? id.slice(0, 8)
  }, [taskList])

  const boundary = useQuery({
    queryKey: ['project-boundary', project.id],
    queryFn: () => ogcFeature('projects', project.id),
    staleTime: 5 * 60_000,
    retry: false,
  })

  const boundaryInfo = useMemo(() => describeGeometry(boundary.data?.geometry ?? null), [boundary.data])

  const latest = useMemo(() => {
    const dates = sampleList.map((s) => s.sample_datetime).filter(Boolean).sort()
    return dates.length ? formatDay(dates[dates.length - 1]) : null
  }, [sampleList])

  return (
    <>
      <div className="panel" style={{ marginBottom: 14 }}>
        <div className="row" style={{ marginBottom: 6 }}>
          <h2 style={{ margin: 0 }}>{project.name}</h2>
          <span className="badge mono">{project.code}</span>
          <div className="spacer" />
          <a
            className="button secondary"
            href={`/ogc/collections/projects/items/${project.id}`}
            target="_blank"
            rel="noreferrer"
          >
            GeoJSON
          </a>
        </div>

        <div className="project-head">
          <div style={{ flex: 1, minWidth: 260 }}>
            <StatStrip
              items={[
                { label: 'Tasks', value: taskList.length, accent: 'blue' },
                { label: 'Locations', value: locationList.length, accent: 'green' },
                { label: 'Samples', value: sampleList.length, accent: 'blue' },
                { label: 'Latest sample', value: latest ?? '—' },
                { label: 'SRID', value: project.srid },
              ]}
            />
            {project.notes && <p className="muted" style={{ margin: 0 }}>{project.notes}</p>}
          </div>

          <figure style={{ margin: 0, width: 300 }}>
            {boundary.data?.geometry ? (
              <BoundaryMap feature={boundary.data} />
            ) : (
              <div className="boundary-missing">
                {boundary.isLoading ? 'Loading boundary…' : 'No boundary geometry'}
              </div>
            )}
            <figcaption className="muted" style={{ fontSize: 11, marginTop: 6 }}>
              Project boundary · {boundaryInfo}
            </figcaption>
          </figure>
        </div>
      </div>

      <div className="panel">
        <div className="tabs" role="tablist">
          {(
            [
              ['overview', 'Overview', null],
              ['tasks', 'Tasks', taskList.length],
              ['locations', 'Locations', locationList.length],
              ['samples', 'Samples', sampleList.length],
            ] as const
          ).map(([id, label, count]) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={tab === id}
              className="tab"
              onClick={() => setTab(id)}
            >
              {label}
              {count !== null && <span className="tab-count">{count}</span>}
            </button>
          ))}
        </div>

        {tab === 'overview' && (
          <Overview
            project={project}
            boundaryInfo={boundaryInfo}
            taskCount={taskList.length}
            locationCount={locationList.length}
            sampleCount={sampleList.length}
            latest={latest}
          />
        )}

        {tab === 'tasks' && (
          <TasksTab projectId={project.id} tasks={taskList} loading={tasks.isLoading} />
        )}

        {tab === 'locations' && (
          <LocationsTab
            projectId={project.id}
            locations={locationList}
            loading={locations.isLoading}
          />
        )}

        {tab === 'samples' && (
          <SamplesTab
            samples={sampleList}
            loading={samples.isLoading}
            locationName={locationName}
            taskCode={taskCode}
          />
        )}
      </div>
    </>
  )
}

/** Describe a GeoJSON geometry for the boundary caption. */
function describeGeometry(geom: { type: string; coordinates: unknown } | null): string {
  if (!geom) return '—'
  const count = (c: unknown): number =>
    Array.isArray(c) && typeof c[0] === 'number' ? 1 : Array.isArray(c) ? c.reduce((n, x) => n + count(x), 0) : 0

  switch (geom.type) {
    case 'Polygon':
      return `polygon · ${count(geom.coordinates)} vertices`
    case 'MultiPolygon':
      return `multipolygon · ${count(geom.coordinates)} vertices`
    case 'Point':
      return 'point'
    case 'LineString':
      return `line · ${count(geom.coordinates)} vertices`
    default:
      return geom.type.toLowerCase()
  }
}

function Overview({
  project,
  boundaryInfo,
  taskCount,
  locationCount,
  sampleCount,
  latest,
}: {
  project: Project
  boundaryInfo: string
  taskCount: number
  locationCount: number
  sampleCount: number
  latest: string | null
}) {
  return (
    <table>
      <tbody>
        <tr>
          <th style={{ width: 160 }}>Project code</th>
          <td className="mono">{project.code}</td>
        </tr>
        <tr>
          <th>Name</th>
          <td>{project.name}</td>
        </tr>
        <tr>
          <th>Boundary</th>
          <td>{boundaryInfo}</td>
        </tr>
        <tr>
          <th>Native SRID</th>
          <td>{project.srid}</td>
        </tr>
        <tr>
          <th>Elevation unit</th>
          <td>{project.elev_unit}</td>
        </tr>
        <tr>
          <th>Started</th>
          <td>{formatDay(project.start_time) || '—'}</td>
        </tr>
        <tr>
          <th>Sampling events</th>
          <td>{taskCount}</td>
        </tr>
        <tr>
          <th>Locations</th>
          <td>{locationCount}</td>
        </tr>
        <tr>
          <th>Samples</th>
          <td>
            {sampleCount}
            {latest ? ` · latest ${latest}` : ''}
          </td>
        </tr>
        <tr>
          <th>Notes</th>
          <td>{project.notes ?? '—'}</td>
        </tr>
      </tbody>
    </table>
  )
}

function TasksTab({
  projectId,
  tasks,
  loading,
}: {
  projectId: string
  tasks: { id: string; code: string; name?: string; start_date?: string; end_date?: string }[]
  loading: boolean
}) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState('')

  const create = useMutation({
    mutationFn: () => api.createTask({ project_id: projectId, code }),
    onSuccess: () => {
      setCode('')
      void queryClient.invalidateQueries({ queryKey: ['tasks', projectId] })
      void queryClient.invalidateQueries({ queryKey: ['stats'] })
    },
  })

  if (loading) return <p className="empty">Loading tasks…</p>

  return (
    <>
      <form
        className="row"
        style={{ marginBottom: 12 }}
        onSubmit={(e) => {
          e.preventDefault()
          if (code) create.mutate()
        }}
      >
        <input
          placeholder="New task code (e.g. Q3-2024)"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        />
        <button type="submit" className="secondary" disabled={!code || create.isPending}>
          Add sampling event
        </button>
        {create.error && <span className="error">{String(create.error)}</span>}
      </form>

      {tasks.length === 0 ? (
        <p className="empty">No sampling events for this project yet.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Code</th>
              <th>Name</th>
              <th>Start</th>
              <th>End</th>
            </tr>
          </thead>
          <tbody>
            {tasks.map((t) => (
              <tr key={t.id}>
                <td className="mono">{t.code}</td>
                <td>{t.name ?? ''}</td>
                <td>{t.start_date ?? ''}</td>
                <td>{t.end_date ?? ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}

function LocationsTab({
  projectId,
  locations,
  loading,
}: {
  projectId: string
  locations: {
    id: string
    name: string
    loc_type: string
    lon: number
    lat: number
    total_depth?: number
    depth_unit?: string
    status: string
  }[]
  loading: boolean
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [locType, setLocType] = useState('WELL')
  const [lon, setLon] = useState('')
  const [lat, setLat] = useState('')

  const create = useMutation({
    mutationFn: () =>
      api.createLocation({
        project_id: projectId,
        name,
        loc_type: locType,
        lon: Number(lon),
        lat: Number(lat),
      }),
    onSuccess: () => {
      setName('')
      setLon('')
      setLat('')
      void queryClient.invalidateQueries({ queryKey: ['locations', projectId] })
      void queryClient.invalidateQueries({ queryKey: ['stats'] })
    },
  })

  if (loading) return <p className="empty">Loading locations…</p>

  return (
    <>
      <form
        className="row"
        style={{ marginBottom: 12 }}
        onSubmit={(e) => {
          e.preventDefault()
          if (name && lon && lat) create.mutate()
        }}
      >
        <input
          placeholder="Location name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          style={{ width: 150 }}
        />
        <select value={locType} onChange={(e) => setLocType(e.target.value)}>
          {['WELL', 'BORING', 'SURFACE_WATER', 'OUTFALL', 'SOIL', 'AIR', 'SEDIMENT', 'OTHER'].map(
            (t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ),
          )}
        </select>
        <input
          placeholder="Longitude"
          value={lon}
          onChange={(e) => setLon(e.target.value)}
          style={{ width: 110 }}
        />
        <input
          placeholder="Latitude"
          value={lat}
          onChange={(e) => setLat(e.target.value)}
          style={{ width: 110 }}
        />
        <button
          type="submit"
          className="secondary"
          disabled={!name || !lon || !lat || create.isPending}
        >
          Add location
        </button>
        {create.error && <span className="error">{String(create.error)}</span>}
      </form>

      {locations.length === 0 ? (
        <p className="empty">No locations yet. Import an EDD or add one above.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Longitude</th>
              <th>Latitude</th>
              <th>Depth</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {locations.map((l) => (
              <tr key={l.id}>
                <td className="mono">{l.name}</td>
                <td>{l.loc_type}</td>
                <td>{l.lon.toFixed(5)}</td>
                <td>{l.lat.toFixed(5)}</td>
                <td>{l.total_depth != null ? `${l.total_depth} ${l.depth_unit ?? ''}` : ''}</td>
                <td className="muted">{l.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  )
}

function SamplesTab({
  samples,
  loading,
  locationName,
  taskCode,
}: {
  samples: {
    id: string
    name: string
    sample_type: string
    matrix: string
    sample_datetime: string
    start_depth?: number
    end_depth?: number
    depth_unit?: string
    location_id: string
    task_id: string
  }[]
  loading: boolean
  locationName: (id: string) => string
  taskCode: (id: string) => string
}) {
  if (loading) return <p className="empty">Loading samples…</p>
  if (samples.length === 0) {
    return <p className="empty">No samples for this project yet.</p>
  }

  return (
    <table>
      <thead>
        <tr>
          <th>Sample</th>
          <th>Location</th>
          <th>Event</th>
          <th>Date</th>
          <th>Matrix</th>
          <th>Type</th>
          <th>Depth</th>
        </tr>
      </thead>
      <tbody>
        {samples.map((s) => (
          <tr key={s.id}>
            <td className="mono">{s.name}</td>
            <td>{locationName(s.location_id)}</td>
            <td className="mono">{taskCode(s.task_id)}</td>
            <td>{formatDay(s.sample_datetime)}</td>
            <td>{s.matrix}</td>
            <td>
              <span className="badge">{s.sample_type}</span>
            </td>
            <td>
              {s.start_depth != null
                ? `${s.start_depth}${s.end_depth != null ? `–${s.end_depth}` : ''} ${s.depth_unit ?? ''}`
                : ''}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
