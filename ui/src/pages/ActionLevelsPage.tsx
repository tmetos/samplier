import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type ActionLevelGroup, type ActionLevelParameter } from '../lib/api'
import { useProjects } from '../hooks/useProjects'

interface GroupForm {
  type_code: string
  code: string
  name: string
  matrix: string
  fraction: string
  project_id: string
  start_date: string
  end_date: string
}

const emptyGroup: GroupForm = {
  type_code: 'EPA',
  code: '',
  name: '',
  matrix: '',
  fraction: '',
  project_id: '',
  start_date: '',
  end_date: '',
}

interface ParamForm {
  analyte_id: string
  action_level: string
  action_level_min: string
  unit: string
  fraction: string
  analytic_method: string
}

const emptyParam: ParamForm = {
  analyte_id: '',
  action_level: '',
  action_level_min: '',
  unit: 'mg/L',
  fraction: '',
  analytic_method: '',
}

export function ActionLevelsPage() {
  const { projects } = useProjects()
  const [type, setType] = useState('EPA')
  const [selectedId, setSelectedId] = useState('')
  const [editingGroupId, setEditingGroupId] = useState<string | null>(null)
  const [groupForm, setGroupForm] = useState<GroupForm>(emptyGroup)
  const [editingParamId, setEditingParamId] = useState<string | null>(null)
  const [paramForm, setParamForm] = useState<ParamForm>(emptyParam)
  const [error, setError] = useState('')

  // Type creation form.
  const [newTypeCode, setNewTypeCode] = useState('')
  const [newTypeName, setNewTypeName] = useState('')

  const groups = useQuery({
    queryKey: ['action-levels', type],
    queryFn: () => api.listActionLevels({ type }),
  })
  const types = useQuery({ queryKey: ['action-level-types'], queryFn: api.listActionLevelTypes })
  const analytes = useQuery({ queryKey: ['analytes'], queryFn: () => api.listAnalytes(1000) })
  const units = useQuery({ queryKey: ['units'], queryFn: api.listUnits })

  const items = groups.data?.items ?? []
  const selected = items.find((g) => g.id === selectedId) ?? items[0]

  function startGroup(g?: ActionLevelGroup) {
    setEditingGroupId(g?.id ?? null)
    setGroupForm(
      g
        ? {
            type_code: g.type_code ?? 'EPA',
            code: g.code,
            name: g.name,
            matrix: g.matrix ?? '',
            fraction: g.fraction ?? '',
            project_id: g.project_id ?? '',
            start_date: g.start_date ?? '',
            end_date: g.end_date ?? '',
          }
        : emptyGroup,
    )
    setError('')
  }

  async function saveGroup() {
    if (!groupForm.type_code || !groupForm.code.trim() || !groupForm.name.trim()) {
      setError('Type, code and name are required.')
      return
    }
    setError('')
    const payload = {
      type_code: groupForm.type_code,
      code: groupForm.code.trim(),
      name: groupForm.name.trim(),
      matrix: groupForm.matrix.trim() || undefined,
      fraction: groupForm.fraction.trim() || undefined,
      project_id: groupForm.project_id || undefined,
      start_date: groupForm.start_date || undefined,
      end_date: groupForm.end_date || undefined,
    }
    try {
      if (editingGroupId) {
        await api.updateActionLevelGroup(editingGroupId, payload)
      } else {
        await api.createActionLevelGroup(payload)
      }
      startGroup()
      await groups.refetch()
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    }
  }

  async function removeGroup(g: ActionLevelGroup) {
    if (!window.confirm(`Delete limit set “${g.code}”?`)) return
    try {
      await api.deleteActionLevelGroup(g.id)
      if (selectedId === g.id) setSelectedId('')
      await groups.refetch()
    } catch (e) {
      window.alert(String(e instanceof Error ? e.message : e))
    }
  }

  function startParam(p?: ActionLevelParameter) {
    setEditingParamId(p?.id ?? null)
    setParamForm(
      p
        ? {
            analyte_id: p.analyte_id,
            action_level: String(p.action_level),
            action_level_min: p.action_level_min != null ? String(p.action_level_min) : '',
            unit: p.unit,
            fraction: p.fraction ?? '',
            analytic_method: p.analytic_method ?? '',
          }
        : emptyParam,
    )
    setError('')
  }

  async function saveParam() {
    if (!selected || !paramForm.analyte_id || !paramForm.unit || paramForm.action_level === '') {
      setError('Analyte, limit and unit are required.')
      return
    }
    setError('')
    const payload = {
      analyte_id: paramForm.analyte_id,
      action_level: Number(paramForm.action_level),
      action_level_min: paramForm.action_level_min === '' ? undefined : Number(paramForm.action_level_min),
      unit: paramForm.unit,
      fraction: paramForm.fraction.trim() || undefined,
      analytic_method: paramForm.analytic_method.trim() || undefined,
    }
    try {
      if (editingParamId) {
        await api.updateActionLevelParameter(editingParamId, payload)
      } else {
        await api.createActionLevelParameter(selected.id, payload)
      }
      startParam()
      await groups.refetch()
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    }
  }

  async function removeParam(id: string) {
    if (!window.confirm('Delete this limit parameter?')) return
    try {
      await api.deleteActionLevelParameter(id)
      await groups.refetch()
    } catch (e) {
      window.alert(String(e instanceof Error ? e.message : e))
    }
  }

  async function addType() {
    if (!newTypeCode.trim() || !newTypeName.trim()) {
      setError('Type code and name are required.')
      return
    }
    setError('')
    try {
      await api.createActionLevelType({ code: newTypeCode.trim(), name: newTypeName.trim() })
      setNewTypeCode('')
      setNewTypeName('')
      await types.refetch()
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    }
  }

  return (
    <>
      <h1>Action Levels</h1>
      <p className="sub">
        Create, edit and delete the regulatory limit sets used for exceedance detection and reporting.
      </p>

      <div className="panel">
        <div className="row" style={{ marginBottom: 14 }}>
          <label className="muted">Type</label>
          <select value={type} onChange={(e) => setType(e.target.value)}>
            {(types.data?.items ?? []).map((t) => (
              <option key={t.code} value={t.code}>
                {t.code} — {t.name}
              </option>
            ))}
          </select>

          <div className="spacer" />
          <input value={newTypeCode} onChange={(e) => setNewTypeCode(e.target.value)} placeholder="New type code" />
          <input value={newTypeName} onChange={(e) => setNewTypeName(e.target.value)} placeholder="New type name" />
          <button className="secondary" onClick={addType}>
            Add type
          </button>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="panel-inner" style={{ marginBottom: 16 }}>
          <div className="row">
            <select
              value={groupForm.type_code}
              onChange={(e) => setGroupForm((f) => ({ ...f, type_code: e.target.value }))}
            >
              {(types.data?.items ?? []).map((t) => (
                <option key={t.code} value={t.code}>
                  {t.code}
                </option>
              ))}
            </select>
            <input
              value={groupForm.code}
              onChange={(e) => setGroupForm((f) => ({ ...f, code: e.target.value }))}
              placeholder="Code"
            />
            <input
              value={groupForm.name}
              onChange={(e) => setGroupForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="Name"
            />
            <input
              value={groupForm.matrix}
              onChange={(e) => setGroupForm((f) => ({ ...f, matrix: e.target.value }))}
              placeholder="Matrix"
            />
            <input
              value={groupForm.fraction}
              onChange={(e) => setGroupForm((f) => ({ ...f, fraction: e.target.value }))}
              placeholder="Fraction"
            />
            <select
              value={groupForm.project_id}
              onChange={(e) => setGroupForm((f) => ({ ...f, project_id: e.target.value }))}
            >
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.code}
                </option>
              ))}
            </select>
            <input
              type="date"
              value={groupForm.start_date}
              onChange={(e) => setGroupForm((f) => ({ ...f, start_date: e.target.value }))}
            />
            <input
              type="date"
              value={groupForm.end_date}
              onChange={(e) => setGroupForm((f) => ({ ...f, end_date: e.target.value }))}
            />
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button onClick={saveGroup}>{editingGroupId ? 'Update limit set' : 'Add limit set'}</button>
            {editingGroupId && (
              <button className="secondary" onClick={() => startGroup()}>
                Cancel
              </button>
            )}
          </div>
        </div>

        {items.length ? (
          items.map((g) => (
            <div key={g.id} style={{ marginBottom: 20 }}>
              <h2>
                <button className="linkish" onClick={() => setSelectedId(g.id)}>
                  {g.code}
                </button>{' '}
                · {g.name}{' '}
                <span className="badge">{g.type_code}</span>
                {g.matrix && <span className="badge">{g.matrix}</span>}
              </h2>
              <div className="row" style={{ marginBottom: 8 }}>
                <button className="secondary" onClick={() => startGroup(g)}>
                  Edit set
                </button>{' '}
                <button className="secondary" onClick={() => removeGroup(g)}>
                  Delete set
                </button>
              </div>
              <table>
                <thead>
                  <tr>
                    <th>Analyte</th>
                    <th>CAS RN</th>
                    <th>Limit</th>
                    <th>Min</th>
                    <th>Unit</th>
                    <th>Fraction</th>
                    <th />
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
                      <td>{p.fraction ?? ''}</td>
                      <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                        <button className="secondary" onClick={() => startParam(p)}>
                          Edit
                        </button>{' '}
                        <button className="secondary" onClick={() => removeParam(p.id)}>
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))
        ) : (
          <p className="muted">No limit sets for this type yet.</p>
        )}
      </div>

      {selected && (
        <div className="panel">
          <div className="section-head">
            <h2>{selected.code} — parameters</h2>
          </div>
          <div className="row">
            <select
              value={paramForm.analyte_id}
              onChange={(e) => setParamForm((f) => ({ ...f, analyte_id: e.target.value }))}
            >
              <option value="">Analyte…</option>
              {(analytes.data?.items ?? []).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.chemical_name} {a.cas_rn ? `· ${a.cas_rn}` : ''}
                </option>
              ))}
            </select>
            <input
              value={paramForm.action_level}
              onChange={(e) => setParamForm((f) => ({ ...f, action_level: e.target.value }))}
              placeholder="Limit"
              type="number"
              step="any"
            />
            <input
              value={paramForm.action_level_min}
              onChange={(e) => setParamForm((f) => ({ ...f, action_level_min: e.target.value }))}
              placeholder="Min (optional)"
              type="number"
              step="any"
            />
            <select value={paramForm.unit} onChange={(e) => setParamForm((f) => ({ ...f, unit: e.target.value }))}>
              {(units.data?.items ?? []).map((u) => (
                <option key={u.code} value={u.code}>
                  {u.code}
                </option>
              ))}
            </select>
            <input
              value={paramForm.fraction}
              onChange={(e) => setParamForm((f) => ({ ...f, fraction: e.target.value }))}
              placeholder="Fraction"
            />
            <input
              value={paramForm.analytic_method}
              onChange={(e) => setParamForm((f) => ({ ...f, analytic_method: e.target.value }))}
              placeholder="Method"
            />
          </div>
          <div className="row" style={{ marginTop: 10 }}>
            <button onClick={saveParam}>{editingParamId ? 'Update parameter' : 'Add parameter'}</button>
            {editingParamId && (
              <button className="secondary" onClick={() => startParam()}>
                Cancel
              </button>
            )}
          </div>
        </div>
      )}
    </>
  )
}
