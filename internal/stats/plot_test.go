package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlotRoundnessByUser(t *testing.T) {
	data := []RoundnessPoint{
		{Index: 1, Roundness: 0.88},
		{Index: 2, Roundness: 0.42},
		{Index: 3, Roundness: 0.91},
	}
	out := filepath.Join(t.TempDir(), "plots", "123_roundhistory.png")
	if err := PlotRoundnessByUser(data, out); err != nil {
		t.Fatalf("PlotRoundnessByUser: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if info.Size() == 0 {
		t.Error("plot file is empty")
	}
	// Confirm it's a PNG (magic bytes).
	f, _ := os.Open(out)
	defer f.Close()
	hdr := make([]byte, 8)
	if _, err := f.Read(hdr); err != nil {
		t.Fatal(err)
	}
	if string(hdr[1:4]) != "PNG" {
		t.Errorf("not a PNG file, header = %v", hdr)
	}
}

func TestPlotEmptyData(t *testing.T) {
	// A user with no history should still produce a valid (empty) chart, not
	// an error.
	out := filepath.Join(t.TempDir(), "empty.png")
	if err := PlotRoundnessByUser(nil, out); err != nil {
		t.Fatalf("PlotRoundnessByUser(nil): %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected output file: %v", err)
	}
}
