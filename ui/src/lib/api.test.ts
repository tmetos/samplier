import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, formatDay, formatResult, ogcCollection, ogcFeature, qs } from './api'

describe('formatDay', () => {
  it('keeps only the calendar date of an RFC3339 timestamp', () => {
    expect(formatDay('2024-03-09T23:30:00Z')).toBe('2024-03-09')
  })

  it('returns an empty string for missing input', () => {
    expect(formatDay(undefined)).toBe('')
    expect(formatDay(null)).toBe('')
  })
})

describe('qs', () => {
  it('returns empty string when no params', () => {
    expect(qs({})).toBe('')
  })

  it('omits empty and undefined values', () => {
    expect(qs({ a: 'x', b: undefined, c: '' })).toBe('?a=x')
  })

  it('encodes values', () => {
    expect(qs({ q: 'a b&c' })).toBe('?q=a+b%26c')
  })

  it('stringifies numbers', () => {
    expect(qs({ limit: 10 })).toBe('?limit=10')
  })
})

describe('formatResult', () => {
  it('renders a detect value', () => {
    expect(formatResult({ detect_flag: 'Y', result_numeric: 50.3 })).toBe('50.3')
  })

  it('renders a non-detect against the reporting limit', () => {
    expect(
      formatResult({ detect_flag: 'N', reporting_detection_limit: 5 }),
    ).toBe('< 5')
  })

  it('renders ND when a non-detect has no limit', () => {
    expect(formatResult({ detect_flag: 'N' })).toBe('ND')
  })

  it('falls back to result text', () => {
    expect(formatResult({ detect_flag: 'Y', result_text: 'PRESENT' })).toBe('PRESENT')
  })

  it('handles a zero detect value', () => {
    expect(formatResult({ detect_flag: 'Y', result_numeric: 0 })).toBe('0')
  })
})

describe('ApiError', () => {
  it('carries the status code', () => {
    const err = new ApiError(404, 'not found')
    expect(err.status).toBe(404)
    expect(err.message).toBe('not found')
    expect(err.name).toBe('ApiError')
    expect(err).toBeInstanceOf(Error)
  })
})

describe('ogcFeature', () => {
  afterEach(() => vi.restoreAllMocks())

  it('fetches a single feature by id', async () => {
    const feature = { type: 'Feature', properties: { name: 'Site A' }, geometry: null }
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify(feature), { status: 200 }))

    const res = await ogcFeature('projects', 'p1')
    expect(res.properties.name).toBe('Site A')
    expect(spy.mock.calls[0][0]).toBe('/ogc/collections/projects/items/p1')
  })

  it('throws ApiError when the feature is missing', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('missing', { status: 404, statusText: 'Not Found' }),
    )
    await expect(ogcFeature('projects', 'nope')).rejects.toThrow(ApiError)
  })
})

describe('ogcCollection', () => {
  afterEach(() => vi.restoreAllMocks())

  it('requests the collection and returns GeoJSON', async () => {
    const payload = {
      type: 'FeatureCollection',
      numberMatched: 1,
      numberReturned: 1,
      features: [],
    }
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))

    const result = await ogcCollection('locations', { limit: 10, bbox: '1,2,3,4' })
    expect(result.numberMatched).toBe(1)
    const url = spy.mock.calls[0][0] as string
    expect(url).toContain('/ogc/collections/locations/items')
    expect(url).toContain('limit=10')
    expect(url).toContain('bbox=1%2C2%2C3%2C4')
  })

  it('throws ApiError on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('nope', { status: 500, statusText: 'Server Error' }),
    )
    await expect(ogcCollection('locations')).rejects.toThrow(ApiError)
  })
})

describe('api client', () => {
  afterEach(() => vi.restoreAllMocks())

  function mockJson(body: unknown, status = 200) {
    // Return a fresh Response per call: bodies can only be read once.
    return vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(JSON.stringify(body), { status }))
  }

  it('lists projects with a limit', async () => {
    const spy = mockJson({ items: [], limit: 10, offset: 0 })
    const res = await (await import('./api')).api.listProjects(10)
    expect(res.limit).toBe(10)
    expect(spy.mock.calls[0][0]).toBe('/api/v1/projects?limit=10')
  })

  it('gets a project', async () => {
    const spy = mockJson({ id: 'p1', code: 'P-1' })
    const { api } = await import('./api')
    const p = await api.getProject('p1')
    expect(p.code).toBe('P-1')
    expect(spy.mock.calls[0][0]).toBe('/api/v1/projects/p1')
  })

  it('creates a project with POST', async () => {
    const spy = mockJson({ id: 'new' }, 201)
    const { api } = await import('./api')
    await api.createProject({ code: 'X', name: 'Y' })
    const init = spy.mock.calls[0][1] as RequestInit
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ code: 'X', name: 'Y' }))
  })

  it('updates and deletes a project', async () => {
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: 'p' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const { api } = await import('./api')
    await api.updateProject('p', { name: 'Z' })
    await expect(api.deleteProject('p')).resolves.toBeUndefined()
  })

  it('lists tasks and locations scoped to a project', async () => {
    const spy = mockJson({ items: [], limit: 1, offset: 0 })
    const { api } = await import('./api')
    await api.listTasks('proj-1')
    await api.listLocations('proj-1')
    expect(spy.mock.calls[0][0]).toContain('project_id=proj-1')
    expect(spy.mock.calls[1][0]).toContain('project_id=proj-1')
  })

  it('creates tasks and locations', async () => {
    const spy = mockJson({ id: 'x' }, 201)
    const { api } = await import('./api')
    await api.createTask({ project_id: 'p', code: 'T' })
    await api.createLocation({ project_id: 'p', name: 'MW-1' })
    expect((spy.mock.calls[0][1] as RequestInit).method).toBe('POST')
    expect((spy.mock.calls[1][1] as RequestInit).method).toBe('POST')
  })

  it('lists samples by task or location', async () => {
    const spy = mockJson({ items: [], limit: 1, offset: 0 })
    const { api } = await import('./api')
    await api.listSamples({ taskId: 't1' })
    await api.listSamples({ locationId: 'l1', limit: 5 })
    expect(spy.mock.calls[0][0]).toContain('task_id=t1')
    expect(spy.mock.calls[1][0]).toContain('location_id=l1')
    expect(spy.mock.calls[1][0]).toContain('limit=5')
  })

  it('lists tests and results', async () => {
    const spy = mockJson({ items: [], limit: 1, offset: 0 })
    const { api } = await import('./api')
    await api.listTests('s1')
    await api.listResults('t1')
    expect(spy.mock.calls[0][0]).toBe('/api/v1/samples/s1/tests')
    expect(spy.mock.calls[1][0]).toBe('/api/v1/tests/t1/results')
  })

  it('lists analytes, units and action levels', async () => {
    const spy = mockJson({ items: [] })
    const { api } = await import('./api')
    await api.listAnalytes()
    await api.listUnits()
    await api.listActionLevels({ type: 'EPA', projectId: 'p1' })
    expect(spy.mock.calls[0][0]).toContain('/analytes')
    expect(spy.mock.calls[1][0]).toBe('/api/v1/units')
    expect(spy.mock.calls[2][0]).toContain('type=EPA')
    expect(spy.mock.calls[2][0]).toContain('project_id=p1')
  })

  it('fetches exceedances', async () => {
    const spy = mockJson({ count: 2, items: [] })
    const { api } = await import('./api')
    const res = await api.exceedances('p1', 'EPA')
    expect(res.count).toBe(2)
    expect(spy.mock.calls[0][0]).toContain('/exceedances')
  })

  it('exposes EDD and export format endpoints', async () => {
    const spy = mockJson({ items: ['EFWEDD'] })
    const { api } = await import('./api')
    await api.eddFormats()
    await api.exportFormats()
    expect(spy.mock.calls[0][0]).toBe('/api/v1/edd/formats')
    expect(spy.mock.calls[1][0]).toBe('/api/v1/exports/formats')
  })

  it('builds export download URLs', async () => {
    const { api } = await import('./api')
    expect(api.exportUrl('CSV', { projectId: 'p1' })).toBe(
      '/api/v1/exports/CSV?project_id=p1',
    )
    expect(api.exportUrl('WQX', { projectId: 'p1', type: 'EPA' })).toContain('type=EPA')
  })

  it('fetches company stats and activity', async () => {
    const spy = mockJson({ items: [] })
    const { api } = await import('./api')
    await api.stats()
    await api.activity()
    await api.activity(5)
    expect(spy.mock.calls[0][0]).toBe('/api/v1/stats')
    expect(spy.mock.calls[1][0]).toBe('/api/v1/activity?limit=20')
    expect(spy.mock.calls[2][0]).toBe('/api/v1/activity?limit=5')
  })

  it('surfaces the API error message from the body', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: 'project_id is required' }), {
        status: 400,
        statusText: 'Bad Request',
      }),
    )
    const { api } = await import('./api')
    await expect(api.listProjects()).rejects.toThrow('project_id is required')
  })

  it('falls back to statusText for non-JSON errors', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('<html>oops</html>', { status: 502, statusText: 'Bad Gateway' }),
    )
    const { api } = await import('./api')
    await expect(api.listUnits()).rejects.toThrow('Bad Gateway')
  })
})
