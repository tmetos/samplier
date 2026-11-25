import { describe, expect, it } from 'vitest'
import { boundsOfGeometry, graticule, needsGraticule } from './mapStyle'

describe('needsGraticule', () => {
  it('is true for the embedded Natural Earth basemap', () => {
    expect(needsGraticule({ 'samplier:basemap': 'natural-earth-110m' })).toBe(true)
  })

  it('is false for OpenMapTiles', () => {
    expect(needsGraticule({ 'samplier:basemap': 'openmaptiles' })).toBe(false)
  })

  it('is false when metadata is missing', () => {
    expect(needsGraticule(undefined)).toBe(false)
    expect(needsGraticule({})).toBe(false)
  })
})

describe('graticule', () => {
  it('builds meridian and parallel lines at the requested spacing', () => {
    const fc = graticule(5)
    // 0.5° step: 721 meridians (-180..180) and 321 parallels (-80..80).
    expect(fc.type).toBe('FeatureCollection')
    const lines = fc.features as Array<{
      geometry: { coordinates: [[number, number], [number, number]] }
    }>
    const meridians = lines.filter(
      (f) => f.geometry.coordinates[0][0] === f.geometry.coordinates[1][0],
    )
    expect(meridians.length).toBe(721)
    expect(fc.features.length).toBe(721 + 321)
  })

  it('tightens the spacing as the map zooms in', () => {
    const coarse = graticule(3).features.length
    const city = graticule(12).features.length
    expect(city).toBeGreaterThan(coarse)
  })

  it('uses a wider step at low zoom', () => {
    // 2° step at zoom 3: 181 meridians (-180..180) and 81 parallels (-80..80).
    expect(graticule(3).features.length).toBe(181 + 81)
  })
})

describe('boundsOfGeometry', () => {
  it('returns a padded lon/lat extent for a point', () => {
    const bounds = boundsOfGeometry({ type: 'Point', coordinates: [-120.5, 47.4] })
    expect(bounds).not.toBeNull()
    const [[minLon, minLat], [maxLon, maxLat]] = bounds!
    expect(minLon).toBeCloseTo(-120.5008)
    expect(minLat).toBeCloseTo(47.3992)
    expect(maxLon).toBeCloseTo(-120.4992)
    expect(maxLat).toBeCloseTo(47.4008)
  })

  it('frames a polygon by its outer ring', () => {
    const bounds = boundsOfGeometry({
      type: 'Polygon',
      coordinates: [
        [
          [-122.7, 45.5],
          [-122.6, 45.5],
          [-122.6, 45.6],
          [-122.7, 45.6],
          [-122.7, 45.5],
        ],
      ],
    })
    expect(bounds).not.toBeNull()
    const [[minLon, minLat], [maxLon, maxLat]] = bounds!
    expect(minLon).toBeLessThan(-122.7)
    expect(maxLon).toBeGreaterThan(-122.6)
    expect(minLat).toBeLessThan(45.5)
    expect(maxLat).toBeGreaterThan(45.6)
  })

  it('returns null for empty or missing geometry', () => {
    expect(boundsOfGeometry(null)).toBeNull()
    expect(boundsOfGeometry({ type: 'Point', coordinates: [] })).toBeNull()
  })
})
