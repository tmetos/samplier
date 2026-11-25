import { describe, expect, it } from 'vitest'
import { boundsOfGeometry, needsGraticule } from './mapStyle'

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

  it('returns null for empty or missing geometry', () => {
    expect(boundsOfGeometry(null)).toBeNull()
    expect(boundsOfGeometry({ type: 'Point', coordinates: [] })).toBeNull()
  })
})
