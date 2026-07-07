// Package stats renders the roundness-history chart. It ports src/stats/plots.py
// (matplotlib/seaborn) to pure-Go gonum/plot so builds need no CGO or Python.
package stats

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// RoundnessPoint is one point in a user's roundness history: a 1-based index
// and a roundness fraction (0..1). Mirrors the (X, Y) tuples the Python plot
// consumed.
type RoundnessPoint struct {
	Index     int
	Roundness float64
}

// teal/orange to echo the original seaborn styling.
var (
	lineColor  = color.RGBA{R: 0x00, G: 0x80, B: 0x80, A: 0xff} // teal
	pointColor = color.RGBA{R: 0xff, G: 0xa5, B: 0x00, A: 0xff} // orange
)

// PlotRoundnessByUser writes a roundness-history line+scatter chart (Y as a
// percentage) to filePath as a PNG, creating parent directories. Ports
// plot_roundness_by_user. An empty data set still produces an (empty) chart.
func PlotRoundnessByUser(data []RoundnessPoint, filePath string) error {
	if dir := filepath.Dir(filePath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("stats: create dir %q: %w", dir, err)
		}
	}

	p := plot.New()
	p.Title.Text = "Amazing roundness history for User"
	p.X.Label.Text = "X"
	p.Y.Label.Text = "Y (%)"
	p.Add(plotter.NewGrid())

	pts := make(plotter.XYs, len(data))
	for i, d := range data {
		pts[i].X = float64(d.Index)
		pts[i].Y = d.Roundness * 100 // scale to percent, matching Python
	}

	if len(pts) > 0 {
		line, points, err := plotter.NewLinePoints(pts)
		if err != nil {
			return fmt.Errorf("stats: build line: %w", err)
		}
		line.Color = lineColor
		line.Width = vg.Points(2.5)
		line.Dashes = []vg.Length{vg.Points(6), vg.Points(4)} // dashed, matching linestyle="--"
		points.Color = pointColor
		points.Radius = vg.Points(4)
		p.Add(line, points)
	}

	if err := p.Save(12*vg.Inch, 7*vg.Inch, filePath); err != nil {
		return fmt.Errorf("stats: save plot %q: %w", filePath, err)
	}
	return nil
}
