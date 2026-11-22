// Package edd implements the pluggable EDD (Electronic Data Deliverable)
// import framework and the built-in EFWEDD (4-file) lab format.
//
// A format is a declarative definition: sections map file worksheets (or
// delimited files) to logical EDD sections, and each section declares its
// columns, target field, type, and required/reference-value rules. Adding a new
// lab format means registering a new Definition, not changing the loader.
package edd

import (
	"fmt"
	"strings"
)

// ColumnType is the expected data type of a column value.
type ColumnType string

const (
	TypeString ColumnType = "string"
	TypeNumber ColumnType = "number"
	TypeDate   ColumnType = "date"
	TypeBool   ColumnType = "boolean"
)

// Column describes one EDD column.
type Column struct {
	// Header is the EDD header name (as it appears in the file).
	Header string
	// Field is the target database field name.
	Field string
	// Type is the expected value type.
	Type ColumnType
	// Required marks the column as mandatory.
	Required bool
	// ReferenceTable, when set, names the RT_* vocabulary used for validation.
	ReferenceTable string
	// AlternateKey marks the column as part of the section's alternate key.
	AlternateKey bool
}

// Section is one EDD table/section.
type Section struct {
	// Name is the EDD section name (e.g. EFW2LabRES) and, for delimited files,
	// the expected file name stem.
	Name string
	// TargetTable is the logical entity populated (sample, test, result...).
	TargetTable string
	// Columns are the expected columns.
	Columns []Column
	// Optional marks sections that may be absent.
	Optional bool
}

// Definition is a complete EDD format.
type Definition struct {
	Code        string
	Name        string
	Version     string
	Description string
	Sections    []Section
	// ParentSection names the section that must exist for records to be valid.
	// It is used for parent/child integrity checks.
	ParentSection map[string]string
}

// Section looks up a section by name (case-insensitive).
func (d Definition) Section(name string) (Section, bool) {
	for _, s := range d.Sections {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Section{}, false
}

// SectionNames returns the section names in definition order.
func (d Definition) SectionNames() []string {
	out := make([]string, 0, len(d.Sections))
	for _, s := range d.Sections {
		out = append(out, s.Name)
	}
	return out
}

// registry holds all known formats.
var registry = map[string]Definition{}

// Register adds a format definition to the registry.
func Register(d Definition) {
	registry[strings.ToUpper(d.Code)] = d
}

// Lookup returns a registered format by code.
func Lookup(code string) (Definition, error) {
	d, ok := registry[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return Definition{}, fmt.Errorf("unknown EDD format %q", code)
	}
	return d, nil
}

// Formats returns all registered format codes.
func Formats() []string {
	out := make([]string, 0, len(registry))
	for code := range registry {
		out = append(out, code)
	}
	return out
}

// efwedd is the 4-file lab format. Field and lab sample sections both map to
// the sample entity; only one should be populated per deliverable.
var efwedd = Definition{
	Code:        "EFWEDD",
	Name:        "EFWEDD 4-File Laboratory EDD",
	Version:     "1.0",
	Description: "Standard 4-file lab deliverable: field/lab samples, tests, results, batch.",
	ParentSection: map[string]string{
		"EFW2LabTST": "EFW2LabSMP",
		"EFW2LabRES": "EFW2LabTST",
	},
	Sections: []Section{
		{
			Name:        "EFW2FSample",
			TargetTable: "sample",
			Optional:    true,
			Columns: []Column{
				{Header: "SYS_LOC_CODE", Field: "sys_loc_code", Type: TypeNumber, Required: true},
				{Header: "SAMPLE_ID", Field: "name", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "SAMPLE_TYPE_CODE", Field: "sample_type", Type: TypeString, ReferenceTable: "sample_type"},
				{Header: "MATRIX_CODE", Field: "matrix", Type: TypeString, Required: true, ReferenceTable: "matrix"},
				{Header: "SAMPLE_DATE", Field: "sample_datetime", Type: TypeDate, Required: true, AlternateKey: true},
				{Header: "START_DEPTH", Field: "start_depth", Type: TypeNumber},
				{Header: "END_DEPTH", Field: "end_depth", Type: TypeNumber},
				{Header: "DEPTH_UNIT", Field: "depth_unit", Type: TypeString, ReferenceTable: "unit"},
				{Header: "TASK_CODE", Field: "task_code", Type: TypeString},
				{Header: "SAMPLE_NAME", Field: "reported_name", Type: TypeString},
			},
		},
		{
			Name:        "EFW2LabSMP",
			TargetTable: "sample",
			Optional:    true,
			Columns: []Column{
				{Header: "SYS_LOC_CODE", Field: "sys_loc_code", Type: TypeNumber, Required: true},
				{Header: "SAMPLE_ID", Field: "name", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "SAMPLE_TYPE_CODE", Field: "sample_type", Type: TypeString, ReferenceTable: "sample_type"},
				{Header: "MATRIX_CODE", Field: "matrix", Type: TypeString, Required: true, ReferenceTable: "matrix"},
				{Header: "SAMPLE_DATE", Field: "sample_datetime", Type: TypeDate, Required: true, AlternateKey: true},
				{Header: "START_DEPTH", Field: "start_depth", Type: TypeNumber},
				{Header: "END_DEPTH", Field: "end_depth", Type: TypeNumber},
				{Header: "DEPTH_UNIT", Field: "depth_unit", Type: TypeString, ReferenceTable: "unit"},
				{Header: "TASK_CODE", Field: "task_code", Type: TypeString},
				{Header: "LAB_SAMPLE_ID", Field: "lab_sample_id", Type: TypeString},
			},
		},
		{
			Name:        "EFW2LabTST",
			TargetTable: "test",
			Columns: []Column{
				{Header: "SAMPLE_ID", Field: "sample_id", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "TEST_ID", Field: "test_id", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "ANALYTIC_METHOD", Field: "analytic_method", Type: TypeString, AlternateKey: true, ReferenceTable: "method"},
				{Header: "FRACTION", Field: "fraction", Type: TypeString, AlternateKey: true, ReferenceTable: "fraction"},
				{Header: "ANALYSIS_DATE", Field: "analysis_date", Type: TypeDate, AlternateKey: true},
				{Header: "TEST_TYPE", Field: "test_type", Type: TypeString, ReferenceTable: "test_type"},
				{Header: "LAB_SDG", Field: "lab_sdg", Type: TypeString},
				{Header: "LAB_NAME_CODE", Field: "lab", Type: TypeString, ReferenceTable: "lab"},
			},
		},
		{
			Name:        "EFW2LabRES",
			TargetTable: "result",
			Columns: []Column{
				{Header: "SAMPLE_ID", Field: "sample_id", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "TEST_ID", Field: "test_id", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "CAS_RN", Field: "cas_rn", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "CHEMICAL_NAME", Field: "chemical_name", Type: TypeString},
				{Header: "RESULT_VALUE", Field: "result_numeric", Type: TypeNumber},
				{Header: "RESULT_UNIT", Field: "units", Type: TypeString, ReferenceTable: "unit"},
				{Header: "DETECT_FLAG", Field: "detect_flag", Type: TypeString},
				{Header: "REPORTING_DETECTION_LIMIT", Field: "reporting_detection_limit", Type: TypeNumber},
				{Header: "METHOD_DETECTION_LIMIT", Field: "method_detection_limit", Type: TypeNumber},
				{Header: "DETECTION_LIMIT_UNIT", Field: "detection_limit_unit", Type: TypeString, ReferenceTable: "unit"},
				{Header: "LAB_QUALIFIERS", Field: "lab_qualifiers", Type: TypeString},
				{Header: "VALIDATOR_QUALIFIERS", Field: "validator_qualifiers", Type: TypeString},
				{Header: "RESULT_TEXT", Field: "result_text", Type: TypeString},
			},
		},
		{
			Name:        "EFW2Batch",
			TargetTable: "batch",
			Optional:    true,
			Columns: []Column{
				{Header: "LAB_SDG", Field: "name", Type: TypeString, Required: true, AlternateKey: true},
				{Header: "LAB_NAME_CODE", Field: "lab", Type: TypeString, ReferenceTable: "lab"},
				{Header: "RECEIVED_DATE", Field: "received_date", Type: TypeDate},
			},
		},
	},
}

func init() {
	Register(efwedd)
}

// EFWEDD returns the built-in 4-file lab format definition.
func EFWEDD() Definition { return efwedd }
