import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Analyte, type AnalyteGroup } from '../lib/api'

type Tab = 'analytes' | 'groups'

export function ReferenceDataPage() {
  const [tab, setTab] = useState<Tab>('analytes')

  const analytes = useQuery({ queryKey: ['analytes'], queryFn: () => api.listAnalytes(1000) })
  const groups = useQuery({ queryKey: ['analyte-groups'], queryFn: api.listAnalyteGroups })

  return (
    <>
      <h1>Reference Data</h1>
      <p className="sub">
        Manage analytes and the analyte grouping sets used as crosstab tabs in custom reports.
      </p>

      <div className="tabs">
        <button className="tab" aria-selected={tab === 'analytes'} onClick={() => setTab('analytes')}>
          Analytes
        </button>
        <button className="tab" aria-selected={tab === 'groups'} onClick={() => setTab('groups')}>
          Analyte Groups
        </button>
      </div>

      {tab === 'analytes' ? (
        <AnalytesPanel data={analytes.data?.items ?? []} onChanged={() => analytes.refetch()} />
      ) : (
        <GroupsPanel
          groups={groups.data?.items ?? []}
          analytes={analytes.data?.items ?? []}
          onChanged={() => {
            groups.refetch()
            analytes.refetch()
          }}
        />
      )}
    </>
  )
}

function AnalytesPanel({ data, onChanged }: { data: Analyte[]; onChanged: () => void }) {
  const [editing, setEditing] = useState<Analyte | null>(null)
  const [name, setName] = useState('')
  const [cas, setCas] = useState('')
  const [shortName, setShortName] = useState('')
  const [aliases, setAliases] = useState('')
  const [error, setError] = useState('')

  function startEdit(a?: Analyte) {
    setEditing(a ?? null)
    setName(a?.chemical_name ?? '')
    setCas(a?.cas_rn ?? '')
    setShortName(a?.short_name ?? '')
    setAliases((a?.aliases ?? []).join(', '))
    setError('')
  }

  async function save() {
    if (!name.trim()) {
      setError('Chemical name is required.')
      return
    }
    setError('')
    const payload = {
      chemical_name: name.trim(),
      cas_rn: cas.trim() || undefined,
      short_name: shortName.trim() || undefined,
      aliases: aliases
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    }
    try {
      if (editing) {
        await api.updateAnalyte(editing.id, payload)
      } else {
        await api.createAnalyte(payload)
      }
      startEdit()
      onChanged()
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    }
  }

  async function remove(a: Analyte) {
    if (!window.confirm(`Delete analyte “${a.chemical_name}”?`)) return
    try {
      await api.deleteAnalyte(a.id)
      onChanged()
    } catch (e) {
      window.alert(String(e instanceof Error ? e.message : e))
    }
  }

  return (
    <div className="panel">
      <div className="section-head">
        <h2>Analytes</h2>
        <span className="count">{data.length}</span>
      </div>

      <div className="panel-inner" style={{ marginBottom: 16 }}>
        <div className="row">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Chemical name" />
          <input value={cas} onChange={(e) => setCas(e.target.value)} placeholder="CAS RN" />
          <input value={shortName} onChange={(e) => setShortName(e.target.value)} placeholder="Short name" />
          <input value={aliases} onChange={(e) => setAliases(e.target.value)} placeholder="Aliases (comma separated)" style={{ flex: 2 }} />
        </div>
        {error && <p className="error">{error}</p>}
        <div className="row" style={{ marginTop: 10 }}>
          <button onClick={save}>{editing ? 'Update analyte' : 'Add analyte'}</button>
          {editing && (
            <button className="secondary" onClick={() => startEdit()}>
              Cancel
            </button>
          )}
        </div>
      </div>

      <table>
        <thead>
          <tr>
            <th>Analyte</th>
            <th>CAS RN</th>
            <th>Short name</th>
            <th>Aliases</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {data.map((a) => (
            <tr key={a.id}>
              <td>{a.chemical_name}</td>
              <td className="mono">{a.cas_rn ?? ''}</td>
              <td>{a.short_name ?? ''}</td>
              <td className="muted">{a.aliases.join(', ')}</td>
              <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                <button className="secondary" onClick={() => startEdit(a)}>
                  Edit
                </button>{' '}
                <button className="secondary" onClick={() => remove(a)}>
                  Delete
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function GroupsPanel({
  groups,
  analytes,
  onChanged,
}: {
  groups: AnalyteGroup[]
  analytes: Analyte[]
  onChanged: () => void
}) {
  const [editing, setEditing] = useState<AnalyteGroup | null>(null)
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')

  const [selectedId, setSelectedId] = useState('')
  const [memberAnalyteId, setMemberAnalyteId] = useState('')
  const [memberMatrix, setMemberMatrix] = useState('')
  const [memberFraction, setMemberFraction] = useState('')
  const [memberUnits, setMemberUnits] = useState('')

  const selected = groups.find((g) => g.id === selectedId) ?? groups[0]

  function startEdit(g?: AnalyteGroup) {
    setEditing(g ?? null)
    setCode(g?.code ?? '')
    setName(g?.name ?? '')
    setDescription(g?.description ?? '')
    setError('')
  }

  async function saveGroup() {
    if (!code.trim() || !name.trim()) {
      setError('Code and name are required.')
      return
    }
    setError('')
    const payload = { code: code.trim(), name: name.trim(), description: description.trim() || undefined }
    try {
      if (editing) {
        await api.updateAnalyteGroup(editing.id, payload)
      } else {
        await api.createAnalyteGroup(payload)
      }
      startEdit()
      onChanged()
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    }
  }

  async function removeGroup(g: AnalyteGroup) {
    if (!window.confirm(`Delete analyte group “${g.name}”?`)) return
    try {
      await api.deleteAnalyteGroup(g.id)
      if (selectedId === g.id) setSelectedId('')
      onChanged()
    } catch (e) {
      window.alert(String(e instanceof Error ? e.message : e))
    }
  }

  async function addMember() {
    if (!selected || !memberAnalyteId) {
      setError('Pick a group and an analyte.')
      return
    }
    setError('')
    try {
      await api.addAnalyteGroupMember(selected.id, {
        analyte_id: memberAnalyteId,
        matrix: memberMatrix || undefined,
        fraction: memberFraction || undefined,
        default_units: memberUnits || undefined,
      })
      setMemberAnalyteId('')
      setMemberMatrix('')
      setMemberFraction('')
      setMemberUnits('')
      onChanged()
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e))
    }
  }

  async function removeMember(groupId: string, memberId: string) {
    if (!window.confirm('Remove this analyte from the group?')) return
    try {
      await api.deleteAnalyteGroupMember(groupId, memberId)
      onChanged()
    } catch (e) {
      window.alert(String(e instanceof Error ? e.message : e))
    }
  }

  return (
    <>
      <div className="panel">
        <div className="section-head">
          <h2>Analyte grouping sets</h2>
          <span className="count">{groups.length}</span>
        </div>

        <div className="panel-inner" style={{ marginBottom: 16 }}>
          <div className="row">
            <input value={code} onChange={(e) => setCode(e.target.value)} placeholder="Code (e.g. VOCs)" />
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" />
            <input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Description" style={{ flex: 2 }} />
          </div>
          {error && <p className="error">{error}</p>}
          <div className="row" style={{ marginTop: 10 }}>
            <button onClick={saveGroup}>{editing ? 'Update group' : 'Add group'}</button>
            {editing && (
              <button className="secondary" onClick={() => startEdit()}>
                Cancel
              </button>
            )}
          </div>
        </div>

        <table>
          <thead>
            <tr>
              <th>Group</th>
              <th>Code</th>
              <th>Analytes</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {groups.map((g) => (
              <tr key={g.id}>
                <td>
                  <button className="linkish" onClick={() => setSelectedId(g.id)}>
                    {g.name}
                  </button>
                  {g.description && <div className="muted" style={{ fontSize: 11 }}>{g.description}</div>}
                </td>
                <td className="mono">{g.code}</td>
                <td className="muted">{g.analytes?.length ?? 0}</td>
                <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                  <button className="secondary" onClick={() => startEdit(g)}>
                    Edit
                  </button>{' '}
                  <button className="secondary" onClick={() => removeGroup(g)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {selected && (
        <div className="panel">
          <div className="section-head">
            <h2>{selected.name} — members</h2>
          </div>

          <div className="row">
            <select value={memberAnalyteId} onChange={(e) => setMemberAnalyteId(e.target.value)}>
              <option value="">Add analyte…</option>
              {analytes.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.chemical_name} {a.cas_rn ? `· ${a.cas_rn}` : ''}
                </option>
              ))}
            </select>
            <input value={memberMatrix} onChange={(e) => setMemberMatrix(e.target.value)} placeholder="Matrix (optional)" />
            <input value={memberFraction} onChange={(e) => setMemberFraction(e.target.value)} placeholder="Fraction (optional)" />
            <input value={memberUnits} onChange={(e) => setMemberUnits(e.target.value)} placeholder="Default units (optional)" />
            <button className="secondary" onClick={addMember}>
              Add
            </button>
          </div>

          <table style={{ marginTop: 12 }}>
            <thead>
              <tr>
                <th>Analyte</th>
                <th>CAS RN</th>
                <th>Matrix</th>
                <th>Fraction</th>
                <th>Default units</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(selected.analytes ?? []).map((m) => (
                <tr key={m.id}>
                  <td>{m.analyte_name ?? m.analyte_id}</td>
                  <td className="mono">{m.cas_rn ?? ''}</td>
                  <td>{m.matrix ?? ''}</td>
                  <td>{m.fraction ?? ''}</td>
                  <td>{m.default_units ?? ''}</td>
                  <td style={{ textAlign: 'right' }}>
                    <button className="secondary" onClick={() => removeMember(selected.id, m.id)}>
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
              {!selected.analytes?.length && (
                <tr>
                  <td colSpan={6} className="muted">
                    No analytes in this group yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}
