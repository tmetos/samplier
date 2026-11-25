import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { pickProject, useProjects } from './useProjects'
import type { Project } from '../lib/api'

const project = (id: string): Project => ({
  id,
  code: id.toUpperCase(),
  name: `Project ${id}`,
  srid: 4326,
  elev_unit: 'ft',
})

afterEach(() => vi.restoreAllMocks())

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('useProjects', () => {
  it('loads projects through the API and exposes them', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ items: [project('a'), project('b')] }), {
        status: 200,
      }),
    )

    const { result } = renderHook(() => useProjects(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.projects.map((p) => p.id)).toEqual(['a', 'b'])
    expect(result.current.error).toBeNull()
  })

  it('defaults to an empty list when items is null', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ items: null }), { status: 200 }),
    )

    const { result } = renderHook(() => useProjects(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.projects).toEqual([])
  })

  it('surfaces a request error', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('boom', { status: 500, statusText: 'Server Error' }),
    )

    const { result } = renderHook(() => useProjects(), { wrapper })
    await waitFor(() => expect(result.current.error).toBeTruthy())
    expect(result.current.projects).toEqual([])
  })
})

describe('pickProject', () => {
  const projects = [project('a'), project('b')]

  it('keeps a selection that still exists', () => {
    expect(pickProject(projects, 'b')).toBe('b')
  })

  it('falls back to the first project when the selection is stale', () => {
    expect(pickProject(projects, 'gone')).toBe('a')
    expect(pickProject(projects, '')).toBe('a')
  })

  it('returns an empty string when there are no projects', () => {
    expect(pickProject([], 'a')).toBe('')
  })
})
