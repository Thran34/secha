package report

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thran34/secha/internal/results"
)

func detectedFile(name string, mean, sd, min, max float64, runs ...float64) results.File {
	f := results.File{
		Experiment: name,
		Summary: results.Summary{
			Runs:             len(runs),
			FaultAppliedRate: 1,
			DetectedRate:     1,
			MTTDMs:           &results.Stats{N: len(runs), Mean: mean, Stddev: sd, Min: min, Max: max},
		},
	}
	for _, v := range runs {
		ms := v
		f.Runs = append(f.Runs, results.Run{FaultApplied: true, Detected: true, MTTDMs: &ms})
	}
	return f
}

func preventedFile(name string) results.File {
	return results.File{
		Experiment: name,
		Runs:       []results.Run{{FaultApplied: false, RejectionDetail: "denied by policy"}},
		Summary:    results.Summary{Runs: 1},
	}
}

func assertWellFormed(t *testing.T, svg string) {
	t.Helper()
	if err := xml.Unmarshal([]byte(svg), new(struct {
		XMLName xml.Name
	})); err != nil {
		t.Fatalf("svg is not well-formed XML: %v", err)
	}
}

func TestMTTDChartIsWellFormedAndNamesEveryExperiment(t *testing.T) {
	files := []results.File{
		detectedFile("networkpolicy-web-to-db", 535, 19, 509, 574, 509, 535, 574),
		preventedFile("rbac-secret-read"),
	}

	svg := MTTDChart(files, Light)

	assertWellFormed(t, svg)
	for _, f := range files {
		if !strings.Contains(svg, f.Experiment) {
			t.Errorf("chart does not name %q", f.Experiment)
		}
	}
}

func TestMTTDChartSaysNeverDetectedRatherThanPlottingZero(t *testing.T) {
	svg := MTTDChart([]results.File{preventedFile("rbac-secret-read")}, Light)

	if !strings.Contains(svg, "never detected") {
		t.Error("an experiment that never detected must say so; plotting it at zero would read as instant detection")
	}
}

func TestMTTDChartPlotsOneDotPerDetectedRun(t *testing.T) {
	svg := MTTDChart([]results.File{detectedFile("e", 20, 5, 10, 30, 10, 20, 30)}, Light)

	if got := strings.Count(svg, "<circle"); got != 4 {
		t.Errorf("got %d circles, want 4 (three runs plus the mean marker)", got)
	}
}

func TestRatesChartKeepsIdentityAtZeroPercent(t *testing.T) {
	svg := RatesChart([]results.File{preventedFile("rbac-secret-read")}, Light)

	assertWellFormed(t, svg)
	for _, hex := range Light.Series {
		if !strings.Contains(svg, hex) {
			t.Errorf("series color %s is absent; a 0%% bar with no mark loses which series it belongs to", hex)
		}
	}
}

func TestRatesChartLabelsEveryBar(t *testing.T) {
	files := []results.File{detectedFile("e", 20, 1, 19, 21, 20)}

	svg := RatesChart(files, Light)

	if got := strings.Count(svg, "100%"); got < 2 {
		t.Errorf("expected direct labels on the bars, got %d occurrences of 100%%", got)
	}
}

func TestChartsDifferBetweenLightAndDark(t *testing.T) {
	files := []results.File{detectedFile("e", 20, 1, 19, 21, 20)}

	if MTTDChart(files, Light) == MTTDChart(files, Dark) {
		t.Error("dark mode must be its own palette, not the same SVG")
	}
	if !strings.Contains(MTTDChart(files, Dark), Dark.Surface) {
		t.Error("dark chart does not paint the dark surface")
	}
}

func TestWriteHTMLShipsBothThemesAndTheTable(t *testing.T) {
	files := []results.File{
		detectedFile("networkpolicy-web-to-db", 535, 19, 509, 574, 509, 535, 574),
		preventedFile("rbac-secret-read"),
	}

	var buf bytes.Buffer
	if err := WriteHTML(&buf, files); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	html := buf.String()

	for _, want := range []string{
		"<!doctype html>", Light.Surface, Dark.Surface,
		"All indicators", "prefers-color-scheme",
		"networkpolicy-web-to-db", "rbac-secret-read",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report is missing %q", want)
		}
	}
}

func TestWriteHTMLSurfacesARejectedFault(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHTML(&buf, []results.File{preventedFile("rbac-secret-read")}); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}

	if !strings.Contains(buf.String(), "denied by policy") {
		t.Error("a rejected fault is a result and must appear in the report")
	}
}

func TestWriteHTMLEscapesExperimentNames(t *testing.T) {
	f := preventedFile("<script>alert(1)</script>")

	var buf bytes.Buffer
	if err := WriteHTML(&buf, []results.File{f}); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}

	if strings.Contains(buf.String(), "<script>alert(1)</script>") {
		t.Error("experiment names must be escaped, they come from a file on disk")
	}
}

func TestWriteSVGsWritesOneFilePerFigure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "figures")
	files := []results.File{detectedFile("e", 20, 1, 19, 21, 20)}

	written, err := WriteSVGs(dir, files, Light)
	if err != nil {
		t.Fatalf("WriteSVGs: %v", err)
	}

	if len(written) != len(Figures(files, Light)) {
		t.Errorf("wrote %d files, want %d", len(written), len(Figures(files, Light)))
	}
	for _, path := range written {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		assertWellFormed(t, string(body))
	}
}

func TestNiceScaleCoversTheDataAndStartsAtZero(t *testing.T) {
	cases := []float64{1, 7, 27.4, 574, 1234}

	for _, max := range cases {
		upper, ticks := niceScale(max)
		if upper < max {
			t.Errorf("niceScale(%v) upper = %v, want >= max", max, upper)
		}
		if ticks[0] != 0 {
			t.Errorf("niceScale(%v) first tick = %v, want 0", max, ticks[0])
		}
		if len(ticks) < 2 || len(ticks) > 7 {
			t.Errorf("niceScale(%v) produced %d ticks, want a readable handful", max, len(ticks))
		}
	}
}

func TestNiceScaleToleratesNoData(t *testing.T) {
	upper, ticks := niceScale(0)

	if upper != 0 || len(ticks) != 1 {
		t.Errorf("niceScale(0) = %v, %v; want a degenerate scale rather than a panic", upper, ticks)
	}
}

func TestFormatMsKeepsPrecisionWhereItMatters(t *testing.T) {
	cases := map[float64]string{0: "0", 2.04: "2.0", 27.4: "27", 534.9: "535"}

	for in, want := range cases {
		if got := formatMs(in); got != want {
			t.Errorf("formatMs(%v) = %q, want %q", in, got, want)
		}
	}
}
