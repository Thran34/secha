package report

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thran34/secha/internal/results"
)

type Figure struct {
	Name string
	SVG  string
}

func Figures(files []results.File, p Palette) []Figure {
	return []Figure{
		{Name: "mttd", SVG: MTTDChart(files, p)},
		{Name: "outcome-rates", SVG: RatesChart(files, p)},
	}
}

func WriteSVGs(dir string, files []results.File, p Palette) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create svg dir %s: %w", dir, err)
	}
	var written []string
	for _, fig := range Figures(files, p) {
		path := filepath.Join(dir, fig.Name+".svg")
		if err := os.WriteFile(path, []byte(fig.SVG), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
		written = append(written, path)
	}
	return written, nil
}
