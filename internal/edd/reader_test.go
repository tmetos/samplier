package edd

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildXLSX creates an in-memory workbook with the given sheets.
func buildXLSX(t *testing.T, sheets map[string][][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	first := true
	for name, rows := range sheets {
		if first {
			// Rename the default sheet.
			if err := f.SetSheetName("Sheet1", name); err != nil {
				t.Fatalf("rename sheet: %v", err)
			}
			first = false
		} else {
			if _, err := f.NewSheet(name); err != nil {
				t.Fatalf("new sheet: %v", err)
			}
		}
		for r, row := range rows {
			for c, val := range row {
				cell, err := excelize.CoordinatesToCellName(c+1, r+1)
				if err != nil {
					t.Fatalf("cell name: %v", err)
				}
				if err := f.SetCellValue(name, cell, val); err != nil {
					t.Fatalf("set cell: %v", err)
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	return buf.Bytes()
}

func TestReadXLSX(t *testing.T) {
	def := EFWEDD()
	data := buildXLSX(t, map[string][][]string{
		"EFW2FSample": {
			{"SAMPLE_ID", "SYS_LOC_CODE", "MATRIX_CODE", "SAMPLE_DATE"},
			{"S-001", "1", "WATER", "2024-01-02"},
			{"S-002", "2", "SOIL", "2024-01-03"},
		},
		"EFW2LabTST": {
			{"SAMPLE_ID", "TEST_ID", "ANALYTIC_METHOD"},
			{"S-001", "T-1", "EPA 8260D"},
		},
	})

	wb, err := Read(bytes.NewReader(data), int64(len(data)), "lab.xlsx", def)
	if err != nil {
		t.Fatalf("read xlsx: %v", err)
	}
	smp, ok := wb.Get("EFW2FSample")
	if !ok {
		t.Fatalf("expected EFW2FSample, got %v", wb.Order)
	}
	if len(smp.Rows) != 2 {
		t.Fatalf("got %d sample rows", len(smp.Rows))
	}
	if _, ok := wb.Get("EFW2LabTST"); !ok {
		t.Fatal("expected EFW2LabTST")
	}
}

func TestReadZipOfCSVs(t *testing.T) {
	def := EFWEDD()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	files := map[string]string{
		"EFW2FSample.csv": "SAMPLE_ID,SYS_LOC_CODE,MATRIX_CODE,SAMPLE_DATE\nS-1,1,WATER,2024-01-02\n",
		"EFW2LabTST.csv":  "SAMPLE_ID,TEST_ID\nS-1,T-1\n",
		"notes.txt":       "this file has no section suffix and is ignored",
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	data := buf.Bytes()
	wb, err := Read(bytes.NewReader(data), int64(len(data)), "package.zip", def)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if _, ok := wb.Get("EFW2FSample"); !ok {
		t.Fatalf("expected EFW2FSample from zip, got %v", wb.Order)
	}
	if _, ok := wb.Get("EFW2LabTST"); !ok {
		t.Fatalf("expected EFW2LabTST from zip, got %v", wb.Order)
	}
}

func TestReadZipWithXLSX(t *testing.T) {
	def := EFWEDD()
	xlsx := buildXLSX(t, map[string][][]string{
		"EFW2FSample": {
			{"SAMPLE_ID", "SYS_LOC_CODE", "MATRIX_CODE", "SAMPLE_DATE"},
			{"S-1", "1", "WATER", "2024-01-02"},
		},
	})

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("lab.xlsx")
	_, _ = w.Write(xlsx)
	_ = zw.Close()

	data := buf.Bytes()
	wb, err := Read(bytes.NewReader(data), int64(len(data)), "package.zip", def)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if _, ok := wb.Get("EFW2FSample"); !ok {
		t.Fatalf("expected EFW2FSample, got %v", wb.Order)
	}
}

func TestReadUnknownExtensionSniffsZip(t *testing.T) {
	def := EFWEDD()
	xlsx := buildXLSX(t, map[string][][]string{
		"EFW2LabTST": {{"SAMPLE_ID", "TEST_ID"}, {"S-1", "T-1"}},
	})
	wb, err := Read(bytes.NewReader(xlsx), int64(len(xlsx)), "mystery.bin", def)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, ok := wb.Get("EFW2LabTST"); !ok {
		t.Fatalf("expected sniffed xlsx, got %v", wb.Order)
	}
}

func TestReadInvalidXLSX(t *testing.T) {
	def := EFWEDD()
	// "PK" prefix but not a valid zip/xlsx.
	data := []byte("PK\x03\x04garbage")
	if _, err := Read(bytes.NewReader(data), int64(len(data)), "bad.xlsx", def); err == nil {
		t.Fatal("expected error for corrupt xlsx")
	}
}

func TestReadDelimitedBadCSV(t *testing.T) {
	def := EFWEDD()
	// Unterminated quote with LazyQuotes still parses; use a reader error case:
	// an empty file yields no tables.
	data := []byte("")
	wb, err := Read(bytes.NewReader(data), 0, "EFW2FSample.csv", def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wb.Order) != 0 {
		t.Fatalf("expected no sections, got %v", wb.Order)
	}
}
