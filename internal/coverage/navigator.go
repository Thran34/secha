package coverage

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	layerVersion     = "4.5"
	navigatorVersion = "4.9.0"
	domain           = "enterprise-attack"
)

type layer struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Domain          string            `json:"domain"`
	Versions        layerVersions     `json:"versions"`
	Techniques      []layerTechnique  `json:"techniques"`
	Gradient        layerGradient     `json:"gradient"`
	LegendItems     []layerLegendItem `json:"legendItems"`
	ShowTacticRowBg bool              `json:"showTacticRowBackground"`
	HideDisabled    bool              `json:"hideDisabled"`
	SortTechniques  int               `json:"sorting"`
}

type layerVersions struct {
	Navigator string `json:"navigator"`
	Layer     string `json:"layer"`
}

type layerTechnique struct {
	TechniqueID string `json:"techniqueID"`
	Score       int    `json:"score"`
	Color       string `json:"color"`
	Comment     string `json:"comment"`
	Enabled     bool   `json:"enabled"`
}

type layerGradient struct {
	Colors   []string `json:"colors"`
	MinValue int      `json:"minValue"`
	MaxValue int      `json:"maxValue"`
}

type layerLegendItem struct {
	Label string `json:"label"`
	Color string `json:"color"`
}

var statusColor = map[Status]string{
	Blocked:      "#4caf50",
	DetectedOnly: "#ffb300",
	Undetected:   "#e53935",
}

func WriteNavigatorLayer(w io.Writer, name string, r Result) error {
	l := layer{
		Name:        name,
		Description: "Resilience measured by secha. Score 100 = attack blocked, 50 = succeeded but detected, 0 = succeeded undetected.",
		Domain:      domain,
		Versions:    layerVersions{Navigator: navigatorVersion, Layer: layerVersion},
		Gradient: layerGradient{
			Colors:   []string{statusColor[Undetected], statusColor[DetectedOnly], statusColor[Blocked]},
			MinValue: 0,
			MaxValue: 100,
		},
		LegendItems: []layerLegendItem{
			{Label: "blocked", Color: statusColor[Blocked]},
			{Label: "succeeded, detected", Color: statusColor[DetectedOnly]},
			{Label: "succeeded, undetected", Color: statusColor[Undetected]},
		},
		Techniques: make([]layerTechnique, 0, len(r.Attack)),
	}

	for _, t := range r.Attack {
		st := t.Status()
		l.Techniques = append(l.Techniques, layerTechnique{
			TechniqueID: t.ID,
			Score:       st.Score(),
			Color:       statusColor[st],
			Comment:     comment(t),
			Enabled:     true,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(l)
}

func comment(t Technique) string {
	parts := []string{
		fmt.Sprintf("secha: %d run(s) across %s", t.Runs, strings.Join(t.Experiments, ", ")),
		fmt.Sprintf("exposed %.0f%%, detected %.0f%%, prevented %.0f%%",
			t.ExposedRate*100, t.DetectedRate*100, t.PreventedRate*100),
	}
	if t.MTTD != nil {
		parts = append(parts, fmt.Sprintf("mttd mean %.1f ms (n=%d)", t.MTTD.Mean, t.MTTD.N))
	}
	return strings.Join(parts, "; ")
}
