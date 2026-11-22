package export

import (
	"bytes"
	"strings"
)

// AQSAdapter produces EPA AQS submission content.
//
// Two flavours are supported:
//   - the legacy pipe-delimited "transaction format" (raw data RD transactions)
//   - AQS XML
//
// The pipe-delimited form is the most widely accepted for batch submissions and
// is used by default; set AQSOptions.Format = "xml" for XML.
type AQSAdapter struct{}

// Code implements Adapter.
func (AQSAdapter) Code() string { return "AQS" }

// ContentType implements Adapter.
func (AQSAdapter) ContentType() string { return "text/plain" }

// FileExtension implements Adapter.
func (AQSAdapter) FileExtension() string { return ".txt" }

// AQSOptions carries submission metadata and output format.
type AQSOptions struct {
	// Format is "pipe" (default) or "xml".
	Format string
	// SiteID is the 4-char AQS state code + 5-char county + 4-char site.
	SiteID string
	// ParameterCode maps a CAS RN / analyte name to an AQS parameter code.
	ParameterCode map[string]string
	// MethodCode maps a method name to an AQS method code.
	MethodCode map[string]string
	// ReportingOrganization is the submitting agency code.
	ReportingOrganization string
}

// DefaultAQSOptions returns AQS options with sensible defaults.
func DefaultAQSOptions() AQSOptions {
	return AQSOptions{
		Format:                "pipe",
		ParameterCode:         map[string]string{},
		MethodCode:            map[string]string{},
		ReportingOrganization: "SAMPLIER",
	}
}

// Code implements Adapter.
func (a AQSAdapter) Encode(ds Dataset, opts Options) ([]byte, error) {
	return EncodeAQS(ds, DefaultAQSOptions(), opts)
}

// EncodeAQS encodes a dataset using explicit AQS options.
func EncodeAQS(ds Dataset, ao AQSOptions, opts Options) ([]byte, error) {
	rows := filterRows(ds.Rows, opts)
	if strings.EqualFold(ao.Format, "xml") {
		return encodeAQSXML(ds, ao, rows)
	}
	return encodeAQSPipe(ds, ao, rows), nil
}

// Transaction type codes used by AQS raw data.
const (
	aqsTransactionRawData = "RD"
	aqsActionInsert       = "I"
)

// encodeAQSPipe writes AQS RD (raw data) transactions, pipe-delimited.
//
// Field order follows the AQS raw data transaction: transaction type, action
// code, site id, parameter code, sample duration/collection, sample date, and
// the result block.
func encodeAQSPipe(ds Dataset, ao AQSOptions, rows []Row) []byte {
	var b bytes.Buffer

	// Header/comment line describing the batch.
	b.WriteString("# Samplier AQS raw data export")
	if ds.ProjectCode != "" {
		b.WriteString(" | project=" + ds.ProjectCode)
	}
	b.WriteString(" | generated=" + formatDateTime(ds.GeneratedAt))
	b.WriteString("\n")

	for _, r := range rows {
		site := ao.SiteID
		if site == "" {
			site = r.LocationName
		}
		param := ao.parameterCode(r)
		method := ao.methodCode(r)

		resultValue := ""
		if !r.IsNonDetect() {
			resultValue = r.ReportedValue()
		}
		dl := ""
		if d := r.DetectionLimit(); d != nil {
			dl = formatNumber(*d)
		}

		fields := []string{
			aqsTransactionRawData,
			aqsActionInsert,
			site,
			param,
			aqsSampleDuration(r),
			formatDayCompact(r.SampleDate),
			aqsSampleTime(r),
			resultValue,
			aqsUnit(r.Units),
			dl,
			method,
			coalesceStr(r.LabQualifier, ""),
			aqsNullCode(r),
		}
		for i, f := range fields {
			fields[i] = strings.ReplaceAll(f, "|", "/")
		}
		b.WriteString(strings.Join(fields, "|"))
		b.WriteString("\n")
	}
	return b.Bytes()
}

// encodeAQSXML writes an AQS XML submission document.
func encodeAQSXML(ds Dataset, ao AQSOptions, rows []Row) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<AQData xmlns=\"http://www.epa.gov/aqs\">\n")
	writeEl(&b, 2, "ReportingOrganization", coalesceStr(ao.ReportingOrganization, "SAMPLIER"))
	writeEl(&b, 2, "ProjectCode", ds.ProjectCode)

	for _, r := range rows {
		site := ao.SiteID
		if site == "" {
			site = r.LocationName
		}
		b.WriteString("  <RawData>\n")
		writeEl(&b, 4, "TransactionType", aqsTransactionRawData)
		writeEl(&b, 4, "SiteIdentifier", site)
		writeEl(&b, 4, "ParameterCode", ao.parameterCode(r))
		writeEl(&b, 4, "SampleCollectionDate", formatDay(r.SampleDate))
		writeEl(&b, 4, "SampleCollectionTime", aqsSampleTime(r))
		writeEl(&b, 4, "SampleMeasurement", aqsMeasurement(r))
		writeEl(&b, 4, "UnitsOfMeasure", aqsUnit(r.Units))
		if d := r.DetectionLimit(); d != nil {
			writeEl(&b, 4, "DetectionLimit", formatNumber(*d))
		}
		writeEl(&b, 4, "MethodCode", ao.methodCode(r))
		writeEl(&b, 4, "QualifierCode", r.LabQualifier)
		writeEl(&b, 4, "NullDataCode", aqsNullCode(r))
		b.WriteString("  </RawData>\n")
	}
	b.WriteString("</AQData>\n")
	return b.Bytes(), nil
}

func (ao AQSOptions) parameterCode(r Row) string {
	for _, key := range []string{r.CASRN, r.AnalyteName} {
		if key == "" {
			continue
		}
		for k, v := range ao.ParameterCode {
			if strings.EqualFold(k, key) {
				return v
			}
		}
	}
	return coalesceStr(r.CASRN, r.AnalyteName)
}

func (ao AQSOptions) methodCode(r Row) string {
	if r.Method == "" {
		return ""
	}
	for k, v := range ao.MethodCode {
		if strings.EqualFold(k, r.Method) {
			return v
		}
	}
	return r.Method
}

// aqsMeasurement renders the measured value (blank for null-coded rows).
func aqsMeasurement(r Row) string {
	if r.IsNonDetect() {
		return ""
	}
	return r.ReportedValue()
}

// aqsNullCode returns the AQS null data code for non-detects.
func aqsNullCode(r Row) string {
	if !r.IsNonDetect() {
		return ""
	}
	// "ND" = no value detected below the detection limit.
	return "ND"
}

// aqsUnit maps units onto AQS unit codes (008 = ug/m3, 001 = ppm, ...).
func aqsUnit(u string) string {
	switch strings.ToLower(strings.TrimSpace(u)) {
	case "ug/l", "µg/l", "ug/kg", "µg/kg":
		return u
	case "mg/l":
		return "mg/L"
	case "ppb":
		return "ppb"
	case "ppm":
		return "ppm"
	case "ug/m3", "µg/m3":
		return "008"
	case "ppb carbon":
		return "078"
	default:
		return u
	}
}

// aqsSampleDuration returns the AQS sample duration code (7 = 24 hour).
func aqsSampleDuration(r Row) string {
	// Samplier stores an instant; AQS wants a duration code. Default to 24-hour.
	return "7"
}

func aqsSampleTime(r Row) string {
	if r.SampleDate.IsZero() {
		return ""
	}
	return r.SampleDate.UTC().Format("1504")
}
