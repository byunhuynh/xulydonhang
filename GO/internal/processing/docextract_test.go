package processing

import (
	"strings"
	"testing"
)

// sample_coop_report.doc la bao cao JDA that Coop gui duoi dang Word
// 97-2003 ("HA THANH DDH CN44_46 02 09.10.doc", 4 don). Truoc day app bo
// qua han file nay vi chi doc duoc .docx.
const sampleCoopReportDoc = "testdata/sample_coop_report.doc"

func TestReadDocText_RealCoopReport(t *testing.T) {
	text, err := readDocText(sampleCoopReportDoc)
	if err != nil {
		t.Fatalf("readDocText = %v", err)
	}
	// Doi chieu voi chu Word tu doc ra (Document.Content.Text): 16508 ky
	// tu, khop tung ky tu sau khi doi het doan ve \n.
	if got := len(strings.TrimRight(text, "\n")); got != 16508 {
		t.Errorf("len(text) = %d, want 16508", got)
	}
	for _, want := range []string{
		"P/O Number:   104665191-00",
		"  3547984-5  NG BLUE huong thao moc 3kg                      EA   C04                       341640.00       341640.00        5.00       20.00         .00                              1,708,200.00\n",
		"Co.opMart Ha Noi",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q", want)
		}
	}
}

func TestExtractPageTexts_SplitsDocReportIntoOrders(t *testing.T) {
	pages, _, err := extractPageTexts(sampleCoopReportDoc)
	if err != nil {
		t.Fatalf("extractPageTexts = %v", err)
	}
	wantPOs := []string{"104665191-00", "104665192-00", "104665193-00", "104665194-00"}
	if len(pages) != len(wantPOs) {
		t.Fatalf("got %d orders, want %d", len(pages), len(wantPOs))
	}
	for i, po := range wantPOs {
		if !strings.Contains(pages[i], "P/O Number:   "+po) {
			t.Errorf("order %d does not carry PO %s", i+1, po)
		}
	}
}

func TestIsWordReport_AcceptsDocAndDocx(t *testing.T) {
	for path, want := range map[string]bool{
		"a.docx": true, "a.DOCX": true, "a.doc": true, "a.DOC": true,
		"a.pdf": false, "a.txt": false, "a.docm": false,
	} {
		if got := isWordReport(path); got != want {
			t.Errorf("isWordReport(%q) = %v, want %v", path, got, want)
		}
	}
}
