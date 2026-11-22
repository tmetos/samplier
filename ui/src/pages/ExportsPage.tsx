import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useProjects } from '../hooks/useProjects'

const DESCRIPTIONS: Record<string, string> = {
  CROSSTAB: 'Formatted Excel crosstab (analytes as columns, locations as rows)',
  CSV: 'Flat CSV of all results',
  WQX: 'EPA WQX / ESAR XML (water quality exchange)',
  AQS: 'EPA AQS raw data transactions (air quality)',
  EIM: 'Washington State EIM submission (CSV or XML)',
}

export function ExportsPage() {
  const { projects } = useProjects()
  const [projectId, setProjectId] = useState('')
  const activeId = projectId || projects[0]?.id || ''
  const [type, setType] = useState('EPA')
  const [detectsOnly, setDetectsOnly] = useState(false)

  const formats = useQuery({ queryKey: ['export-formats'], queryFn: api.exportFormats })

  function href(format: string) {
    const base = api.exportUrl(format, { projectId: activeId, type })
    return detectsOnly ? `${base}&detects_only=true` : base
  }

  return (
    <>
      <h1>Reports &amp; Exports</h1>
      <p className="sub">
        Deliverables for regulators and project tables. Every export respects the selected
        action level type.
      </p>

      <div className="panel">
        <div className="row" style={{ marginBottom: 16 }}>
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

          <label className="row" style={{ gap: 6 }}>
            <input
              type="checkbox"
              checked={detectsOnly}
              onChange={(e) => setDetectsOnly(e.target.checked)}
            />
            Detects only
          </label>
        </div>

        {!activeId && <p className="muted">Select a project to enable exports.</p>}

        {formats.data?.items?.length ? (
          <table>
            <thead>
              <tr>
                <th>Format</th>
                <th>Description</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {formats.data.items.map((f) => (
                <tr key={f}>
                  <td className="mono">{f}</td>
                  <td className="muted">{DESCRIPTIONS[f] ?? 'Export'}</td>
                  <td style={{ textAlign: 'right' }}>
                    <a
                      className="button"
                      href={activeId ? href(f) : undefined}
                      aria-disabled={!activeId}
                    >
                      Download
                    </a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p className="muted">Loading formats…</p>
        )}
      </div>
    </>
  )
}
