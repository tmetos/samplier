import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, formatResult } from '../lib/api'
import { useProjects } from '../hooks/useProjects'

export function ResultsPage() {
  const { projects } = useProjects()
  const [projectId, setProjectId] = useState('')
  const activeId = projectId || projects[0]?.id || ''
  const [type, setType] = useState('EPA')

  const exceedances = useQuery({
    queryKey: ['exceedances', activeId, type],
    queryFn: () => api.exceedances(activeId, type),
    enabled: Boolean(activeId),
  })

  return (
    <>
      <h1>Results</h1>
      <p className="sub">
        Query analytical results and compare them against regulatory action levels.
      </p>

      <div className="panel">
        <div className="row" style={{ marginBottom: 14 }}>
          <label className="muted">Project</label>
          <select value={activeId} onChange={(e) => setProjectId(e.target.value)}>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.code} — {p.name}
              </option>
            ))}
          </select>

          <label className="muted">Action level type</label>
          <select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="EPA">EPA</option>
            <option value="STATE">State</option>
            <option value="LOCAL">Local</option>
            <option value="CUSTOM">Custom</option>
          </select>

          <div className="spacer" />
          <a className="button secondary" href={api.exportUrl('CSV', { projectId: activeId, type })}>
            Download CSV
          </a>
          <a className="button" href={api.exportUrl('CROSSTAB', { projectId: activeId, type })}>
            Crosstab (XLSX)
          </a>
        </div>

        {!activeId && <p className="muted">Select a project to view results.</p>}
        {exceedances.error && <p className="error">{String(exceedances.error)}</p>}

        {activeId && (
          <>
            <h2>
              Exceedances{' '}
              <span className={exceedances.data?.count ? 'badge exceed' : 'badge'}>
                {exceedances.data?.count ?? 0}
              </span>
            </h2>
            {exceedances.data?.items?.length ? (
              <table>
                <thead>
                  <tr>
                    <th>Location</th>
                    <th>Sample</th>
                    <th>Date</th>
                    <th>Analyte</th>
                    <th>CAS RN</th>
                    <th>Result</th>
                    <th>Unit</th>
                    <th>Limit</th>
                    <th>Group</th>
                  </tr>
                </thead>
                <tbody>
                  {exceedances.data.items.map((row, i) => (
                    <tr key={i} className="is-exceed">
                      <td className="mono">{String(row.location)}</td>
                      <td className="mono">{String(row.sample)}</td>
                      <td>{String(row.sample_date)}</td>
                      <td>{String(row.analyte)}</td>
                      <td className="mono">{String(row.cas_rn ?? '')}</td>
                      <td className="exceed">
                        {formatResult({
                          detect_flag: row.detect_flag == null ? 'Y' : String(row.detect_flag),
                          result_numeric:
                            row.result_numeric == null ? undefined : Number(row.result_numeric),
                          result_text: row.result == null ? undefined : String(row.result),
                          reporting_detection_limit:
                            row.reporting_detection_limit == null
                              ? undefined
                              : Number(row.reporting_detection_limit),
                        })}
                      </td>
                      <td>{String(row.unit ?? '')}</td>
                      <td>{row.action_level == null ? '' : String(row.action_level)}</td>
                      <td className="muted">{String(row.group ?? '')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">No exceedances for this project and action level type.</p>
            )}
          </>
        )}
      </div>
    </>
  )
}
