-- =============================================================================
-- Samplier demo dataset
--
-- Two contrasting sites, so the UI shows a real mix of outcomes:
--
--   DEMO-1  Riverside Industrial Site   former plating works, groundwater
--   DEMO-2  Northgate Landfill          closed landfill, groundwater + surface water
--
-- Across both projects the samples deliberately cover every case:
--   * exceedances (some analytes over the limit)
--   * compliant (detects, but all below the limit)
--   * nothing detected (every analyte non-detect)
--
-- Idempotent: removes both demo projects first, then rebuilds them.
-- Run with:  make demo-data     (or)  docker exec -i samplier-db-1 \
--              psql -U samplier -d samplier -f - < demo/demo.sql
-- =============================================================================

BEGIN;

-- ---------------------------------------------------------------------------
-- Clean slate for the demo project (cascades to tasks/samples/tests/results)
-- ---------------------------------------------------------------------------
DELETE FROM project WHERE code IN ('DEMO-1', 'DEMO-2');
DELETE FROM action_level WHERE code LIKE 'DEMO-%';

-- ---------------------------------------------------------------------------
-- Project and sampling tasks
-- ---------------------------------------------------------------------------
INSERT INTO project (code, name, notes, srid, elev_unit, start_time, geometry)
VALUES (
  'DEMO-1',
  'Riverside Industrial Site',
  'Former plating facility. Quarterly groundwater monitoring under a state order.',
  4326,
  'ft',
  '2023-01-15',
  ST_GeomFromText(
    'POLYGON((-122.6772 45.5153, -122.6763 45.5192, -122.6733 45.5204,
              -122.6700 45.5197, -122.6687 45.5171, -122.6696 45.5138,
              -122.6719 45.5126, -122.6752 45.5130, -122.6770 45.5142,
              -122.6772 45.5153))', 4326)
);

INSERT INTO task (project_id, code, name, notes, start_date, end_date, geometry)
SELECT id, 'Q1-2024', 'Q1 2024 Groundwater Monitoring', 'First quarterly event of 2024',
       '2024-01-15', '2024-01-20',
       ST_GeomFromText(
         'POLYGON((-122.6772 45.5153, -122.6763 45.5192, -122.6733 45.5204,
                   -122.6700 45.5197, -122.6687 45.5171, -122.6706 45.5160,
                   -122.6740 45.5150, -122.6770 45.5142, -122.6772 45.5153))', 4326)
FROM project WHERE code = 'DEMO-1';

INSERT INTO task (project_id, code, name, notes, start_date, end_date, geometry)
SELECT id, 'Q2-2024', 'Q2 2024 Groundwater Monitoring', 'Second quarterly event of 2024',
       '2024-04-15', '2024-04-20',
       ST_GeomFromText(
         'POLYGON((-122.6706 45.5160, -122.6687 45.5171, -122.6696 45.5138,
                   -122.6719 45.5126, -122.6752 45.5130, -122.6770 45.5142,
                   -122.6740 45.5150, -122.6706 45.5160))', 4326)
FROM project WHERE code = 'DEMO-1';

-- ---------------------------------------------------------------------------
-- Monitoring wells (with construction details)
-- ---------------------------------------------------------------------------
INSERT INTO location (project_id, name, loc_type, geometry, total_depth, depth_unit,
                      elevation, elev_unit, notes)
SELECT id, 'MW-01', 'WELL', ST_SetSRID(ST_MakePoint(-122.6742, 45.5168), 4326),
       32, 'ft', 24.5, 'ft', 'Upgradient background well'
FROM project WHERE code = 'DEMO-1';

INSERT INTO location (project_id, name, loc_type, geometry, total_depth, depth_unit,
                      elevation, elev_unit, notes)
SELECT id, 'MW-02', 'WELL', ST_SetSRID(ST_MakePoint(-122.6728, 45.5151), 4326),
       38, 'ft', 22.8, 'ft', 'Source-area well, former plating line'
FROM project WHERE code = 'DEMO-1';

INSERT INTO location (project_id, name, loc_type, geometry, total_depth, depth_unit,
                      elevation, elev_unit, notes)
SELECT id, 'MW-03', 'WELL', ST_SetSRID(ST_MakePoint(-122.6710, 45.5138), 4326),
       35, 'ft', 21.9, 'ft', 'Downgradient well, near property boundary'
FROM project WHERE code = 'DEMO-1';

-- Well construction: header + screened interval for each well.
INSERT INTO well (location_id, well_id, depth_of_well, depth_unit, top_casing_elev)
SELECT id, name || '-W', total_depth, depth_unit, elevation + 3
FROM location WHERE project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

INSERT INTO well_segment (location_id, segment_type, start_depth, end_depth, depth_unit,
                          inner_diameter, outer_diameter, remark)
SELECT id, 'SCREEN', total_depth - 12, total_depth, 'ft', 2, 12, '12-ft well screen'
FROM location WHERE project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

INSERT INTO well_segment (location_id, segment_type, start_depth, end_depth, depth_unit,
                          inner_diameter, outer_diameter, remark)
SELECT id, 'CASING', 0, total_depth - 12, 'ft', 2, 8, 'Steel surface casing'
FROM location WHERE project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

-- ---------------------------------------------------------------------------
-- Analytes
-- ---------------------------------------------------------------------------
INSERT INTO analyte (cas_rn, chemical_name, short_name) VALUES
  ('7439-92-1', 'Lead',     'Pb'),
  ('7440-38-2', 'Arsenic',  'As'),
  ('7440-43-9', 'Cadmium',  'Cd'),
  ('71-43-2',   'Benzene',  'Benz'),
  ('79-01-6',   'Trichloroethene', 'TCE'),
  ('127-18-4',  'Tetrachloroethene', 'PCE'),
  ('75-01-4',   'Vinyl chloride', 'VC'),
  ('7440-47-3', 'Chromium', 'Cr')
ON CONFLICT (cas_rn) DO UPDATE
  SET chemical_name = EXCLUDED.chemical_name,
      short_name = EXCLUDED.short_name;

-- ---------------------------------------------------------------------------
-- Analyte grouping sets (each group becomes a crosstab tab in custom reports)
-- ---------------------------------------------------------------------------
INSERT INTO analyte_group (code, name, description) VALUES
  ('DEMO-METALS', 'Metals',     'Inorganic metals by ICP-MS (EPA 6020B)'),
  ('DEMO-VOCS',   'VOCs',       'Volatile organic compounds by GC/MS (EPA 8260D)')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

INSERT INTO analyte_group_member (group_id, analyte_id, matrix, display_order)
SELECT g.id, a.id, 'WATER', v.ord
FROM analyte_group g
JOIN (VALUES
  ('DEMO-METALS', '7439-92-1', 1),  -- Lead
  ('DEMO-METALS', '7440-38-2', 2),  -- Arsenic
  ('DEMO-METALS', '7440-43-9', 3),  -- Cadmium
  ('DEMO-METALS', '7440-47-3', 4),  -- Chromium
  ('DEMO-VOCS',   '71-43-2',   1),  -- Benzene
  ('DEMO-VOCS',   '79-01-6',   2),  -- TCE
  ('DEMO-VOCS',   '127-18-4',  3),  -- PCE
  ('DEMO-VOCS',   '75-01-4',   4)   -- Vinyl chloride
) AS v(code, cas_rn, ord) ON v.code = g.code
JOIN analyte a ON a.cas_rn = v.cas_rn
ON CONFLICT (group_id, analyte_id, matrix, fraction) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Samples: 3 wells x 2 quarters, including a field duplicate
-- ---------------------------------------------------------------------------
INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'Q1-2024-MW-01', 'N', 'WATER', 22, 32, 'ft',
       '2024-01-16 09:15:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'Q1-2024' AND l.name = 'MW-01'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'Q1-2024-MW-02', 'N', 'WATER', 26, 38, 'ft',
       '2024-01-16 11:40:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'Q1-2024' AND l.name = 'MW-02'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'Q1-2024-MW-03', 'N', 'WATER', 23, 35, 'ft',
       '2024-01-17 08:30:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'Q1-2024' AND l.name = 'MW-03'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

-- Field duplicate of MW-02 (links to its parent sample).
INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime,
                    parent_sample_id, notes)
SELECT s.task_id, s.location_id, 'Q1-2024-MW-02-FD', 'FD', 'WATER',
       26, 38, 'ft', s.sample_datetime + interval '10 minutes', s.id,
       'Field duplicate of Q1-2024-MW-02'
FROM sample s WHERE s.name = 'Q1-2024-MW-02';

-- Q2 round
INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'Q2-2024-MW-01', 'N', 'WATER', 22, 32, 'ft',
       '2024-04-16 09:05:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'Q2-2024' AND l.name = 'MW-01'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'Q2-2024-MW-02', 'N', 'WATER', 26, 38, 'ft',
       '2024-04-16 11:20:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'Q2-2024' AND l.name = 'MW-02'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'Q2-2024-MW-03', 'N', 'WATER', 23, 35, 'ft',
       '2024-04-17 08:45:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'Q2-2024' AND l.name = 'MW-03'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-1');

-- ---------------------------------------------------------------------------
-- Laboratory reference value
-- ---------------------------------------------------------------------------
INSERT INTO lab (code, name, description) VALUES
  ('TESTLABWA', 'TestAmerica Seattle', 'Demo contract laboratory')
ON CONFLICT (code) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Lab delivery group (SDG) and tests
-- ---------------------------------------------------------------------------
INSERT INTO batch (project_id, task_id, name, lab, received_date)
SELECT id, (SELECT id FROM task WHERE code = 'Q1-2024'), 'SDG-2401-118', 'TESTLABWA', '2024-01-18'
FROM project WHERE code = 'DEMO-1'
ON CONFLICT (project_id, name) DO NOTHING;

INSERT INTO batch (project_id, task_id, name, lab, received_date)
SELECT id, (SELECT id FROM task WHERE code = 'Q2-2024'), 'SDG-2404-142', 'TESTLABWA', '2024-04-18'
FROM project WHERE code = 'DEMO-1'
ON CONFLICT (project_id, name) DO NOTHING;

-- Metals test for every Q1/Q2 sample.
INSERT INTO test (sample_id, test_id, analytic_method, fraction, analysis_date,
                  batch_id, test_type, lab)
SELECT s.id,
       s.name || '-MET',
       'EPA 6020B',
       'TOTAL',
       (s.sample_datetime + interval '4 days')::date,
       b.id,
       'INITIAL',
       'TESTLABWA'
FROM sample s
JOIN batch b ON b.name = CASE WHEN s.name LIKE 'Q1%' THEN 'SDG-2401-118' ELSE 'SDG-2404-142' END
WHERE s.name LIKE '%MW-%';

-- VOC test for MW-02 and MW-03 samples only.
INSERT INTO test (sample_id, test_id, analytic_method, fraction, analysis_date,
                  batch_id, test_type, lab)
SELECT s.id,
       s.name || '-VOC',
       'EPA 8260D',
       'TOTAL',
       (s.sample_datetime + interval '3 days')::date,
       b.id,
       'INITIAL',
       'TESTLABWA'
FROM sample s
JOIN batch b ON b.name = CASE WHEN s.name LIKE 'Q1%' THEN 'SDG-2401-118' ELSE 'SDG-2404-142' END
WHERE s.name LIKE '%MW-02%' OR s.name LIKE '%MW-03%';

-- ---------------------------------------------------------------------------
-- Results
--
-- Metals: every sample, with non-detects below the reporting limit.
-- VOCs: MW-02 shows real contamination (TCE/PCE/Benzene above MCLs).
-- ---------------------------------------------------------------------------
WITH metal_values(sample_pattern, cas_rn, value, rdl) AS (
  VALUES
    -- Q1 MW-01 (background): all low / non-detect
    ('Q1-2024-MW-01', '7439-92-1', 0.0018::numeric, 0.001::numeric),
    ('Q1-2024-MW-01', '7440-38-2', NULL::numeric,   0.002::numeric),
    ('Q1-2024-MW-01', '7440-43-9', NULL::numeric,   0.0005::numeric),
    -- Q1 MW-02 (source area): Lead and Cadmium exceed
    ('Q1-2024-MW-02', '7439-92-1', 0.0421, 0.001),
    ('Q1-2024-MW-02', '7440-38-2', 0.0142, 0.002),
    ('Q1-2024-MW-02', '7440-43-9', 0.0087, 0.0005),
    -- Q1 MW-02 field duplicate (close to parent)
    ('Q1-2024-MW-02-FD', '7439-92-1', 0.0438, 0.001),
    ('Q1-2024-MW-02-FD', '7440-38-2', 0.0139, 0.002),
    ('Q1-2024-MW-02-FD', '7440-43-9', 0.0084, 0.0005),
    -- Q1 MW-03 (downgradient): Lead slightly over
    ('Q1-2024-MW-03', '7439-92-1', 0.0192, 0.001),
    ('Q1-2024-MW-03', '7440-38-2', 0.0071, 0.002),
    ('Q1-2024-MW-03', '7440-43-9', NULL,   0.0005),
    -- Q2 MW-01: nothing detected at all
    ('Q2-2024-MW-01', '7439-92-1', NULL, 0.001),
    ('Q2-2024-MW-01', '7440-38-2', NULL, 0.002),
    ('Q2-2024-MW-01', '7440-43-9', NULL, 0.0005),
    -- Q2 MW-02: still elevated
    ('Q2-2024-MW-02', '7439-92-1', 0.0387, 0.001),
    ('Q2-2024-MW-02', '7440-38-2', 0.0126, 0.002),
    ('Q2-2024-MW-02', '7440-43-9', 0.0079, 0.0005),
    -- Q2 MW-03: improving but still over
    ('Q2-2024-MW-03', '7439-92-1', 0.0164, 0.001),
    ('Q2-2024-MW-03', '7440-38-2', 0.0068, 0.002),
    ('Q2-2024-MW-03', '7440-43-9', NULL,   0.0005)
)
INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units,
                    reporting_detection_limit, detection_limit_unit,
                    lab_qualifiers, reportable_result)
SELECT t.id,
       a.id,
       m.value,
       CASE WHEN m.value IS NULL THEN 'N' ELSE 'Y' END,
       'mg/L',
       m.rdl,
       'mg/L',
       CASE WHEN m.value IS NULL THEN 'U' ELSE NULL END,
       true
FROM metal_values m
JOIN sample s ON s.name = m.sample_pattern
JOIN test t ON t.sample_id = s.id AND t.test_id LIKE '%-MET'
JOIN analyte a ON a.cas_rn = m.cas_rn;

WITH voc_values(sample_pattern, cas_rn, value, rdl) AS (
  VALUES
    -- Q1 MW-02: TCE and PCE and Benzene exceed EPA MCLs
    ('Q1-2024-MW-02', '71-43-2', 0.0124::numeric, 0.0005::numeric),
    ('Q1-2024-MW-02', '79-01-6', 0.0338,          0.0005),
    ('Q1-2024-MW-02', '127-18-4', 0.0189,         0.0005),
    -- Q1 MW-02 duplicate
    ('Q1-2024-MW-02-FD', '71-43-2', 0.0119, 0.0005),
    ('Q1-2024-MW-02-FD', '79-01-6', 0.0351, 0.0005),
    ('Q1-2024-MW-02-FD', '127-18-4', 0.0195, 0.0005),
    -- Q1 MW-03: not detected
    ('Q1-2024-MW-03', '71-43-2', NULL, 0.0005),
    ('Q1-2024-MW-03', '79-01-6', NULL, 0.0005),
    ('Q1-2024-MW-03', '127-18-4', NULL, 0.0005),
    -- Q2 MW-02: still above
    ('Q2-2024-MW-02', '71-43-2', 0.0102, 0.0005),
    ('Q2-2024-MW-02', '79-01-6', 0.0295, 0.0005),
    ('Q2-2024-MW-02', '127-18-4', 0.0164, 0.0005),
    -- Q2 MW-03: now detected just under
    ('Q2-2024-MW-03', '71-43-2', NULL,   0.0005),
    ('Q2-2024-MW-03', '79-01-6', 0.0041, 0.0005),
    ('Q2-2024-MW-03', '127-18-4', 0.0022, 0.0005)
)
INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units,
                    reporting_detection_limit, detection_limit_unit,
                    lab_qualifiers, reportable_result)
SELECT t.id,
       a.id,
       v.value,
       CASE WHEN v.value IS NULL THEN 'N' ELSE 'Y' END,
       'mg/L',
       v.rdl,
       'mg/L',
       CASE WHEN v.value IS NULL THEN 'U' ELSE NULL END,
       true
FROM voc_values v
JOIN sample s ON s.name = v.sample_pattern
JOIN test t ON t.sample_id = s.id AND t.test_id LIKE '%-VOC'
JOIN analyte a ON a.cas_rn = v.cas_rn;

-- ---------------------------------------------------------------------------
-- Action levels: EPA MCLs (drinking water maximum contaminant levels)
-- ---------------------------------------------------------------------------
INSERT INTO action_level (type_id, code, name, matrix, notes)
SELECT t.id, 'DEMO-EPA-MCL', 'EPA Drinking Water MCLs', 'WATER',
       'Maximum contaminant levels under the Safe Drinking Water Act'
FROM action_level_type t WHERE t.code = 'EPA';

INSERT INTO action_level_parameter (action_level_id, analyte_id, action_level, unit)
SELECT al.id, a.id, v.mcl, 'mg/L'
FROM action_level al
JOIN (VALUES
  ('7439-92-1', 0.015::numeric),   -- Lead (action level)
  ('7440-38-2', 0.010),            -- Arsenic (MCL)
  ('7440-43-9', 0.005),            -- Cadmium (MCL)
  ('71-43-2',   0.005),            -- Benzene (MCL)
  ('79-01-6',   0.005),            -- TCE (MCL)
  ('127-18-4',  0.005),            -- PCE (MCL)
  ('75-01-4',   0.002),            -- Vinyl chloride (MCL)
  ('7440-47-3', 0.100)             -- Chromium, total (MCL)
) AS v(cas_rn, mcl) ON true
JOIN analyte a ON a.cas_rn = v.cas_rn
WHERE al.code = 'DEMO-EPA-MCL';

-- A stricter state screening level so the UI shows two limit sets.
INSERT INTO action_level (type_id, code, name, matrix, notes)
SELECT t.id, 'DEMO-STATE-SL', 'State Screening Levels', 'WATER',
       'State risk-based screening levels (example values)'
FROM action_level_type t WHERE t.code = 'STATE';

INSERT INTO action_level_parameter (action_level_id, analyte_id, action_level, unit)
SELECT al.id, a.id, v.sl, 'mg/L'
FROM action_level al
JOIN (VALUES
  ('7439-92-1', 0.005::numeric),
  ('7440-38-2', 0.003),
  ('7440-43-9', 0.002),
  ('71-43-2',   0.001),
  ('79-01-6',   0.001),
  ('127-18-4',  0.001),
  ('75-01-4',   0.001),
  ('7440-47-3', 0.050)
) AS v(cas_rn, sl) ON true
JOIN analyte a ON a.cas_rn = v.cas_rn
WHERE al.code = 'DEMO-STATE-SL';

-- ===========================================================================
-- DEMO-2 — Northgate Landfill (closed municipal landfill)
--   MW-01 upgradient      : nothing detected, both rounds
--   MW-02 downgradient    : exceedances (heavy in spring, light in autumn)
--   SW-01 surface water   : detects but all compliant
-- ===========================================================================
INSERT INTO project (code, name, notes, srid, elev_unit, start_time, geometry)
VALUES (
  'DEMO-2',
  'Northgate Landfill',
  'Closed municipal landfill. Semi-annual groundwater and surface water monitoring.',
  4326,
  'ft',
  '2022-06-01',
  ST_GeomFromText(
    'POLYGON((-122.6584 45.5574, -122.6561 45.5602, -122.6523 45.5599,
              -122.6491 45.5572, -122.6477 45.5539, -122.6499 45.5515,
              -122.6535 45.5511, -122.6569 45.5528, -122.6584 45.5574))', 4326)
);

INSERT INTO task (project_id, code, name, notes, start_date, end_date, geometry)
SELECT id, 'NG-2024S', 'Spring 2024 Monitoring', 'First semi-annual event',
       '2024-03-11', '2024-03-13',
       ST_GeomFromText(
         'POLYGON((-122.6584 45.5574, -122.6561 45.5602, -122.6535 45.5585,
                   -122.6552 45.5522, -122.6569 45.5528, -122.6584 45.5574))', 4326)
FROM project WHERE code = 'DEMO-2';

INSERT INTO task (project_id, code, name, notes, start_date, end_date, geometry)
SELECT id, 'NG-2024F', 'Autumn 2024 Monitoring', 'Second semi-annual event',
       '2024-09-16', '2024-09-18',
       ST_GeomFromText(
         'POLYGON((-122.6535 45.5585, -122.6523 45.5599, -122.6491 45.5572,
                   -122.6477 45.5539, -122.6499 45.5515, -122.6530 45.5516,
                   -122.6535 45.5585))', 4326)
FROM project WHERE code = 'DEMO-2';

INSERT INTO location (project_id, name, loc_type, geometry, total_depth, depth_unit,
                      elevation, elev_unit, notes)
SELECT id, 'NG-MW-01', 'WELL', ST_SetSRID(ST_MakePoint(-122.6552, 45.5561), 4326),
       28, 'ft', 41.2, 'ft', 'Upgradient background well'
FROM project WHERE code = 'DEMO-2';

INSERT INTO location (project_id, name, loc_type, geometry, total_depth, depth_unit,
                      elevation, elev_unit, notes)
SELECT id, 'NG-MW-02', 'WELL', ST_SetSRID(ST_MakePoint(-122.6511, 45.5528), 4326),
       34, 'ft', 38.6, 'ft', 'Downgradient well, leachate-affected'
FROM project WHERE code = 'DEMO-2';

INSERT INTO location (project_id, name, loc_type, geometry, total_depth, depth_unit,
                      elevation, elev_unit, notes)
SELECT id, 'NG-SW-01', 'SURFACE_WATER', ST_SetSRID(ST_MakePoint(-122.6490, 45.5508), 4326),
       NULL, NULL, 32.4, 'ft', 'Downgradient surface water station'
FROM project WHERE code = 'DEMO-2';

-- Well construction for the two wells only.
INSERT INTO well (location_id, well_id, depth_of_well, depth_unit, top_casing_elev)
SELECT id, name || '-W', total_depth, depth_unit, elevation + 3
FROM location WHERE project_id = (SELECT id FROM project WHERE code = 'DEMO-2')
  AND loc_type = 'WELL';

INSERT INTO well_segment (location_id, segment_type, start_depth, end_depth, depth_unit,
                          inner_diameter, outer_diameter, remark)
SELECT id, 'SCREEN', total_depth - 10, total_depth, 'ft', 2, 12, '10-ft well screen'
FROM location WHERE project_id = (SELECT id FROM project WHERE code = 'DEMO-2')
  AND loc_type = 'WELL';

INSERT INTO well_segment (location_id, segment_type, start_depth, end_depth, depth_unit,
                          inner_diameter, outer_diameter, remark)
SELECT id, 'CASING', 0, total_depth - 10, 'ft', 2, 8, 'Steel surface casing'
FROM location WHERE project_id = (SELECT id FROM project WHERE code = 'DEMO-2')
  AND loc_type = 'WELL';

-- Samples
INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'NG-2024S-MW-01', 'N', 'WATER', 18, 28, 'ft',
       '2024-03-11 09:40:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'NG-2024S' AND l.name = 'NG-MW-01'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-2');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'NG-2024S-MW-02', 'N', 'WATER', 22, 34, 'ft',
       '2024-03-11 11:15:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'NG-2024S' AND l.name = 'NG-MW-02'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-2');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'NG-2024S-SW-01', 'N', 'WATER', NULL, NULL, NULL,
       '2024-03-12 10:05:00+00', 'Grab sample, 0.3 m depth'
FROM task t, location l
WHERE t.code = 'NG-2024S' AND l.name = 'NG-SW-01'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-2');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'NG-2024F-MW-01', 'N', 'WATER', 18, 28, 'ft',
       '2024-09-16 09:25:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'NG-2024F' AND l.name = 'NG-MW-01'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-2');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'NG-2024F-MW-02', 'N', 'WATER', 22, 34, 'ft',
       '2024-09-16 11:50:00+00', 'Low-flow purge'
FROM task t, location l
WHERE t.code = 'NG-2024F' AND l.name = 'NG-MW-02'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-2');

INSERT INTO sample (task_id, location_id, name, sample_type, matrix,
                    start_depth, end_depth, depth_unit, sample_datetime, notes)
SELECT t.id, l.id, 'NG-2024F-SW-01', 'N', 'WATER', NULL, NULL, NULL,
       '2024-09-17 10:30:00+00', 'Grab sample, 0.3 m depth'
FROM task t, location l
WHERE t.code = 'NG-2024F' AND l.name = 'NG-SW-01'
  AND t.project_id = (SELECT id FROM project WHERE code = 'DEMO-2');

-- Batches
INSERT INTO batch (project_id, task_id, name, lab, received_date)
SELECT id, (SELECT id FROM task WHERE code = 'NG-2024S'), 'SDG-2403-071', 'TESTLABWA', '2024-03-13'
FROM project WHERE code = 'DEMO-2'
ON CONFLICT (project_id, name) DO NOTHING;

INSERT INTO batch (project_id, task_id, name, lab, received_date)
SELECT id, (SELECT id FROM task WHERE code = 'NG-2024F'), 'SDG-2409-104', 'TESTLABWA', '2024-09-19'
FROM project WHERE code = 'DEMO-2'
ON CONFLICT (project_id, name) DO NOTHING;

-- Tests: metals and VOCs for every sample.
INSERT INTO test (sample_id, test_id, analytic_method, fraction, analysis_date,
                  batch_id, test_type, lab)
SELECT s.id, s.name || '-MET', 'EPA 6020B', 'TOTAL',
       (s.sample_datetime + interval '4 days')::date, b.id, 'INITIAL', 'TESTLABWA'
FROM sample s
JOIN task t ON t.id = s.task_id
JOIN project p ON p.id = t.project_id AND p.code = 'DEMO-2'
JOIN batch b ON b.name = CASE WHEN s.name LIKE 'NG-2024S%' THEN 'SDG-2403-071' ELSE 'SDG-2409-104' END;

INSERT INTO test (sample_id, test_id, analytic_method, fraction, analysis_date,
                  batch_id, test_type, lab)
SELECT s.id, s.name || '-VOC', 'EPA 8260D', 'TOTAL',
       (s.sample_datetime + interval '3 days')::date, b.id, 'INITIAL', 'TESTLABWA'
FROM sample s
JOIN task t ON t.id = s.task_id
JOIN project p ON p.id = t.project_id AND p.code = 'DEMO-2'
JOIN batch b ON b.name = CASE WHEN s.name LIKE 'NG-2024S%' THEN 'SDG-2403-071' ELSE 'SDG-2409-104' END;

-- Metals results
WITH ng_metals(sample_pattern, cas_rn, value, rdl) AS (
  VALUES
    -- Spring, upgradient: nothing detected
    ('NG-2024S-MW-01', '7439-92-1', NULL::numeric, 0.001::numeric),
    ('NG-2024S-MW-01', '7440-38-2', NULL,           0.002),
    ('NG-2024S-MW-01', '7440-43-9', NULL,           0.0005),
    ('NG-2024S-MW-01', '7440-47-3', NULL,           0.005),
    -- Spring, downgradient: exceedances for lead and arsenic
    ('NG-2024S-MW-02', '7439-92-1', 0.0210, 0.001),
    ('NG-2024S-MW-02', '7440-38-2', 0.0113, 0.002),
    ('NG-2024S-MW-02', '7440-43-9', 0.0012, 0.0005),
    ('NG-2024S-MW-02', '7440-47-3', 0.0081, 0.005),
    -- Spring, surface water: detects, all compliant
    ('NG-2024S-SW-01', '7439-92-1', 0.0031, 0.001),
    ('NG-2024S-SW-01', '7440-38-2', 0.0024, 0.002),
    ('NG-2024S-SW-01', '7440-43-9', NULL,   0.0005),
    ('NG-2024S-SW-01', '7440-47-3', 0.0011, 0.005),
    -- Autumn, upgradient: still nothing detected
    ('NG-2024F-MW-01', '7439-92-1', NULL, 0.001),
    ('NG-2024F-MW-01', '7440-38-2', NULL, 0.002),
    ('NG-2024F-MW-01', '7440-43-9', NULL, 0.0005),
    ('NG-2024F-MW-01', '7440-47-3', NULL, 0.005),
    -- Autumn, downgradient: compliant on metals
    ('NG-2024F-MW-02', '7439-92-1', 0.0121, 0.001),
    ('NG-2024F-MW-02', '7440-38-2', 0.0064, 0.002),
    ('NG-2024F-MW-02', '7440-43-9', NULL,   0.0005),
    ('NG-2024F-MW-02', '7440-47-3', 0.0033, 0.005),
    -- Autumn, surface water: compliant, mostly non-detect
    ('NG-2024F-SW-01', '7439-92-1', 0.0018, 0.001),
    ('NG-2024F-SW-01', '7440-38-2', NULL,   0.002),
    ('NG-2024F-SW-01', '7440-43-9', NULL,   0.0005),
    ('NG-2024F-SW-01', '7440-47-3', 0.0009, 0.005)
)
INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units,
                    reporting_detection_limit, detection_limit_unit,
                    lab_qualifiers, reportable_result)
SELECT t.id, a.id, m.value,
       CASE WHEN m.value IS NULL THEN 'N' ELSE 'Y' END,
       'mg/L', m.rdl, 'mg/L',
       CASE WHEN m.value IS NULL THEN 'U' ELSE NULL END, true
FROM ng_metals m
JOIN sample s ON s.name = m.sample_pattern
JOIN test t ON t.sample_id = s.id AND t.test_id LIKE '%-MET'
JOIN analyte a ON a.cas_rn = m.cas_rn;

-- VOC results
WITH ng_vocs(sample_pattern, cas_rn, value, rdl) AS (
  VALUES
    -- Spring, upgradient: nothing detected
    ('NG-2024S-MW-01', '75-01-4',  NULL::numeric, 0.0005::numeric),
    ('NG-2024S-MW-01', '71-43-2',  NULL,          0.0005),
    ('NG-2024S-MW-01', '79-01-6',  NULL,          0.0005),
    ('NG-2024S-MW-01', '127-18-4', NULL,          0.0005),
    -- Spring, downgradient: vinyl chloride, benzene and TCE over
    ('NG-2024S-MW-02', '75-01-4',  0.0125, 0.0005),
    ('NG-2024S-MW-02', '71-43-2',  0.0074, 0.0005),
    ('NG-2024S-MW-02', '79-01-6',  0.0061, 0.0005),
    ('NG-2024S-MW-02', '127-18-4', 0.0018, 0.0005),
    -- Spring, surface water: compliant
    ('NG-2024S-SW-01', '75-01-4',  NULL,   0.0005),
    ('NG-2024S-SW-01', '71-43-2',  NULL,   0.0005),
    ('NG-2024S-SW-01', '79-01-6',  0.0008, 0.0005),
    ('NG-2024S-SW-01', '127-18-4', NULL,   0.0005),
    -- Autumn, upgradient: nothing detected
    ('NG-2024F-MW-01', '75-01-4',  NULL, 0.0005),
    ('NG-2024F-MW-01', '71-43-2',  NULL, 0.0005),
    ('NG-2024F-MW-01', '79-01-6',  NULL, 0.0005),
    ('NG-2024F-MW-01', '127-18-4', NULL, 0.0005),
    -- Autumn, downgradient: only vinyl chloride still over
    ('NG-2024F-MW-02', '75-01-4',  0.0042, 0.0005),
    ('NG-2024F-MW-02', '71-43-2',  0.0031, 0.0005),
    ('NG-2024F-MW-02', '79-01-6',  NULL,   0.0005),
    ('NG-2024F-MW-02', '127-18-4', NULL,   0.0005),
    -- Autumn, surface water: nothing detected
    ('NG-2024F-SW-01', '75-01-4',  NULL, 0.0005),
    ('NG-2024F-SW-01', '71-43-2',  NULL, 0.0005),
    ('NG-2024F-SW-01', '79-01-6',  NULL, 0.0005),
    ('NG-2024F-SW-01', '127-18-4', NULL, 0.0005)
)
INSERT INTO result (test_id, analyte_id, result_numeric, detect_flag, units,
                    reporting_detection_limit, detection_limit_unit,
                    lab_qualifiers, reportable_result)
SELECT t.id, a.id, v.value,
       CASE WHEN v.value IS NULL THEN 'N' ELSE 'Y' END,
       'mg/L', v.rdl, 'mg/L',
       CASE WHEN v.value IS NULL THEN 'U' ELSE NULL END, true
FROM ng_vocs v
JOIN sample s ON s.name = v.sample_pattern
JOIN test t ON t.sample_id = s.id AND t.test_id LIKE '%-VOC'
JOIN analyte a ON a.cas_rn = v.cas_rn;

COMMIT;

-- ---------------------------------------------------------------------------
-- Summary: counts and outcome mix per project
-- ---------------------------------------------------------------------------
SELECT p.code,
       p.name,
       (SELECT count(*) FROM location l WHERE l.project_id = p.id) AS locations,
       (SELECT count(*) FROM task t WHERE t.project_id = p.id)     AS tasks,
       (SELECT count(*) FROM sample s JOIN task t ON t.id = s.task_id
         WHERE t.project_id = p.id)                              AS samples,
       (SELECT count(*) FROM result r
          JOIN test te ON te.id = r.test_id
          JOIN sample s ON s.id = te.sample_id
          JOIN task t ON t.id = s.task_id
         WHERE t.project_id = p.id)                              AS results,
       (SELECT count(*) FROM result r
          JOIN test te ON te.id = r.test_id
          JOIN sample s ON s.id = te.sample_id
          JOIN task t ON t.id = s.task_id
         WHERE t.project_id = p.id AND r.detect_flag = 'N')      AS nondetects
FROM project p
WHERE p.code IN ('DEMO-1', 'DEMO-2')
ORDER BY p.code;

-- Per-sample outcome mix (each sample: detected, non-detect, and whether any
-- analyte lands above an EPA MCL).
SELECT s.name AS sample,
       count(r.id)                                            AS results,
       count(r.id) FILTER (WHERE r.detect_flag = 'Y')          AS detects,
       count(r.id) FILTER (WHERE r.detect_flag = 'N')          AS nondetects,
       count(r.id) FILTER (
         WHERE r.detect_flag = 'Y'
           AND r.result_numeric > COALESCE((
                 SELECT p.action_level
                 FROM action_level_parameter p
                 JOIN action_level al ON al.id = p.action_level_id
                 WHERE al.code = 'DEMO-EPA-MCL' AND p.analyte_id = r.analyte_id
               ), 'Infinity'::numeric)
       )                                                      AS above_mcl
FROM sample s
JOIN test te ON te.sample_id = s.id
JOIN result r ON r.test_id = te.id
JOIN task t ON t.id = s.task_id
JOIN project p ON p.id = t.project_id
WHERE p.code IN ('DEMO-1', 'DEMO-2')
GROUP BY s.name
ORDER BY s.name;
