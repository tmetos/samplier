package export

import (
	"bytes"
	"strconv"
	"strings"
)

// WQXAdapter produces a WQX (Water Quality Exchange) XML document.
//
// WQX is the XML implementation of the EPA ESAR standard. The payload follows
// the WQX 3.x physical/chemical submission shape:
//
//	<WQX><Organization>...<Project>...<MonitoringLocation>...
//	  <Activity><ActivityDescription><Result>...</Result></ActivityDescription></Activity>
type WQXAdapter struct{}

// Code implements Adapter.
func (WQXAdapter) Code() string { return "WQX" }

// ContentType implements Adapter.
func (WQXAdapter) ContentType() string { return "application/xml" }

// FileExtension implements Adapter.
func (WQXAdapter) FileExtension() string { return ".xml" }

// WQXOptions carries values WQX requires that Samplier does not store.
type WQXOptions struct {
	OrganizationID         string
	OrganizationName       string
	OrganizationFormalName string
	ElectronicAddress      string
	MonitoringLocationID   string
	MonitoringLocationName string
	MonitoringLocationType string
	SourceMapScale         string
	HorizontalAccuracy     string
	VerticalMeasure        string
	VerticalMeasureUnit    string
	VerticalDatum          string
}

// DefaultWQXOptions derives sensible WQX metadata from a dataset.
func DefaultWQXOptions(ds Dataset) WQXOptions {
	org := ds.ProjectCode
	if org == "" {
		org = "SAMPLIER"
	}
	return WQXOptions{
		OrganizationID:         org,
		OrganizationName:       ds.ProjectName,
		OrganizationFormalName: ds.ProjectName,
		ElectronicAddress:      "",
		MonitoringLocationName: "",
		MonitoringLocationType: "Well",
		VerticalDatum:          "NAVD88",
	}
}

// Encode implements Adapter.
func (WQXAdapter) Encode(ds Dataset, opts Options) ([]byte, error) {
	return EncodeWQX(ds, DefaultWQXOptions(ds), opts)
}

// EncodeWQX encodes a dataset as WQX XML with explicit organization metadata.
func EncodeWQX(ds Dataset, wo WQXOptions, opts Options) ([]byte, error) {
	rows := filterRows(ds.Rows, opts)

	// Group by location, then activity (sample).
	locations := groupByLocation(rows)

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<WQX xmlns="http://www.exchangenetwork.net/schema/wqx/3">` + "\n")

	// Organization
	b.WriteString("  <Organization>\n")
	writeEl(&b, 4, "OrganizationIdentifier", wo.OrganizationID)
	writeEl(&b, 4, "OrganizationFormalName", coalesceStr(wo.OrganizationFormalName, wo.OrganizationName, wo.OrganizationID))
	b.WriteString("    <ElectronicAddress>\n")
	writeEl(&b, 6, "ElectronicAddressText", wo.ElectronicAddress)
	writeEl(&b, 6, "ElectronicAddressTypeName", "URL")
	b.WriteString("    </ElectronicAddress>\n")
	b.WriteString("  </Organization>\n")

	// Project
	b.WriteString("  <Project>\n")
	writeEl(&b, 4, "ProjectIdentifier", ds.ProjectCode)
	writeEl(&b, 4, "ProjectName", coalesceStr(ds.ProjectName, ds.ProjectCode))
	b.WriteString("  </Project>\n")

	// Monitoring locations
	for _, locName := range orderedKeys(locations) {
		lrows := locations[locName]
		first := lrows[0]

		b.WriteString("  <MonitoringLocation>\n")
		b.WriteString("    <MonitoringLocationIdentity>\n")
		writeEl(&b, 6, "MonitoringLocationIdentifier", mapLocationIdentifier(wo, locName))
		writeEl(&b, 6, "MonitoringLocationName", coalesceStr(wo.MonitoringLocationName, locName))
		writeEl(&b, 6, "MonitoringLocationTypeName", coalesceStr(first.LocationType, wo.MonitoringLocationType))
		b.WriteString("    </MonitoringLocationIdentity>\n")
		b.WriteString("    <MonitoringLocationGeospatial>\n")
		writeEl(&b, 6, "LatitudeMeasure", formatNumber(first.Latitude))
		writeEl(&b, 6, "LongitudeMeasure", formatNumber(first.Longitude))
		writeEl(&b, 6, "SourceMapScaleNumeric", coalesceStr(wo.SourceMapScale, "24000"))
		writeEl(&b, 6, "HorizontalCollectionMethodName", "GPS")
		writeEl(&b, 6, "HorizontalCoordinateReferenceSystemDatumName", "WGS84")
		b.WriteString("    </MonitoringLocationGeospatial>\n")
		b.WriteString("  </MonitoringLocation>\n")

		// Activities grouped by sample.
		activities := groupBySample(lrows)
		for _, sampleName := range orderedKeys2(activities) {
			arows := activities[sampleName]
			firstRow := arows[0]

			b.WriteString("  <Activity>\n")
			b.WriteString("    <ActivityDescription>\n")
			writeEl(&b, 6, "ActivityIdentifier", sampleName)
			writeEl(&b, 6, "ActivityTypeCode", "Sample-Routine")
			writeEl(&b, 6, "ActivityMediaName", wqxMedia(firstRow.Matrix))
			writeEl(&b, 6, "ActivityStartDate", formatDay(firstRow.SampleDate))
			if !firstRow.SampleDate.IsZero() {
				b.WriteString("      <ActivityStartTime>\n")
				writeEl(&b, 8, "Time", firstRow.SampleDate.UTC().Format("15:04:05"))
				writeEl(&b, 8, "TimeZoneCode", "UTC")
				b.WriteString("      </ActivityStartTime>\n")
			}
			b.WriteString("    </ActivityDescription>\n")

			b.WriteString("    <Result>\n")
			for _, r := range arows {
				writeWQXResult(&b, r)
			}
			b.WriteString("    </Result>\n")
			b.WriteString("  </Activity>\n")
		}
	}

	b.WriteString("</WQX>\n")
	return b.Bytes(), nil
}

func writeWQXResult(b *bytes.Buffer, r Row) {
	b.WriteString("      <ResultDescription>\n")
	writeEl(b, 8, "ResultAlternateIdentifier", r.SampleName+"-"+coalesceStr(r.CASRN, r.AnalyteName))
	writeEl(b, 8, "CharacteristicName", r.AnalyteName)
	if r.CASRN != "" {
		writeEl(b, 8, "CharacteristicCASNumber", r.CASRN)
	}
	if r.Fraction != "" {
		writeEl(b, 8, "MethodSpeciationName", wqxSpeciation(r.Fraction))
	}
	writeEl(b, 8, "ResultSampleFractionText", coalesceStr(r.Fraction, "Total"))
	if r.Method != "" {
		b.WriteString("        <ResultAnalyticalMethod>\n")
		writeEl(b, 10, "MethodIdentifier", r.Method)
		writeEl(b, 10, "MethodIdentifierContext", "EPA")
		writeEl(b, 10, "MethodName", r.Method)
		b.WriteString("        </ResultAnalyticalMethod>\n")
	}
	b.WriteString("      </ResultDescription>\n")

	b.WriteString("      <ResultMeasure>\n")
	if r.IsNonDetect() {
		writeEl(b, 8, "ResultDetectionConditionText", "Not Detected")
	} else {
		writeEl(b, 8, "ResultMeasureValue", r.ReportedValue())
		writeEl(b, 8, "MeasureUnitCode", r.Units)
	}
	b.WriteString("      </ResultMeasure>\n")

	// Detection limit block.
	if dl := r.DetectionLimit(); dl != nil {
		b.WriteString("      <DetectionQuantitationLimit>\n")
		limitType := "Reporting Limit"
		if r.RDL == nil && r.MDL != nil {
			limitType = "Method Detection Level"
		}
		writeEl(b, 8, "DetectionQuantitationLimitTypeName", limitType)
		b.WriteString("        <DetectionQuantitationLimitMeasure>\n")
		writeEl(b, 10, "MeasureValue", formatNumber(*dl))
		writeEl(b, 10, "MeasureUnitCode", coalesceStr(r.LimitUnit, r.Units))
		b.WriteString("        </DetectionQuantitationLimitMeasure>\n")
		b.WriteString("      </DetectionQuantitationLimit>\n")
	}

	if r.LabQualifier != "" || r.ValQualifier != "" {
		b.WriteString("      <ResultLabInformation>\n")
		if r.LabQualifier != "" {
			writeEl(b, 8, "ResultCommentText", "Lab qualifier: "+r.LabQualifier)
		}
		b.WriteString("      </ResultLabInformation>\n")
	}

	// Precision / depth context.
	if r.StartDepth != nil || r.EndDepth != nil {
		b.WriteString("      <ResultDepthHeightMeasure>\n")
		depth := r.EndDepth
		if depth == nil {
			depth = r.StartDepth
		}
		writeEl(b, 8, "MeasureValue", formatNumber(*depth))
		writeEl(b, 8, "MeasureUnitCode", coalesceStr(r.DepthUnit, "ft"))
		b.WriteString("      </ResultDepthHeightMeasure>\n")
	}
}

// wqxMedia maps a Samplier matrix to a WQX ActivityMediaName.
func wqxMedia(matrix string) string {
	switch strings.ToUpper(matrix) {
	case "WATER":
		return "Water"
	case "SEDIMENT":
		return "Sediment"
	case "SOIL":
		return "Soil"
	case "AIR":
		return "Air"
	case "BIOTA":
		return "Biological Tissue"
	default:
		return coalesceStr(matrix, "Water")
	}
}

// wqxSpeciation maps a fraction to a WQX speciation term.
func wqxSpeciation(fraction string) string {
	switch strings.ToUpper(fraction) {
	case "DISSOLVED":
		return "Dissolved"
	case "TOTAL":
		return "Total"
	case "SUSPENDED":
		return "Suspended"
	default:
		return fraction
	}
}

func mapLocationIdentifier(wo WQXOptions, locName string) string {
	if wo.MonitoringLocationID != "" {
		return wo.MonitoringLocationID
	}
	return locName
}

func writeEl(b *bytes.Buffer, indent int, name, value string) {
	if value == "" {
		return
	}
	// Defensive: element names must not contain path separators, which would
	// produce invalid XML. Callers needing nesting must emit explicit blocks.
	safe := strings.ReplaceAll(name, "/", "_")
	b.WriteString(strings.Repeat(" ", indent))
	b.WriteString("<" + safe + ">")
	b.WriteString(xmlText(value))
	b.WriteString("</" + safe + ">\n")
}

func coalesceStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// groupByLocation buckets rows by location name.
func groupByLocation(rows []Row) map[string][]Row {
	out := map[string][]Row{}
	for _, r := range rows {
		out[r.LocationName] = append(out[r.LocationName], r)
	}
	return out
}

// groupBySample buckets rows by sample name.
func groupBySample(rows []Row) map[string][]Row {
	out := map[string][]Row{}
	for _, r := range rows {
		out[r.SampleName] = append(out[r.SampleName], r)
	}
	return out
}

// orderedKeys returns map keys sorted ascending.
func orderedKeys(m map[string][]Row) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func orderedKeys2(m map[string][]Row) []string { return orderedKeys(m) }

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

var _ = strconv.Itoa
