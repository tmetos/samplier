-- =============================================================================
-- Samplier — PostgreSQL/PostGIS schema
-- Requires: PostgreSQL 13+ and PostGIS 3+
-- Geometry policy: all geometry is stored in EPSG:4326 (WGS84). The project's
-- "native" coordinate system is recorded in project.srid and used for display
-- and native-CRS export/reprojection on demand.
--
-- Ordering note: reference vocabularies, unit, and analyte are created first so
-- foreign keys from the data tables resolve.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()

-- -----------------------------------------------------------------------------
-- Generic updated_at trigger
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- 1. UNITS & CONVERSIONS
-- =============================================================================

CREATE TABLE unit (
  code        text PRIMARY KEY,           -- 'mg/L', 'ug/L', 'mg/kg', 'ft', 'm', ...
  name        text NOT NULL,
  description text,
  quantity    text                        -- 'mass concentration', 'length', ...
);

CREATE TABLE unit_conversion (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  reported_unit     text NOT NULL REFERENCES unit(code),
  target_unit       text NOT NULL REFERENCES unit(code),
  conversion_factor numeric NOT NULL DEFAULT 1,  -- converted = value*factor + delta
  delta             numeric NOT NULL DEFAULT 0,
  formula           text,                 -- optional non-linear formula
  UNIQUE (reported_unit, target_unit)
);

-- =============================================================================
-- 2. REFERENCE VOCABULARIES
-- =============================================================================

CREATE TABLE matrix (
  code        text PRIMARY KEY,           -- WATER, SOIL, AIR, SEDIMENT, ...
  name        text NOT NULL,
  description text
);

CREATE TABLE fraction (
  code        text PRIMARY KEY,           -- TOTAL, DISSOLVED, SUSPENDED, ...
  name        text NOT NULL,
  description text
);

CREATE TABLE sample_type (
  code        text PRIMARY KEY,           -- N, FD, FB, TB, EB, DUP, ...
  name        text NOT NULL,
  description text
);

CREATE TABLE location_type (
  code        text PRIMARY KEY,           -- WELL, BORING, SURFACE_WATER, OUTFALL, SOIL, ...
  name        text NOT NULL,
  description text
);

CREATE TABLE method (
  code        text PRIMARY KEY,           -- EPA 8260D, SM 2540C, ...
  name        text NOT NULL,
  description text
);

CREATE TABLE lab (
  code        text PRIMARY KEY,
  name        text NOT NULL,
  description text
);

CREATE TABLE qualifier (
  code        text PRIMARY KEY,           -- J, U, B, E, ...
  description text NOT NULL
);

CREATE TABLE material (
  code        text PRIMARY KEY,           -- PVC, STEEL, SAND, ...
  name        text NOT NULL,
  description text
);

CREATE TABLE well_segment_type (
  code         text PRIMARY KEY,          -- SCREEN, CASING, FILTER_PACK, ...
  name         text NOT NULL,
  is_screen_yn boolean NOT NULL DEFAULT false
);

CREATE TABLE test_type (
  code        text PRIMARY KEY,           -- INITIAL, REANALYSIS, DILUTION, ...
  name        text NOT NULL
);

-- =============================================================================
-- 3. ANALYTES & GROUPS
-- =============================================================================

CREATE TABLE analyte (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  cas_rn        text UNIQUE,              -- null for field params (pH, temp, DO)
  chemical_name text NOT NULL,
  short_name    text,
  aliases       text[] NOT NULL DEFAULT '{}',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_analyte_name ON analyte (lower(chemical_name));
CREATE INDEX idx_analyte_aliases ON analyte USING gin (aliases);

CREATE TABLE analyte_group (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        text NOT NULL UNIQUE,
  name        text NOT NULL,
  description text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE analyte_group_member (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id      uuid NOT NULL REFERENCES analyte_group(id) ON DELETE CASCADE,
  analyte_id    uuid NOT NULL REFERENCES analyte(id) ON DELETE CASCADE,
  matrix        text REFERENCES matrix(code),
  fraction      text REFERENCES fraction(code),
  default_units text REFERENCES unit(code),
  display_order integer NOT NULL DEFAULT 0,
  UNIQUE (group_id, analyte_id, matrix, fraction)
);

-- =============================================================================
-- 4. IDENTITY / AUTH (OIDC/SSO backed)
-- =============================================================================

CREATE TABLE app_user (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  issuer       text NOT NULL,             -- OIDC issuer URL
  subject      text NOT NULL,             -- 'sub' claim
  email        text,
  display_name text,
  is_active    boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (issuer, subject)
);

CREATE TABLE api_key (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  name         text NOT NULL,
  key_hash     text NOT NULL,             -- SHA-256 of secret; never store plaintext
  scopes       text[] NOT NULL DEFAULT '{}',
  expires_at   timestamptz,
  last_used_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, name)
);

-- =============================================================================
-- 5. PROJECT / TASK / LOCATION
-- =============================================================================

CREATE TABLE project (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        text NOT NULL UNIQUE,
  name        text NOT NULL,
  notes       text,
  geometry    geometry(Geometry, 4326),   -- site boundary (polygon) or point
  srid        integer NOT NULL DEFAULT 4326, -- native/display SRID for this project
  start_time  timestamptz,
  end_time    timestamptz,
  elev_unit   text NOT NULL DEFAULT 'ft' REFERENCES unit(code),
  created_by  uuid REFERENCES app_user(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_project_geom ON project USING gist (geometry);

CREATE TABLE project_member (
  project_id uuid NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  role       text NOT NULL DEFAULT 'viewer'
             CHECK (role IN ('viewer','editor','admin')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, user_id)
);

CREATE TABLE task (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id  uuid NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  code        text NOT NULL,
  name        text,
  notes       text,
  start_date  date,
  end_date    date,
  geometry    geometry(Geometry, 4326),   -- optional task extent
  created_by  uuid REFERENCES app_user(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, code)
);
CREATE INDEX idx_task_project ON task (project_id);
CREATE INDEX idx_task_geom ON task USING gist (geometry);

CREATE TABLE location (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id     uuid NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  sys_loc_code   bigint GENERATED BY DEFAULT AS IDENTITY UNIQUE,
  name           text NOT NULL,             -- LOC_NAME
  loc_type       text NOT NULL REFERENCES location_type(code),
  geometry       geometry(Point, 4326) NOT NULL,
  notes          text,
  total_depth    numeric,
  depth_unit     text REFERENCES unit(code),
  elevation      numeric,                   -- ground surface elevation
  elev_unit      text REFERENCES unit(code),
  bearing        numeric,                   -- degrees (inclined wells)
  plunge         numeric,                   -- degrees from vertical
  status         text NOT NULL DEFAULT 'active',
  created_by     uuid REFERENCES app_user(id),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_location_project ON location (project_id);
CREATE INDEX idx_location_geom ON location USING gist (geometry);
CREATE INDEX idx_location_name ON location (lower(name));

-- Well construction header (one per location that is a well)
CREATE TABLE well (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id        uuid NOT NULL UNIQUE REFERENCES location(id) ON DELETE CASCADE,
  well_id            text,                 -- permit/registry id
  depth_of_well      numeric,
  depth_unit         text REFERENCES unit(code),
  top_casing_elev    numeric,
  geologic_unit_code text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

-- Well construction segments / screen intervals
CREATE TABLE well_segment (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id        uuid NOT NULL REFERENCES location(id) ON DELETE CASCADE,
  segment_type       text NOT NULL REFERENCES well_segment_type(code),
  material_type_code text REFERENCES material(code),
  start_depth        numeric NOT NULL,
  end_depth          numeric NOT NULL,
  depth_unit         text NOT NULL REFERENCES unit(code),
  inner_diameter     numeric,
  outer_diameter     numeric,
  remark             text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  CHECK (end_depth >= start_depth)
);
CREATE INDEX idx_well_segment_location ON well_segment (location_id, start_depth);

-- =============================================================================
-- 6. SAMPLE / BATCH / TEST / RESULT
-- =============================================================================

CREATE TABLE sample (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id          uuid NOT NULL REFERENCES task(id) ON DELETE CASCADE,
  location_id      uuid NOT NULL REFERENCES location(id) ON DELETE CASCADE,
  sys_sample_code  bigint GENERATED BY DEFAULT AS IDENTITY UNIQUE,
  name             text NOT NULL,          -- reported sample id (SAMPLE_NAME)
  sample_type      text NOT NULL DEFAULT 'N' REFERENCES sample_type(code),
  matrix           text NOT NULL REFERENCES matrix(code),
  start_depth      numeric,
  end_depth        numeric,
  depth_unit       text REFERENCES unit(code),
  sample_datetime  timestamptz NOT NULL,
  parent_sample_id uuid REFERENCES sample(id) ON DELETE SET NULL, -- duplicates
  notes            text,
  created_by       uuid REFERENCES app_user(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (task_id, name)
);
CREATE INDEX idx_sample_task ON sample (task_id);
CREATE INDEX idx_sample_location ON sample (location_id);
CREATE INDEX idx_sample_datetime ON sample (sample_datetime);
CREATE INDEX idx_sample_matrix ON sample (matrix);

-- Lab sample delivery group (SDG)
CREATE TABLE batch (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id    uuid NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  task_id       uuid REFERENCES task(id) ON DELETE SET NULL,
  name          text NOT NULL,            -- SDG name
  lab           text REFERENCES lab(code),
  received_date date,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name)
);

CREATE TABLE test (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sample_id       uuid NOT NULL REFERENCES sample(id) ON DELETE CASCADE,
  test_id         text NOT NULL,          -- business test id from lab
  analytic_method text REFERENCES method(code),
  fraction        text REFERENCES fraction(code),
  analysis_date   date,
  batch_id        uuid REFERENCES batch(id) ON DELETE SET NULL,
  test_type       text REFERENCES test_type(code),
  lab             text REFERENCES lab(code),
  column_number   integer,
  notes           text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (sample_id, test_id)
);
CREATE INDEX idx_test_sample ON test (sample_id);
CREATE INDEX idx_test_method ON test (analytic_method);
CREATE INDEX idx_test_batch ON test (batch_id);

CREATE TABLE result (
  id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  test_id                    uuid NOT NULL REFERENCES test(id) ON DELETE CASCADE,
  analyte_id                 uuid NOT NULL REFERENCES analyte(id) ON DELETE RESTRICT,
  result_numeric             numeric(28,19),  -- nullable when non-detect
  result_text                text,            -- 'ND', '<5', 'PRESENT', etc.
  detect_flag                char(1) CHECK (detect_flag IN ('Y','N')),
  units                      text REFERENCES unit(code),
  reporting_detection_limit  numeric,         -- RDL (used for non-detect '< DL')
  method_detection_limit     numeric,         -- MDL
  quantitation_limit         numeric,         -- QL / LOQ
  detection_limit_unit       text REFERENCES unit(code),
  lab_qualifiers             text,
  validator_qualifiers       text,
  interpreted_qualifiers     text,
  reportable_result          boolean NOT NULL DEFAULT true,
  reported_name              text,            -- verbatim chemical name from lab
  notes                      text,
  created_at                 timestamptz NOT NULL DEFAULT now(),
  updated_at                 timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_result_test ON result (test_id);
CREATE INDEX idx_result_analyte ON result (analyte_id);
CREATE INDEX idx_result_units ON result (units);
CREATE INDEX idx_result_detect ON result (detect_flag);

-- =============================================================================
-- 7. ACTION LEVELS (EPA / STATE / LOCAL / CUSTOM limits)
-- =============================================================================

CREATE TABLE action_level_type (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        text NOT NULL UNIQUE,       -- EPA, STATE, LOCAL, CUSTOM
  name        text NOT NULL,
  description text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE action_level (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  type_id       uuid NOT NULL REFERENCES action_level_type(id) ON DELETE CASCADE,
  code          text NOT NULL UNIQUE,
  name          text NOT NULL,
  matrix        text REFERENCES matrix(code),
  fraction      text REFERENCES fraction(code),
  start_date    date,
  end_date      date,
  project_id    uuid REFERENCES project(id) ON DELETE CASCADE, -- null = global
  land_use      text,
  geologic_unit text,
  depth_min     numeric,
  depth_max     numeric,
  notes         text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_action_level_type ON action_level (type_id);

CREATE TABLE action_level_parameter (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  action_level_id uuid NOT NULL REFERENCES action_level(id) ON DELETE CASCADE,
  analyte_id      uuid NOT NULL REFERENCES analyte(id) ON DELETE CASCADE,
  action_level    numeric NOT NULL,       -- exceed if result > this
  action_level_min numeric,               -- exceed if result < this (screening range)
  unit            text NOT NULL REFERENCES unit(code),
  fraction        text REFERENCES fraction(code),
  analytic_method text REFERENCES method(code),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (action_level_id, analyte_id, unit, fraction, analytic_method)
);
CREATE INDEX idx_action_level_param_analyte ON action_level_parameter (analyte_id);

-- =============================================================================
-- 8. EDD IMPORT ARTIFACTS (audit + validation)
-- =============================================================================

CREATE TABLE edd_format (
  code      text PRIMARY KEY,             -- EFWEDD, ...
  name      text NOT NULL,
  version   text,
  config    jsonb NOT NULL DEFAULT '{}',  -- section/column mapping
  is_active boolean NOT NULL DEFAULT true
);

CREATE TABLE edd_batch (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  format_code   text NOT NULL REFERENCES edd_format(code),
  file_name     text NOT NULL,
  commit_mode   text NOT NULL DEFAULT 'INSERT_ONLY'
                CHECK (commit_mode IN ('INSERT_ONLY','INSERT_UPDATE','INSERT_MERGE')),
  status        text NOT NULL DEFAULT 'VALIDATING'
                CHECK (status IN ('VALIDATING','VALIDATED','COMMITTED','ROLLED_BACK','ERROR')),
  error_count   integer NOT NULL DEFAULT 0,
  warning_count integer NOT NULL DEFAULT 0,
  user_id       uuid REFERENCES app_user(id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  committed_at  timestamptz
);

CREATE TABLE edd_validation_error (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  edd_batch_id uuid NOT NULL REFERENCES edd_batch(id) ON DELETE CASCADE,
  section      text NOT NULL,             -- e.g. EFW2LabRES
  row_number   integer,
  field        text,
  message      text NOT NULL,
  severity     text NOT NULL CHECK (severity IN ('ERROR','WARNING')),
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_edd_error_batch ON edd_validation_error (edd_batch_id);

-- =============================================================================
-- 8b. SAVED REPORT VIEWS (custom crosstab reports)
-- =============================================================================

CREATE TABLE report_view (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name                   text NOT NULL,
  description            text,
  title                  text,
  project_id             uuid REFERENCES project(id) ON DELETE CASCADE,
  analyte_ids            uuid[] NOT NULL DEFAULT '{}',
  analyte_group_ids      uuid[] NOT NULL DEFAULT '{}',
  action_level_group_ids uuid[] NOT NULL DEFAULT '{}',
  include_non_detects    boolean NOT NULL DEFAULT true,
  detects_only           boolean NOT NULL DEFAULT false,
  location_filter        text,
  start_date             date,
  end_date               date,
  config                 jsonb NOT NULL DEFAULT '{}',
  created_by             uuid REFERENCES app_user(id),
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_report_view_project ON report_view (project_id);

-- =============================================================================
-- 9. updated_at triggers
-- =============================================================================
DO $$
DECLARE
  t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'app_user','project','task','location','well','well_segment','sample','batch',
    'test','result','action_level','action_level_parameter','analyte','report_view'
  ] LOOP
    EXECUTE format('CREATE TRIGGER trg_%I_updated_at BEFORE UPDATE ON %I
                    FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
  END LOOP;
END $$;
