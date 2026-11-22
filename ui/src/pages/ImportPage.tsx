import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useProjects } from '../hooks/useProjects'

interface Finding {
  section: string
  row: number
  field?: string
  message: string
  severity: 'ERROR' | 'WARNING'
}

interface ValidationResponse {
  format: string
  valid: boolean
  errors: number
  warnings: number
  findings: Finding[] | null
  sections: string[] | null
}

export function ImportPage() {
  const { projects } = useProjects()
  const [projectId, setProjectId] = useState('')
  const activeId = projectId || projects[0]?.id || ''
  const [taskId, setTaskId] = useState('')
  const [format, setFormat] = useState('EFWEDD')
  const [commitMode, setCommitMode] = useState('INSERT_UPDATE')
  const [file, setFile] = useState<File | null>(null)
  const [result, setResult] = useState<ValidationResponse | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const tasks = useQuery({
    queryKey: ['tasks', activeId],
    queryFn: () => api.listTasks(activeId),
    enabled: Boolean(activeId),
  })
  const formats = useQuery({ queryKey: ['edd-formats'], queryFn: api.eddFormats })

  async function submit(validateOnly: boolean) {
    if (!file) return
    setBusy(true)
    setError(null)
    setResult(null)

    const body = new FormData()
    body.set('file', file)
    if (!validateOnly) {
      body.set('project_id', activeId)
      body.set('task_id', taskId)
      body.set('commit_mode', commitMode)
      body.set('file_name', file.name)
    }

    const path = validateOnly ? '/api/v1/edd/validate' : '/api/v1/edd/load'
    try {
      const res = await fetch(`${path}?format=${encodeURIComponent(format)}`, {
        method: 'POST',
        body,
      })
      const json = (await res.json()) as unknown
      if (!res.ok) {
        const payload = json as { error?: string; findings?: Finding[] }
        setError(payload.error ?? `Request failed (${res.status})`)
        if (payload.findings) {
          setResult({ format, valid: false, errors: payload.findings.length, warnings: 0, findings: payload.findings, sections: [] })
        }
      } else if (validateOnly) {
        setResult(json as ValidationResponse)
      } else {
        setResult({ format, valid: true, errors: 0, warnings: 0, findings: [], sections: [] })
      }
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <h1>EDD Import</h1>
      <p className="sub">
        Load laboratory deliverables. EFWEDD (4-file lab format) is supported; XLSX, CSV, TSV and ZIP are accepted.
      </p>

      <div className="panel">
        <div className="row" style={{ marginBottom: 12 }}>
          <label className="muted">Format</label>
          <select value={format} onChange={(e) => setFormat(e.target.value)}>
            {(formats.data?.items ?? ['EFWEDD']).map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>

          <label className="muted">Project</label>
          <select
            value={activeId}
            onChange={(e) => {
              setProjectId(e.target.value)
              setTaskId('')
            }}
          >
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.code}
              </option>
            ))}
          </select>

          <label className="muted">Task</label>
          <select value={taskId} onChange={(e) => setTaskId(e.target.value)}>
            <option value="">Select a task…</option>
            {(tasks.data?.items ?? []).map((t) => (
              <option key={t.id} value={t.id}>
                {t.code}
              </option>
            ))}
          </select>

          <label className="muted">Commit mode</label>
          <select value={commitMode} onChange={(e) => setCommitMode(e.target.value)}>
            <option value="INSERT_ONLY">Insert only</option>
            <option value="INSERT_UPDATE">Insert and update</option>
            <option value="INSERT_MERGE">Insert and merge</option>
          </select>
        </div>

        <div className="row">
          <input
            type="file"
            accept=".xlsx,.csv,.tsv,.txt,.zip"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
          <button className="secondary" disabled={!file || busy} onClick={() => void submit(true)}>
            Validate only
          </button>
          <button disabled={!file || !activeId || !taskId || busy} onClick={() => void submit(false)}>
            {busy ? 'Working…' : 'Validate and load'}
          </button>
        </div>

        {error && <p className="error">{error}</p>}
      </div>

      {result && (
        <div className="panel">
          <h2>
            Result{' '}
            <span className={result.valid ? 'badge' : 'badge exceed'}>
              {result.valid ? 'valid' : 'invalid'}
            </span>
          </h2>
          <p className="muted">
            Sections: {(result.sections ?? []).join(', ') || '—'} · Errors: {result.errors} ·
            Warnings: {result.warnings}
          </p>
          {result.findings && result.findings.length > 0 && (
            <table>
              <thead>
                <tr>
                  <th>Severity</th>
                  <th>Section</th>
                  <th>Row</th>
                  <th>Field</th>
                  <th>Message</th>
                </tr>
              </thead>
              <tbody>
                {result.findings.map((f, i) => (
                  <tr key={i}>
                    <td className={f.severity === 'ERROR' ? 'danger' : 'warn'}>{f.severity}</td>
                    <td className="mono">{f.section}</td>
                    <td>{f.row || ''}</td>
                    <td className="mono">{f.field ?? ''}</td>
                    <td>{f.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </>
  )
}
