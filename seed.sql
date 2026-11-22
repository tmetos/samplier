-- =============================================================================
-- Samplier — essential seed data
-- Run after schema.sql. Values here are the minimum needed to load typical
-- environmental data; expand as needed per project.
-- =============================================================================

-- Units
INSERT INTO unit (code, name, quantity) VALUES
  ('mg/L',  'milligrams per liter',       'mass concentration'),
  ('ug/L',  'micrograms per liter',       'mass concentration'),
  ('ng/L',  'nanograms per liter',        'mass concentration'),
  ('mg/kg', 'milligrams per kilogram',    'mass concentration (solid)'),
  ('ug/kg', 'micrograms per kilogram',    'mass concentration (solid)'),
  ('%',     'percent',                    'dimensionless'),
  ('ppb',   'parts per billion',          'mass concentration'),
  ('ppm',   'parts per million',          'mass concentration'),
  ('ft',    'feet',                       'length'),
  ('m',     'meters',                     'length'),
  ('deg C', 'degrees Celsius',            'temperature'),
  ('deg F', 'degrees Fahrenheit',         'temperature')
ON CONFLICT (code) DO NOTHING;

-- Common unit conversions (converted = value * factor + delta)
INSERT INTO unit_conversion (reported_unit, target_unit, conversion_factor, delta) VALUES
  ('mg/L', 'ug/L', 1000, 0),
  ('ug/L', 'mg/L', 0.001, 0),
  ('mg/kg','ug/kg', 1000, 0),
  ('ug/kg','mg/kg', 0.001, 0),
  ('mg/L', 'ppm', 1, 0),
  ('ug/L', 'ppb', 1, 0),
  ('ft',   'm',   0.3048, 0),
  ('m',    'ft',  3.28084, 0)
ON CONFLICT (reported_unit, target_unit) DO NOTHING;

-- Matrix
INSERT INTO matrix (code, name) VALUES
  ('WATER', 'Water'),
  ('SOIL', 'Soil'),
  ('SEDIMENT', 'Sediment'),
  ('AIR', 'Air'),
  ('WASTE', 'Waste'),
  ('BIOTA', 'Biota/Tissue'),
  ('SLUDGE', 'Sludge'),
  ('OTHER', 'Other')
ON CONFLICT (code) DO NOTHING;

-- Fraction
INSERT INTO fraction (code, name) VALUES
  ('TOTAL', 'Total'),
  ('DISSOLVED', 'Dissolved'),
  ('SUSPENDED', 'Suspended'),
  ('EXTRACTABLE', 'Extractable')
ON CONFLICT (code) DO NOTHING;

-- Sample types
INSERT INTO sample_type (code, name, description) VALUES
  ('N',  'Normal',            'Routine sample'),
  ('FD', 'Field Duplicate',   'Duplicate collected in the field'),
  ('LD', 'Lab Duplicate',     'Duplicate prepared in the laboratory'),
  ('FB', 'Field Blank',       'Blank prepared in the field'),
  ('TB', 'Trip Blank',        'Blank transported with samples'),
  ('EB', 'Equipment Blank',   'Blank collected from sampling equipment'),
  ('MS', 'Matrix Spike',      'Matrix spike QC sample'),
  ('MSD','Matrix Spike Duplicate', 'Matrix spike duplicate QC sample')
ON CONFLICT (code) DO NOTHING;

-- Location types
INSERT INTO location_type (code, name) VALUES
  ('WELL', 'Monitoring Well'),
  ('BORING', 'Soil Boring'),
  ('SURFACE_WATER', 'Surface Water'),
  ('OUTFALL', 'Outfall'),
  ('SOIL', 'Soil Sample Location'),
  ('AIR', 'Air Monitoring Station'),
  ('SEDIMENT', 'Sediment Sample Location'),
  ('OTHER', 'Other')
ON CONFLICT (code) DO NOTHING;

-- Action level types (regulatory jurisdictions)
INSERT INTO action_level_type (code, name, description) VALUES
  ('EPA',    'EPA',              'US EPA federal limits'),
  ('STATE',  'State',            'State regulatory limits'),
  ('LOCAL',  'Local',            'Local / county limits'),
  ('CUSTOM', 'Custom',           'Client- or project-specific limits')
ON CONFLICT (code) DO NOTHING;

-- Well segment types
INSERT INTO well_segment_type (code, name, is_screen_yn) VALUES
  ('SCREEN', 'Screen', true),
  ('CASING', 'Casing', false),
  ('FILTER_PACK', 'Filter Pack', false),
  ('SEAL', 'Seal', false),
  ('OTHER', 'Other', false)
ON CONFLICT (code) DO NOTHING;

-- Test types
INSERT INTO test_type (code, name) VALUES
  ('INITIAL', 'Initial analysis'),
  ('REANALYSIS', 'Re-analysis'),
  ('DILUTION', 'Dilution'),
  ('CONFIRMATION', 'Confirmation')
ON CONFLICT (code) DO NOTHING;

-- Methods (common examples; add per project)
INSERT INTO method (code, name) VALUES
  ('EPA 8260D', 'Volatile Organic Compounds by GC/MS'),
  ('EPA 6020B', 'Metals by ICP-MS'),
  ('SM 2540C',  'Total Dissolved Solids'),
  ('SM 4500-H', 'pH (electrometric)'),
  ('EPA 8015D', 'Non-halogenated Organics by GC/FID')
ON CONFLICT (code) DO NOTHING;

-- Built-in EDD format registry (configs implemented in code)
INSERT INTO edd_format (code, name, version, config) VALUES
  ('EFWEDD', 'EFWEDD 4-File Laboratory EDD', '1.0',
   '{"sections":["EFW2FSample","EFW2LabSMP","EFW2LabTST","EFW2LabRES","EFW2Batch"]}')
ON CONFLICT (code) DO NOTHING;
