import { useQuery } from '@tanstack/react-query'
import { api, type Project } from '../lib/api'

/** Load projects and expose a helper for the selected project. */
export function useProjects() {
  const query = useQuery({
    queryKey: ['projects'],
    queryFn: async (): Promise<Project[]> => {
      const res = await api.listProjects()
      return res.items ?? []
    },
  })
  return {
    projects: query.data ?? [],
    isLoading: query.isLoading,
    error: query.error,
  }
}

/** Pick a project id, preferring the stored selection. */
export function pickProject(projects: Project[], selected: string): string {
  if (selected && projects.some((p) => p.id === selected)) return selected
  return projects[0]?.id ?? ''
}
