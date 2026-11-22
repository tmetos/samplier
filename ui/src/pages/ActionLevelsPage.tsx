import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useProjects } from '../hooks/useProjects'

export function ActionLevelsPage() {
  const { projects } = useProjects()
  const [projectId, setProjectId] = useState('')
  const activeId = projectId || projects[0]?.id || ''
  const [type, setType] = useState('EPA')

  const groups = useQuery({
    queryKey: ['action-levels', activeId, type],
    queryFn: () => api.listActionLevels({ type, projectId: activeId }),
  })

  return (
    <>
      <h1>Action Levels</h1>
      <p className="sub">
        EPA, state, local and custom limits compared against results during reporting.
      </p>

      <div className="panel">
        <div className="row" style={{ marginBottom: 14 }}>
          <label className="muted">Type</label>
          <select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="EPA">EPA</option>
            <option value="STATE">State</option>
            <option value="LOCAL">Local</option>
            <option value="CUSTOM">Custom</option>
          </select>
          <label className="muted">Project</label>
          <select value={activeId} onChange={(e) => setProjectId(e.target.value)}>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.code}
              </option>
            ))}
          </select>
        </div>

        {groups.error && <p className="error">{String(groups.error)}</p>}
        {groups.data?.items?.length ? (
          groups.data.items.map((g) => (
            <div key={g.id} style={{ marginBottom: 20 }}>
              <h2>
                {g.code} · {g.name}{' '}
                <span className="badge">{g.type_code}</span>
                {g.matrix && <span className="badge">{g.matrix}</span>}
              </h2>
              <table>
                <thead>
                  <tr>
                    <th>Analyte</th>
                    <th>CAS RN</th>
                    <th>Limit</th>
                    <th>Min</th>
                    <th>Unit</th>
                  </tr>
                </thead>
                <tbody>
                  {(g.parameters ?? []).map((p) => (
                    <tr key={p.id}>
                      <td>{p.analyte_name ?? p.analyte_id}</td>
                      <td className="mono">{p.cas_rn ?? ''}</td>
                      <td>{p.action_level}</td>
                      <td>{p.action_level_min ?? ''}</td>
                      <td>{p.unit}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))
        ) : (
          <p className="muted">No action level groups for this type yet.</p>
        )}
      </div>
    </>
  )
}
