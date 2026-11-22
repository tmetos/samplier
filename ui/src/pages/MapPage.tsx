import { useEffect, useRef, useState } from 'react'
import maplibregl, { type Map as MapLibreMap } from 'maplibre-gl'
import { ogcCollection } from '../lib/api'
import { STYLE_URL, graticule } from '../lib/mapStyle'

/**
 * The basemap is fully self-hosted: the API serves a MapLibre style document
 * plus embedded Natural Earth land/boundary GeoJSON, so the browser makes no
 * third-party tile requests and needs no API key.
 */

interface CollectionSpec {
  id: string
  label: string
  color: string
  /** Base layer id in the style (fill/outline/circle derive from it). */
  layer: string
  hint: string
  defaultOn: boolean
}

const COLLECTIONS: CollectionSpec[] = [
  {
    id: 'projects',
    label: 'Project boundaries',
    color: '#7d97b8',
    layer: 'samplier-projects',
    hint: 'Site areas (polygon or point)',
    defaultOn: true,
  },
  {
    id: 'tasks',
    label: 'Sampling tasks',
    color: '#e0a952',
    layer: 'samplier-tasks',
    hint: 'Field events and extents',
    defaultOn: false,
  },
  {
    id: 'locations',
    label: 'Locations',
    color: '#35b37e',
    layer: 'samplier-locations',
    hint: 'Wells, borings, stations',
    defaultOn: true,
  },
  {
    id: 'samples',
    label: 'Samples',
    color: '#6fa8dc',
    layer: 'samplier-samples',
    hint: 'Collected samples (rings)',
    defaultOn: true,
  },
]

/**
 * Longitude/latitude extent of features, padded slightly. Handles points and
 * polygon rings so a site boundary frames the view. Returns null when there is
 * nothing to frame.
 */
function extentOf(
  features: { geometry: { type: string; coordinates: unknown } | null }[],
): [[number, number], [number, number]] | null {
  let minLon = Infinity
  let minLat = Infinity
  let maxLon = -Infinity
  let maxLat = -Infinity
  let seen = false

  const visit = (coords: unknown) => {
    if (!Array.isArray(coords)) return
    if (typeof coords[0] === 'number' && typeof coords[1] === 'number') {
      const [lon, lat] = coords as [number, number]
      minLon = Math.min(minLon, lon)
      maxLon = Math.max(maxLon, lon)
      minLat = Math.min(minLat, lat)
      maxLat = Math.max(maxLat, lat)
      seen = true
      return
    }
    for (const c of coords) visit(c)
  }

  for (const f of features) {
    if (f.geometry) visit(f.geometry.coordinates)
  }
  if (!seen) return null

  const padLon = Math.max((maxLon - minLon) * 0.15, 0.002)
  const padLat = Math.max((maxLat - minLat) * 0.15, 0.002)
  return [
    [minLon - padLon, minLat - padLat],
    [maxLon + padLon, maxLat + padLat],
  ]
}

export function MapPage() {
  const container = useRef<HTMLDivElement>(null)
  const map = useRef<MapLibreMap | null>(null)
  const [ready, setReady] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [bounds, setBounds] = useState<[[number, number], [number, number]] | null>(null)
  const [visible, setVisible] = useState<Record<string, boolean>>(
    Object.fromEntries(COLLECTIONS.map((c) => [c.id, c.defaultOn])),
  )

  // Initialise the map from the self-hosted style.
  useEffect(() => {
    if (!container.current || map.current) return

    let m: MapLibreMap
    try {
      m = new maplibregl.Map({
        container: container.current,
        style: STYLE_URL,
        center: [-120.5, 47.4],
        zoom: 5,
        attributionControl: { compact: true },
      })
    } catch (e) {
      // WebGL can be unavailable (headless browsers, locked-down desktops).
      setError(
        e instanceof Error && e.message.toLowerCase().includes('webgl')
          ? 'This browser could not start WebGL, so the map cannot render. The data is still available via the OGC API and the tables.'
          : String(e),
      )
      return
    }
    map.current = m
    m.addControl(new maplibregl.NavigationControl(), 'top-right')
    m.addControl(new maplibregl.ScaleControl({ unit: 'imperial' }), 'bottom-left')

    m.on('load', () => setReady(true))
    m.on('error', (e) => {
      const msg = e?.error?.message
      if (msg && !msg.includes('sprite') && !msg.includes('glyphs')) setError(msg)
    })

    return () => {
      m.remove()
      map.current = null
    }
  }, [])

  // Counts and the extent used for the initial view.
  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const [countsByCollection, projects, locations] = await Promise.all([
          Promise.all(
            COLLECTIONS.map(async (c) => {
              const fc = await ogcCollection(c.id, { limit: 1 })
              return [c.id, fc.numberMatched] as const
            }),
          ),
          ogcCollection('projects', { limit: 500 }),
          ogcCollection('locations', { limit: 2000 }),
        ])
        if (cancelled) return
        setCounts(Object.fromEntries(countsByCollection))
        // Prefer the project boundary for framing; fall back to locations.
        setBounds(extentOf(projects.features) ?? extentOf(locations.features))
      } catch (e) {
        if (!cancelled) setError(String(e))
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  // Frame the data once both the style and the extent are ready.
  useEffect(() => {
    const m = map.current
    if (!m || !ready || !bounds) return
    const [[minLon, minLat], [maxLon, maxLat]] = bounds
    if (Math.abs(maxLon - minLon) < 0.01 && Math.abs(maxLat - minLat) < 0.01) {
      m.easeTo({ center: [(minLon + maxLon) / 2, (minLat + maxLat) / 2], zoom: 12, duration: 0 })
      return
    }
    m.fitBounds(bounds, { padding: 70, maxZoom: 13, duration: 0 })
  }, [ready, bounds])

  // Point popups, attached once the style layers exist.
  useEffect(() => {
    const m = map.current
    if (!m || !ready) return

    const attached: Array<[string, (ev: maplibregl.MapLayerMouseEvent) => void]> = []

    for (const c of COLLECTIONS) {
      // Entities can be areas (fill/outline) or points (circle).
      const hitLayers = [c.layer, `${c.layer}-fill`, `${c.layer}-outline`].filter((id) =>
        m.getLayer(id),
      )
      for (const layerId of hitLayers) {
        const onClick = (ev: maplibregl.MapLayerMouseEvent) => {
          const f = ev.features?.[0]
          if (!f) return
          const props = (f.properties ?? {}) as Record<string, unknown>
          const rows = Object.entries(props)
            .slice(0, 8)
            .map(([k, v]) => `<div><span class="popup-key">${k}</span> ${String(v ?? '')}</div>`)
            .join('')
          new maplibregl.Popup({ closeButton: true })
            .setLngLat(ev.lngLat)
            .setHTML(`<div class="popup-title">${c.label}</div>${rows}`)
            .addTo(m)
        }
        m.on('click', layerId, onClick)
        attached.push([layerId, onClick])
      }
    }

    const onEnter = () => {
      m.getCanvas().style.cursor = 'pointer'
    }
    const onLeave = () => {
      m.getCanvas().style.cursor = ''
    }
    for (const c of COLLECTIONS) {
      for (const layerId of [c.layer, `${c.layer}-fill`]) {
        if (m.getLayer(layerId)) {
          m.on('mouseenter', layerId, onEnter)
          m.on('mouseleave', layerId, onLeave)
        }
      }
    }

    return () => {
      for (const [layerId, onClick] of attached) m.off('click', layerId, onClick)
    }
  }, [ready])

  // Keep a graticule in sync with the zoom level.
  useEffect(() => {
    const m = map.current
    if (!m || !ready) return

    const sync = () => {
      const data = graticule(m.getZoom())
      const src = m.getSource('graticule') as maplibregl.GeoJSONSource | undefined
      if (src) {
        src.setData(data as unknown as GeoJSON.FeatureCollection)
        return
      }
      m.addSource('graticule', {
        type: 'geojson',
        data: data as unknown as GeoJSON.FeatureCollection,
      })
      const firstFeatureLayer = m
        .getStyle()
        .layers?.find((l) => l.id.startsWith('samplier-'))?.id
      m.addLayer(
        {
          id: 'graticule',
          type: 'line',
          source: 'graticule',
          paint: { 'line-color': '#2b3d46', 'line-width': 0.5, 'line-opacity': 0.7 },
        },
        firstFeatureLayer,
      )
    }

    sync()
    m.on('moveend', sync)
    return () => {
      m.off('moveend', sync)
    }
  }, [ready])

  // Toggle visibility of every sub-layer belonging to a collection.
  useEffect(() => {
    const m = map.current
    if (!m || !ready) return
    for (const c of COLLECTIONS) {
      const ids = [
        c.layer,
        `${c.layer}-halo`,
        `${c.layer}-glow`,
        `${c.layer}-fill`,
        `${c.layer}-outline`,
      ]
      for (const id of ids) {
        if (m.getLayer(id)) {
          m.setLayoutProperty(id, 'visibility', visible[c.id] ? 'visible' : 'none')
        }
      }
    }
  }, [visible, ready])

  const toggle = (id: string) =>
    setVisible((prev) => ({ ...prev, [id]: !prev[id] }))

  return (
    <>
      <h1>Map</h1>
      <p className="sub">
        Self-hosted MapLibre basemap with Samplier features from the OGC API — no external tile
        service or API key.
      </p>

      <div className="panel">
        <div className="row" style={{ marginBottom: 4 }}>
          <div className="chips">
            {COLLECTIONS.map((c) => (
              <button
                key={c.id}
                type="button"
                className="chip"
                aria-pressed={visible[c.id]}
                onClick={() => toggle(c.id)}
                title={c.hint}
              >
                <span className="chip-dot" style={{ background: c.color, color: c.color }} />
                {c.label}
                <span className="chip-count">{counts[c.id] ?? '—'}</span>
              </button>
            ))}
          </div>
          <div className="spacer" />
          {!ready && <span className="muted">Loading map…</span>}
          <a className="button secondary" href={STYLE_URL} target="_blank" rel="noreferrer">
            Style JSON
          </a>
        </div>

        <div className="legend">
          {COLLECTIONS.map((c) => (
            <span key={c.id}>
              <i style={{ background: c.color }} />
              {c.label} — {c.hint}
            </span>
          ))}
        </div>

        {error && <p className="error">Map error: {error}</p>}
        <div className="map" ref={container} style={{ marginTop: 12 }} />
      </div>
    </>
  )
}
