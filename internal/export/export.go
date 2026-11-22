// Package export implements regulatory and reporting export adapters.
//
// Each adapter takes a neutral Dataset and produces bytes in the target format.
// Adapters are registered by code so new formats can be added without touching
// callers.
package export

import (
	"fmt"
	"time"
)

// Row is the neutral, fully-denormalised representation of one analytical
// result used by every exporter.
type Row struct {
	ProjectCode   string
	ProjectName   string
	LocationName  string
	LocationType  string
	Longitude     float64
	Latitude      float64
	SampleName    string
	SampleType    string
	Matrix        string
	SampleDate    time.Time
	StartDepth    *float64
	EndDepth      *float64
	DepthUnit     string
	TaskCode      string
	TestID        string
	Method        string
	Fraction      string
	AnalysisDate  *time.Time
	LabSDG        string
	Lab           string
	CASRN         string
	AnalyteID     string
	AnalyteName   string
	ResultNumeric *float64
	ResultText    string
	DetectFlag    string
	Units         string
	RDL           *float64
	MDL           *float64
	LimitUnit     string
	LabQualifier  string
	ValQualifier  string

	// Exceedance information, when evaluated.
	ActionLevelGroup string
	ActionLevelType  string
	ActionLevel      *float64
	Exceeded         bool
	ExceedanceNote   string
}

// ReportedValue renders the human-readable result: "<DL" for non-detects.
func (r Row) ReportedValue() string {
	if r.IsNonDetect() {
		dl := r.DetectionLimit()
		if dl == nil {
			return "ND"
		}
		return "< " + formatNumber(*dl)
	}
	if r.ResultNumeric != nil {
		return formatNumber(*r.ResultNumeric)
	}
	if r.ResultText != "" {
		return r.ResultText
	}
	return ""
}

// IsNonDetect reports whether the result is a non-detect.
func (r Row) IsNonDetect() bool {
	return r.DetectFlag == "N"
}

// DetectionLimit returns the first available detection limit.
func (r Row) DetectionLimit() *float64 {
	if r.RDL != nil {
		return r.RDL
	}
	if r.MDL != nil {
		return r.MDL
	}
	return nil
}

// Dataset is a set of rows plus report metadata.
type Dataset struct {
	Title       string
	ProjectCode string
	ProjectName string
	GeneratedAt time.Time
	Rows        []Row
	// ActionLevelGroups lists the groups used for exceedance evaluation.
	ActionLevelGroups []string
}

// Options controls per-adapter behaviour.
type Options struct {
	// AnalyteOrder fixes the crosstab column order (by CAS RN).
	AnalyteOrder []string
	// LocationOrder fixes the crosstab row order.
	LocationOrder []string
	// Title overrides the dataset title.
	Title string
	// IncludeNonDetects controls whether non-detect rows are emitted.
	IncludeNonDetects bool
	// LimitToDetects emits only detect rows (regulatory submissions).
	LimitToDetects bool
}

// Adapter converts a Dataset to bytes.
type Adapter interface {
	// Code is the adapter identifier (e.g. CROSSTAB, WQX).
	Code() string
	// ContentType is the MIME type of the produced file.
	ContentType() string
	// FileExtension is the file extension, including the dot.
	FileExtension() string
	// Encode produces the file bytes.
	Encode(ds Dataset, opts Options) ([]byte, error)
}

var adapters = map[string]Adapter{}

// Register adds an adapter to the registry.
func Register(a Adapter) {
	adapters[a.Code()] = a
}

// Get returns a registered adapter by code (case-insensitive).
func Get(code string) (Adapter, error) {
	for k, a := range adapters {
		if equalFold(k, code) {
			return a, nil
		}
	}
	return nil, fmt.Errorf("unknown export format %q", code)
}

// Codes returns all registered adapter codes.
func Codes() []string {
	out := make([]string, 0, len(adapters))
	for k := range adapters {
		out = append(out, k)
	}
	return out
}

func equalFold(a, b string) bool { return toUpper(a) == toUpper(b) }

func toUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if 'a' <= b[i] && b[i] <= 'z' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

func init() {
	Register(CrosstabAdapter{})
	Register(WQXAdapter{})
	Register(AQSAdapter{})
	Register(EIMAdapter{})
	Register(CSVAdapter{})
}
