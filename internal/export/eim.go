package export

import (
	"bytes"
	"strings"
)

// EIMAdapter produces Washington State Department of Ecology EIM content.
//
// Two representations are supported:
//   - the EIM upload worksheet structure (CSV: one row per result, with the
//     Study/Location/Result columns EIM expects)
//   - EIM XML
//
// EIM identifies a study by a study ID, locations by EIM location ID, and
// analytes by EIM parameter codes.
type EIMAdapter struct{}

// Code implements Adapter.
func (EIMAdapter) Code() string { return "EIM" }

// ContentType implements Adapter.
func (EIMAdapter) ContentType() string { return "text/csv" }

// FileExtension implements Adapter.
func (EIMAdapter) FileExtension() string { return ".csv" }

// EIMOptions carries EIM submission metadata.
type EIMOptions struct {
	// Format is "csv" (default) or "xml".
	Format string
	// StudyID is the EIM study identifier.
	StudyID string
	// StudyName is the human-readable study name.
	StudyName string
	// LocationID maps a Samplier location name to an EIM location ID.
	LocationID map[string]string
	// ParameterCode maps a CAS RN / analyte name to an EIM parameter code.
	ParameterCode map[string]string
	// CollectedBy is the EIM "collected by" organization.
	CollectedBy string
	// LabName is the analyzing laboratory.
	LabName string
}

// DefaultEIMOptions returns EIM options with sensible defaults.
func DefaultEIMOptions(ds Dataset) EIMOptions {
	return EIMOptions{
		Format:        "csv",
		StudyID:       ds.ProjectCode,
		StudyName:     ds.ProjectName,
		LocationID:    map[string]string{},
		ParameterCode: map[string]string{},
		CollectedBy:   ds.ProjectName,
	}
}

// Code implements Adapter.
func (a EIMAdapter) Encode(ds Dataset, opts Options) ([]byte, error) {
	return EncodeEIM(ds, DefaultEIMOptions(ds), opts)
}

// EIM CSV headers matching the EIM submission worksheet.
var eimHeaders = []string{
	"Study ID",
	"Location ID",
	"Location Name",
	"Location Type",
	"Latitude",
	"Longitude",
	"Sample ID",
	"Sample Date",
	"Sample Time",
	"Matrix",
	"Sample Type",
	"Depth (ft)",
	"Parameter Name",
	"Parameter Code",
	"CAS Number",
	"Result Value",
	"Result Unit",
	"Qualifier",
	"Reporting Limit",
	"Data Quality Level",
	"Lab Name",
	"Method",
	"Comment",
}

// EncodeEIM encodes a dataset for EIM using explicit options.
func EncodeEIM(ds Dataset, eo EIMOptions, opts Options) ([]byte, error) {
	rows := filterRows(ds.Rows, opts)
	if strings.EqualFold(eo.Format, "xml") {
		return encodeEIMXML(ds, eo, rows), nil
	}

	var b bytes.Buffer
	b.WriteString(strings.Join(eimHeaders, ","))
	b.WriteString("\n")

	for _, r := range rows {
		depth := ""
		if r.StartDepth != nil {
			depth = formatNumber(*r.StartDepth)
		} else if r.EndDepth != nil {
			depth = formatNumber(*r.EndDepth)
		}

		qualifier := r.LabQualifier
		if r.IsNonDetect() && qualifier == "" {
			qualifier = "U"
		}

		dl := ""
		if d := r.DetectionLimit(); d != nil {
			dl = formatNumber(*d)
		}

		vals := []string{
			coalesceStr(eo.StudyID, ds.ProjectCode),
			eo.locationID(r.LocationName),
			r.LocationName,
			strings.ToUpper(r.LocationType),
			formatNumber(r.Latitude),
			formatNumber(r.Longitude),
			r.SampleName,
			formatDay(r.SampleDate),
			r.SampleDate.UTC().Format("15:04"),
			r.Matrix,
			r.SampleType,
			depth,
			r.AnalyteName,
			eo.parameterCode(r),
			r.CASRN,
			r.ReportedValue(),
			r.Units,
			qualifier,
			dl,
			eimDataQualityLevel(r),
			coalesceStr(eo.LabName, r.Lab),
			r.Method,
			r.ResultText,
		}
		for i, v := range vals {
			vals[i] = csvEscape(v)
		}
		b.WriteString(strings.Join(vals, ","))
		b.WriteString("\n")
	}
	return b.Bytes(), nil
}

// encodeEIMXML writes an EIM XML submission document.
func encodeEIMXML(ds Dataset, eo EIMOptions, rows []Row) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<EIMSubmission>\n")
	writeEl(&b, 2, "StudyID", coalesceStr(eo.StudyID, ds.ProjectCode))
	writeEl(&b, 2, "StudyName", coalesceStr(eo.StudyName, ds.ProjectName))
	writeEl(&b, 2, "CollectedBy", eo.CollectedBy)
	writeEl(&b, 2, "LabName", eo.LabName)
	b.WriteString("  <Results>\n")

	for _, r := range rows {
		b.WriteString("    <Result>\n")
		writeEl(&b, 6, "LocationID", eo.locationID(r.LocationName))
		writeEl(&b, 6, "LocationName", r.LocationName)
		writeEl(&b, 6, "Latitude", formatNumber(r.Latitude))
		writeEl(&b, 6, "Longitude", formatNumber(r.Longitude))
		writeEl(&b, 6, "SampleID", r.SampleName)
		writeEl(&b, 6, "SampleDate", formatDay(r.SampleDate))
		writeEl(&b, 6, "Matrix", r.Matrix)
		if r.StartDepth != nil {
			writeEl(&b, 6, "Depth", formatNumber(*r.StartDepth))
		}
		writeEl(&b, 6, "ParameterName", r.AnalyteName)
		writeEl(&b, 6, "ParameterCode", eo.parameterCode(r))
		writeEl(&b, 6, "CASNumber", r.CASRN)
		writeEl(&b, 6, "ResultValue", r.ReportedValue())
		writeEl(&b, 6, "ResultUnit", r.Units)
		writeEl(&b, 6, "Qualifier", eimQualifier(r))
		if d := r.DetectionLimit(); d != nil {
			writeEl(&b, 6, "ReportingLimit", formatNumber(*d))
		}
		writeEl(&b, 6, "DataQualityLevel", eimDataQualityLevel(r))
		writeEl(&b, 6, "Method", r.Method)
		b.WriteString("    </Result>\n")
	}

	b.WriteString("  </Results>\n")
	b.WriteString("</EIMSubmission>\n")
	return b.Bytes()
}

func (eo EIMOptions) locationID(name string) string {
	for k, v := range eo.LocationID {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return name
}

func (eo EIMOptions) parameterCode(r Row) string {
	for _, key := range []string{r.CASRN, r.AnalyteName} {
		if key == "" {
			continue
		}
		for k, v := range eo.ParameterCode {
			if strings.EqualFold(k, key) {
				return v
			}
		}
	}
	return coalesceStr(r.CASRN, r.AnalyteName)
}

func eimQualifier(r Row) string {
	if r.LabQualifier != "" {
		return r.LabQualifier
	}
	if r.IsNonDetect() {
		return "U"
	}
	return ""
}

// eimDataQualityLevel returns the EIM validation level derived from qualifiers.
func eimDataQualityLevel(r Row) string {
	if r.ValQualifier != "" {
		return "Validated"
	}
	return "Preliminary"
}
