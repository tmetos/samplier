import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type ReportPreview, type ReportSpec, type ReportView } from '../lib/api'
import { useProjects } from '../hooks/useProjects'

function toggle<T>(list: T[], value: T): T[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value]
}

export function ReportsPage() {
  const { projects } = useProjects()
  const [projectId, setProjectId] = useState('')
  const [groupIds, setGroupIds] = useState<string[]>([])
  const [analyteIds, setAnalyteIds] = useState<string[]>([])
  const [limitGroupIds, setLimitGroupIds] = useState<string[]>([])
  const [title, setTitle] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [locationFilter, setLocationFilter] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [includeNonDetects, setIncludeNonDetects] = useState(true)
  const [detectsOnly, setDetectsOnly] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)

  const [preview, setPreview] = useState<ReportPreview | null>(null)
  const [busy, setBusy] = useState<'preview' | 'export' | 'save' | null>(null)
  const [error, setError] = useState('')

  const groups = useQuery({ queryKey: ['analyte-groups'], queryFn: api.listAnalyteGroups })
  const analytes = useQuery({ queryKey: ['analytes'], queryFn: () => api.listAnalytes(1000) })
  const limits = useQuery({ queryKey: ['action-levels', 'all'], queryFn: () => api.listActionLevels({}) })
  const saved = useQuery({ queryKey: ['report-views'], queryFn: api.listReportViews })

  // Deep-link support: /reports?view=<id> loads and previews a saved view.
  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get('view')
    if (!id || !saved.data?.items) return
    const v = saved.data.items.find((x) => x.id === id)
    if (!v) return
    loadView(v)
    runPreview(specFromView(v))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [saved.data])

  function buildSpec(): ReportSpec {
    return {
      title: title || undefined,
      project_id: projectId || undefined,
      analyte_group_ids: groupIds,
      analyte_ids: analyteIds,
      action_level_group_ids: limitGroupIds,
      include_non_detects: includeNonDetects,
      detects_only: detectsOnly,
      location_filter: locationFilter || undefined,
      start_date: startDate || undefined,
      end_date: endDate || undefined,
    }
  }

  function specFromView(v: ReportView): ReportSpec {
    return {
      title: v.title || undefined,
      project_id: v.project_id || undefined,
      analyte_group_ids: v.analyte_group_ids ?? [],
      analyte_ids: v.analyte_ids ?? [],
      action_level_group_ids: v.action_level_group_ids ?? [],
      include_non_detects: v.include_non_detects,
      detects_only: v.detects_only,
      location_filter: v.location_filter || undefined,
      start_date: v.start_date || undefined,
      end_date: v.end_date || undefined,
    }
  }

  async function runPreview(specOverride?: ReportSpec) {
    setError('')
    setBusy('preview')
    try {
      setPreview(await api.previewReport(specOverride ?? buildSpec()))
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    } finally {
      setBusy(null)
    }
  }

  async function runExport(specOverride?: ReportSpec) {
    setError('')
    setBusy('export')
    try {
      await api.exportReport(specOverride ?? buildSpec())
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    } finally {
      setBusy(null)
    }
  }

  async function saveView() {
    if (!name.trim()) {
      setError('Enter a name to save this view.')
      return
    }
    setError('')
    setBusy('save')
    const payload: ReportSpec = { ...buildSpec(), name: name.trim(), description: description || undefined }
    try {
      if (editingId) {
        await api.updateReportView(editingId, payload)
      } else {
        await api.saveReportView(payload)
      }
      await saved.refetch()
      setEditingId(null)
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    } finally {
      setBusy(null)
    }
  }

  function loadView(v: ReportView) {
    setEditingId(v.id)
    setName(v.name ?? '')
    setDescription(v.description ?? '')
    setTitle(v.title ?? '')
    setProjectId(v.project_id ?? '')
    setGroupIds(v.analyte_group_ids ?? [])
    setAnalyteIds(v.analyte_ids ?? [])
    setLimitGroupIds(v.action_level_group_ids ?? [])
    setIncludeNonDetects(v.include_non_detects ?? true)
    setDetectsOnly(v.detects_only ?? false)
    setLocationFilter(v.location_filter ?? '')
    setStartDate(v.start_date ?? '')
    setEndDate(v.end_date ?? '')
    setPreview(null)
    setError('')
  }

  async function deleteView(id: string) {
    if (!window.confirm('Delete this saved report view?')) return
    await api.deleteReportView(id)
    await saved.refetch()
    if (editingId === id) setEditingId(null)
  }

  const activeId = projectId || projects[0]?.id || ''

  return (
    <>
      <h1>Custom Reports</h1>
      <p className="sub">
        Build a crosstab from analytes and one or more regulatory limit sets. Each analyte
        grouping set becomes its own tab in the preview and the Excel workbook.
      </p>

      <div className="report-layout">
        <div className="report-side">
          <div className="panel">
            <div className="section-head">
              <h2>Report builder</h2>
            </div>

            <div className="field">
              <label className="muted">Project</label>
              <select value={activeId} onChange={(e) => setProjectId(e.target.value)}>
                <option value="">All projects</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.code} — {p.name}
                  </option>
                ))}
              </select>
            </div>

            <div className="field">
              <label className="muted">
                Analyte grouping sets <span className="hint">(one tab each)</span>
              </label>
              <div className="check-list">
                {groups.data?.items?.map((g) => (
                  <label key={g.id} className="check">
                    <input
                      type="checkbox"
                      checked={groupIds.includes(g.id)}
                      onChange={() => setGroupIds((xs) => toggle(xs, g.id))}
                    />
                    <span>
                      {g.name} <span className="muted">({g.analytes?.length ?? 0})</span>
                    </span>
                  </label>
                ))}
              </div>
            </div>

            <div className="field">
              <label className="muted">
                Individual analytes <span className="hint">(becomes a “Custom” tab)</span>
              </label>
              <div className="check-list tall">
                {analytes.data?.items?.map((a) => (
                  <label key={a.id} className="check">
                    <input
                      type="checkbox"
                      checked={analyteIds.includes(a.id)}
                      onChange={() => setAnalyteIds((xs) => toggle(xs, a.id))}
                    />
                    <span>
                      {a.chemical_name}{' '}
                      <span className="muted">{a.cas_rn ? `· ${a.cas_rn}` : ''}</span>
                    </span>
                  </label>
                ))}
              </div>
            </div>

            <div className="field">
              <label className="muted">Regulatory limit sets</label>
              <div className="check-list">
                {limits.data?.items?.map((g) => (
                  <label key={g.id} className="check">
                    <input
                      type="checkbox"
                      checked={limitGroupIds.includes(g.id)}
                      onChange={() => setLimitGroupIds((xs) => toggle(xs, g.id))}
                    />
                    <span>
                      {g.code} <span className="muted">({g.type_code})</span>
                    </span>
                  </label>
                ))}
              </div>
            </div>

            <div className="field">
              <label className="muted">Title</label>
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="e.g. Q1 2024 Groundwater vs EPA & State"
              />
            </div>

            <div className="field">
              <label className="muted">Location filter</label>
              <input
                value={locationFilter}
                onChange={(e) => setLocationFilter(e.target.value)}
                placeholder="e.g. MW-02"
              />
            </div>

            <div className="row">
              <label className="muted">From</label>
              <input type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)} />
              <label className="muted">To</label>
              <input type="date" value={endDate} onChange={(e) => setEndDate(e.target.value)} />
            </div>

            <div className="row" style={{ marginTop: 10 }}>
              <label className="check">
                <input
                  type="checkbox"
                  checked={includeNonDetects}
                  onChange={(e) => setIncludeNonDetects(e.target.checked)}
                />
                <span>Include non-detects</span>
              </label>
              <label className="check">
                <input
                  type="checkbox"
                  checked={detectsOnly}
                  onChange={(e) => setDetectsOnly(e.target.checked)}
                />
                <span>Detects only</span>
              </label>
            </div>

            {error && <p className="error">{error}</p>}

            <div className="row" style={{ marginTop: 14 }}>
              <button onClick={() => runPreview()} disabled={busy !== null || !(groupIds.length || analyteIds.length)}>
                {busy === 'preview' ? 'Previewing…' : 'Preview'}
              </button>
              <button
                className="secondary"
                onClick={() => runExport()}
                disabled={busy !== null || !(groupIds.length || analyteIds.length)}
              >
                {busy === 'export' ? 'Exporting…' : 'Export XLSX'}
              </button>
            </div>
          </div>

          <div className="panel">
            <div className="section-head">
              <h2>Saved views</h2>
            </div>

            <div className="field">
              <label className="muted">View name</label>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name to save under" />
              <input
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Description (optional)"
                style={{ marginTop: 6 }}
              />
            </div>
            <div className="row" style={{ marginTop: 10 }}>
              <button onClick={saveView} disabled={busy !== null || !name.trim()}>
                {busy === 'save' ? 'Saving…' : editingId ? 'Update view' : 'Save view'}
              </button>
              {editingId && (
                <button className="secondary" onClick={() => setEditingId(null)}>
                  New view
                </button>
              )}
            </div>

            {saved.data?.items?.length ? (
              <table style={{ marginTop: 14 }}>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {saved.data.items.map((v) => (
                    <tr key={v.id}>
                      <td>
                        <button className="linkish" onClick={() => loadView(v)}>
                          {v.name}
                        </button>
                        <div className="muted" style={{ fontSize: 11 }}>
                          {v.updated_at ? v.updated_at.slice(0, 10) : ''}
                          {v.analyte_group_ids?.length ? ` · ${v.analyte_group_ids.length} tab(s)` : ''}
                        </div>
                      </td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                        <button className="secondary" onClick={() => loadView(v)}>
                          Load
                        </button>{' '}
                        <button
                          className="secondary"
                          onClick={() => runPreview(specFromView(v))}
                        >
                          Preview
                        </button>{' '}
                        <button className="secondary" onClick={() => deleteView(v.id)}>
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="muted">No saved views yet.</p>
            )}
          </div>
        </div>

        <div className="report-main">
          <div className="panel">
            <div className="section-head">
              <h2>Preview</h2>
              {preview && (
                <>
                  <span className="count">
                    {preview.project_name ?? 'All projects'} · {preview.action_level_groups.join(', ') || 'no limits'}
                  </span>
                  <span className={preview.exceedance_count ? 'badge exceed' : 'badge'}>
                    {preview.exceedance_count} exceedance{preview.exceedance_count === 1 ? '' : 's'}
                  </span>
                </>
              )}
            </div>

            {!preview && <p className="muted">Select analytes or grouping sets, then press Preview.</p>}

            {preview?.tabs.map((tab) => (
              <div key={tab.key} className="preview-tab">
                <h3>
                  {tab.label}{' '}
                  <span className="badge">{tab.analyte_count} analytes</span>{' '}
                  {tab.exceedance_count > 0 && (
                    <span className="badge exceed">{tab.exceedance_count} exceed</span>
                  )}
                </h3>
                <div className="preview-scroll">
                  <table className="crosstab">
                    <thead>
                      <tr>
                        <th>Analyte</th>
                        <th>Unit</th>
                        {tab.sample_columns.map((sc, i) => (
                          <th key={i} title={sc.sample}>
                            <div>{sc.location}</div>
                            <div className="muted" style={{ fontWeight: 400 }}>{sc.date}</div>
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {tab.rows.map((row) => (
                        <tr key={row.analyte}>
                          <td>
                            {row.analyte}
                            {row.cas_rn && <div className="muted" style={{ fontSize: 11 }}>{row.cas_rn}</div>}
                          </td>
                          <td className="muted">{row.unit}</td>
                          {row.cells.map((cell, i) => (
                            <td key={i} className={cell.exceeded ? 'exceed' : cell.non_detect ? 'muted' : ''}>
                              {cell.value || '—'}
                              {cell.flags && (
                                <div className={`cell-flag ${cell.exceeded ? 'cell-flag-exceed' : ''}`}>
                                  {cell.flags}
                                </div>
                              )}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </>
  )
}
