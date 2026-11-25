/** Shared MapLibre style URL: served by the API, no external tiles. */
export const STYLE_URL = '/ogc/basemap/style.json'

/** Style metadata value that marks the embedded Natural Earth basemap. */
export const BASEMAP_NATURAL_EARTH = 'natural-earth-110m'

/**
 * The embedded Natural Earth basemap has no detail at site scale, so it needs
 * an adaptive graticule for geographic context. The OpenMapTiles basemap
 * carries its own roads, landuse and labels and needs none.
 */
export function needsGraticule(metadata: Record<string, unknown> | undefined): boolean {
  return metadata?.['samplier:basemap'] === BASEMAP_NATURAL_EARTH
}

/**
 * Build a lat/lon graticule sized to the current zoom. Natural Earth at 1:110m
 * has no detail at site scale, so these reference lines give the map geographic
 * context when zoomed in.
 */
export function graticule(zoom: number) {
  const step = zoom >= 12 ? 0.005 : zoom >= 10 ? 0.02 : zoom >= 7 ? 0.1 : zoom >= 4 ? 0.5 : 2
  const features: unknown[] = []
  for (let lon = -180; lon <= 180; lon += step) {
    features.push({
      type: 'Feature',
      properties: {},
      geometry: { type: 'LineString', coordinates: [[lon, -85], [lon, 85]] },
    })
  }
  for (let lat = -80; lat <= 80; lat += step) {
    features.push({
      type: 'Feature',
      properties: {},
      geometry: { type: 'LineString', coordinates: [[-180, lat], [180, lat]] },
    })
  }
  return { type: 'FeatureCollection' as const, features }
}

/** Geographic extent of any GeoJSON geometry (points, lines or polygons). */
export function boundsOfGeometry(geometry: {
  type: string
  coordinates: unknown
} | null): [[number, number], [number, number]] | null {
  if (!geometry) return null
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

  visit(geometry.coordinates)
  if (!seen) return null
  const padLon = Math.max((maxLon - minLon) * 0.12, 0.0008)
  const padLat = Math.max((maxLat - minLat) * 0.12, 0.0008)
  return [
    [minLon - padLon, minLat - padLat],
    [maxLon + padLon, maxLat + padLat],
  ]
}
