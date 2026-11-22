/**
 * Typed API client for Samplier's REST and OGC endpoints.
 */

export interface Project {
  id: string
  code: string
  name: string
  notes?: string
  srid: number
  start_time?: string
  end_time?: string
  elev_unit: string
}

export interface Task {
  id: string
  project_id: string
  code: string
  name?: string
  start_date?: string
  end_date?: string
}

export interface Location {
  id: string
  project_id: string
  sys_loc_code: number
  name: string
  loc_type: string
  lon: number
  lat: number
  total_depth?: number
  depth_unit?: string
  status: string
}

export interface Sample {
  id: string
  task_id: string
  location_id: string
  sys_sample_code: number
  name: string
  sample_type: string
  matrix: string
  start_depth?: number
  end_depth?: number
  depth_unit?: string
  sample_datetime: string
}

export interface Test {
  id: string
  sample_id: string
  test_id: string
  analytic_method?: string
  fraction?: string
  analysis_date?: string
}

export interface Result {
  id: string
  test_id: string
  analyte_id: string
  cas_rn?: string
  analyte_name?: string
  result_numeric?: number
  result_text?: string
  detect_flag?: string
  units?: string
  reporting_detection_limit?: number
  lab_qualifiers?: string
  validator_qualifiers?: string
}

export interface Analyte {
  id: string
  cas_rn?: string
  chemical_name: string
  short_name?: string
  aliases: string[]
}

export interface ActionLevelParameter {
  id: string
  analyte_id: string
  analyte_name?: string
  cas_rn?: string
  action_level: number
  action_level_min?: number
  unit: string
}

export interface ActionLevelGroup {
  id: string
  type_code: string
  code: string
  name: string
  matrix?: string
  parameters?: ActionLevelParameter[]
}

export interface ProjectSummary {
  id: string
  code: string
  name: string
  tasks: number
  locations: number
  samples: number
  results: number
  last_sample_date?: string
}

export interface CompanyStats {
  projects: number
  tasks: number
  locations: number
  samples: number
  tests: number
  results: number
  analytes: number
  imports: number
  project_detail: ProjectSummary[] | null
}

export interface ActivityEvent {
  kind: 'import' | 'sample' | 'project' | 'task' | string
  id: string
  title: string
  detail: string
  project_id?: string
  at: string
}

export interface ListResponse<T> {
  items: T[] | null
  limit: number
  offset: number
}

export interface GeoFeature {
  type: 'Feature'
  id: string
  geometry: { type: string; coordinates: unknown } | null
  properties: Record<string, unknown>
}

export interface FeatureCollection {
  type: 'FeatureCollection'
  numberMatched: number
  numberReturned: number
  features: GeoFeature[]
}

/** Error raised for non-2xx API responses. */
export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

const BASE = '/api/v1'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  })

  if (!res.ok) {
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** Render a result the way a regulator expects: "< DL" for non-detects. */
export function formatResult(r: Pick<Result, 'detect_flag' | 'result_numeric' | 'result_text' | 'reporting_detection_limit'>): string {
  if (r.detect_flag === 'N') {
    if (r.reporting_detection_limit != null) return `< ${r.reporting_detection_limit}`
    return 'ND'
  }
  if (r.result_numeric != null) return String(r.result_numeric)
  return r.result_text ?? ''
}

/**
 * Render an API timestamp as a calendar date. The API emits UTC (RFC3339), so
 * slicing the date portion is correct and never shifts across timezones.
 */
export function formatDay(iso: string | undefined | null): string {
  if (!iso) return ''
  return iso.slice(0, 10)
}

/** Build query strings, omitting empty values. */
export function qs(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '' ) continue
    search.set(k, String(v))
  }
  const s = search.toString()
  return s ? `?${s}` : ''
}

export const api = {
  listProjects: (limit = 100) =>
    request<ListResponse<Project>>(`/projects${qs({ limit })}`),
  getProject: (id: string) => request<Project>(`/projects/${id}`),
  createProject: (p: Partial<Project>) =>
    request<Project>('/projects', { method: 'POST', body: JSON.stringify(p) }),
  updateProject: (id: string, p: Partial<Project>) =>
    request<Project>(`/projects/${id}`, { method: 'PUT', body: JSON.stringify(p) }),
  deleteProject: (id: string) =>
    request<void>(`/projects/${id}`, { method: 'DELETE' }),

  listTasks: (projectId: string, limit = 200) =>
    request<ListResponse<Task>>(`/tasks${qs({ project_id: projectId, limit })}`),
  createTask: (t: Partial<Task>) =>
    request<Task>('/tasks', { method: 'POST', body: JSON.stringify(t) }),

  listLocations: (projectId: string, limit = 500) =>
    request<ListResponse<Location>>(`/locations${qs({ project_id: projectId, limit })}`),
  createLocation: (l: Partial<Location>) =>
    request<Location>('/locations', { method: 'POST', body: JSON.stringify(l) }),

  listSamples: (params: {
    projectId?: string
    taskId?: string
    locationId?: string
    limit?: number
  }) =>
    request<ListResponse<Sample>>(
      `/samples${qs({
        project_id: params.projectId,
        task_id: params.taskId,
        location_id: params.locationId,
        limit: params.limit ?? 200,
      })}`,
    ),

  listTests: (sampleId: string) =>
    request<ListResponse<Test>>(`/samples/${sampleId}/tests`),
  listResults: (testId: string) =>
    request<ListResponse<Result>>(`/tests/${testId}/results`),

  listAnalytes: (limit = 500) =>
    request<ListResponse<Analyte>>(`/analytes${qs({ limit })}`),
  listUnits: () => request<{ items: { code: string; name: string }[] }>('/units'),
  listActionLevels: (params: { type?: string; projectId?: string }) =>
    request<{ items: ActionLevelGroup[] }>(
      `/action-levels${qs({ type: params.type, project_id: params.projectId })}`,
    ),

  exceedances: (projectId: string, type?: string) =>
    request<{ count: number; items: Record<string, unknown>[] }>(
      `/exceedances${qs({ project_id: projectId, type })}`,
    ),

  /** Company-wide counts and per-project rollups. */
  stats: () => request<CompanyStats>('/stats'),

  /** Recent imports and record changes, newest first. */
  activity: (limit = 20) =>
    request<{ items: ActivityEvent[] | null }>(`/activity${qs({ limit })}`),

  eddFormats: () => request<{ items: string[] }>('/edd/formats'),
  exportFormats: () => request<{ items: string[] }>('/exports/formats'),

  /** Trigger a file download for an export format. */
  exportUrl: (format: string, params: { projectId: string; type?: string }) =>
    `${BASE}/exports/${format}${qs({ project_id: params.projectId, type: params.type })}`,
}

/** Fetch a single OGC feature (e.g. a project boundary). */
export async function ogcFeature(
  collection: string,
  id: string,
): Promise<import('../components/BoundaryMap').GeoFeature> {
  const res = await fetch(`/ogc/collections/${collection}/items/${id}`)
  if (!res.ok) throw new ApiError(res.status, res.statusText)
  return (await res.json()) as import('../components/BoundaryMap').GeoFeature
}

/** Fetch an OGC feature collection as GeoJSON. */
export async function ogcCollection(
  collection: string,
  params: { bbox?: string; limit?: number; offset?: number; datetime?: string } = {},
): Promise<FeatureCollection> {
  const res = await fetch(
    `/ogc/collections/${collection}/items${qs({ ...params })}`,
  )
  if (!res.ok) throw new ApiError(res.status, res.statusText)
  return (await res.json()) as FeatureCollection
}
