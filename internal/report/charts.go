package report

import (
	"fmt"
	"math"

	"github.com/Thran34/secha/internal/results"
)

const (
	chartWidth  = 780
	labelGutter = 224
	rightGutter = 116
	zeroStub    = 3
)

func MTTDChart(files []results.File, p Palette) string {
	rowHeight := 46.0
	top := 78.0
	bottom := 62.0
	height := int(top + rowHeight*float64(len(files)) + bottom)

	c := newCanvas(chartWidth, height, p)
	c.title("Time to detect, per run",
		"Each dot is one experiment run. The bar spans min to max; the ringed dot is the mean.", p)

	upper, ticks := niceScale(maxMTTD(files))
	plotLeft := float64(labelGutter)
	plotRight := float64(chartWidth - rightGutter)
	scale := func(v float64) float64 {
		if upper == 0 {
			return plotLeft
		}
		return plotLeft + (v/upper)*(plotRight-plotLeft)
	}

	axisY := top + rowHeight*float64(len(files)) + 12
	for _, t := range ticks {
		x := scale(t)
		c.hairline(x, top-8, x, axisY, p.Grid)
		c.tabularText(x, axisY+20, formatMs(t), 11, 400, p.Muted, "middle")
	}
	c.hairline(plotLeft, axisY, plotRight, axisY, p.Axis)
	c.text(plotRight, axisY+38, "milliseconds", 11, 400, p.Muted, "end")

	for i, f := range files {
		y := top + rowHeight*float64(i) + rowHeight/2
		c.text(float64(labelGutter)-16, y+4, f.Experiment, 12, 400, p.TextPrimary, "end")

		if f.Summary.MTTDMs == nil {
			c.text(plotLeft, y+4, "never detected", 12, 400, p.Muted, "start")
			continue
		}

		s := f.Summary.MTTDMs
		if s.Max > s.Min {
			c.line(scale(s.Min), y, scale(s.Max), y, p.SeriesWash, 11)
		}
		for _, run := range f.Runs {
			if run.MTTDMs != nil {
				c.dot(scale(*run.MTTDMs), y, 4, p.Series[0], p.Surface)
			}
		}
		c.dot(scale(s.Mean), y, 5.5, p.Series[0], p.Surface)

		label := fmt.Sprintf("%s  n=%d", formatMs(s.Mean), s.N)
		if s.N > 1 {
			label = fmt.Sprintf("%s ± %s  n=%d", formatMs(s.Mean), formatMs(s.Stddev), s.N)
		}
		c.tabularText(plotRight+12, y+4, label, 11, 500, p.TextSecondary, "start")
	}

	return c.done()
}

var rateSeries = []struct {
	label string
	pick  func(results.Summary) float64
}{
	{"fault applied", func(s results.Summary) float64 { return s.FaultAppliedRate }},
	{"data exposed", func(s results.Summary) float64 { return s.DataExposedRate }},
	{"detected", func(s results.Summary) float64 { return s.DetectedRate }},
}

func RatesChart(files []results.File, p Palette) string {
	barHeight := 16.0
	barGap := 2.0
	groupHeight := barHeight*float64(len(rateSeries)) + barGap*float64(len(rateSeries)-1)
	groupPad := 26.0
	top := 98.0
	bottom := 46.0
	height := int(top + (groupHeight+groupPad)*float64(len(files)) + bottom)

	c := newCanvas(chartWidth, height, p)
	c.title("Outcome rates across runs",
		"Share of runs in which the fault landed, the data leaked, and something detected it.", p)

	legendX := 24.0
	for i, s := range rateSeries {
		c.swatch(legendX, 62, p.Series[i])
		c.text(legendX+16, 71, s.label, 11, 400, p.TextSecondary, "start")
		legendX += float64(18 + 8*len(s.label))
	}

	plotLeft := float64(labelGutter)
	plotRight := float64(chartWidth - rightGutter)
	scale := func(v float64) float64 { return (v / 100) * (plotRight - plotLeft) }

	axisY := top + (groupHeight+groupPad)*float64(len(files)) + 4
	for t := 0.0; t <= 100; t += 25 {
		x := plotLeft + scale(t)
		c.hairline(x, top-14, x, axisY, p.Grid)
		c.tabularText(x, axisY+20, fmt.Sprintf("%.0f%%", t), 11, 400, p.Muted, "middle")
	}
	c.hairline(plotLeft, axisY, plotRight, axisY, p.Axis)

	for i, f := range files {
		groupTop := top + (groupHeight+groupPad)*float64(i)
		c.text(plotLeft-16, groupTop+groupHeight/2+4, f.Experiment, 12, 400, p.TextPrimary, "end")

		for j, s := range rateSeries {
			pct := s.pick(f.Summary) * 100
			y := groupTop + (barHeight+barGap)*float64(j)
			width := scale(pct)
			if width < zeroStub {
				width = zeroStub
			}
			c.barRight(plotLeft, y, width, barHeight, 4, p.Series[j])
			c.tabularText(plotLeft+width+8, y+barHeight-4,
				fmt.Sprintf("%.0f%%", pct), 11, 500, p.TextSecondary, "start")
		}
	}

	return c.done()
}

func maxMTTD(files []results.File) float64 {
	max := 0.0
	for _, f := range files {
		if f.Summary.MTTDMs != nil && f.Summary.MTTDMs.Max > max {
			max = f.Summary.MTTDMs.Max
		}
	}
	return max
}

func niceScale(max float64) (float64, []float64) {
	if max <= 0 {
		return 0, []float64{0}
	}
	step := math.Pow(10, math.Floor(math.Log10(max/4)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if max/(step*m) <= 5 {
			step *= m
			break
		}
	}
	upper := math.Ceil(max/step) * step
	var ticks []float64
	for t := 0.0; t <= upper+step/2; t += step {
		ticks = append(ticks, t)
	}
	return upper, ticks
}

func formatMs(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v < 10:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}
