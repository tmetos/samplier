import { useEffect, useRef } from 'react'
import maplibregl, { type Map as MapLibreMap } from 'maplibre-gl'
import { STYLE_URL, boundsOfGeometry } from '../lib/mapStyle'

export interface GeoFeature {
  type: 'Feature'
  id?: string
  geometry: { type: string; coordinates: unknown } | null
  properties: Record<string, unknown>
}

/**
 * A small, non-interactive map that frames a project's boundary polygon over
 * the same self-hosted basemap as the main map.
 */
export function BoundaryMap({
  feature,
  height = 170,
  color = '#8fb3d9',
}: {
  feature: GeoFeature | null
  height?: number
  color?: string
}) {
  const container = useRef<HTMLDivElement>(null)
  const map = useRef<MapLibreMap | null>(null)

  useEffect(() => {
    if (!container.current) return
    try {
      map.current = new maplibregl.Map({
        container: container.current,
        style: STYLE_URL,
        center: [-122.67, 45.52],
        zoom: 12,
        interactive: false,
        attributionControl: false,
      })
    } catch {
      return
    }
    return () => {
      map.current?.remove()
      map.current = null
    }
  }, [])

  useEffect(() => {
    const m = map.current
    if (!m || !feature) return

    const draw = () => {
      // Show only the boundary here; the main map is the multi-layer view.
      for (const layer of m.getStyle().layers ?? []) {
        if (layer.id.startsWith('samplier-')) {
          m.setLayoutProperty(layer.id, 'visibility', 'none')
        }
      }

      const data = {
        type: 'FeatureCollection',
        features: [feature],
      } as unknown as GeoJSON.FeatureCollection

      const existing = m.getSource('boundary') as maplibregl.GeoJSONSource | undefined
      if (existing) {
        existing.setData(data)
      } else {
        m.addSource('boundary', { type: 'geojson', data })
        m.addLayer({
          id: 'boundary-fill',
          type: 'fill',
          source: 'boundary',
          paint: { 'fill-color': color, 'fill-opacity': 0.22 },
        })
        m.addLayer({
          id: 'boundary-line',
          type: 'line',
          source: 'boundary',
          paint: { 'line-color': color, 'line-width': 2 },
        })
      }

      const bounds = boundsOfGeometry(feature.geometry)
      if (bounds) m.fitBounds(bounds, { padding: 18, maxZoom: 16, duration: 0 })
    }

    if (m.loaded()) draw()
    else m.once('load', draw)
  }, [feature, color])

  return (
    <div
      ref={container}
      style={{
        height,
        borderRadius: 8,
        overflow: 'hidden',
        border: '1px solid var(--border)',
        background: 'var(--panel-2)',
      }}
      aria-label="Project boundary"
    />
  )
}
