package bot

import (
	"strings"
	"testing"

	"github.com/hawawa4/breadbotdiscord/internal/inference"
)

func TestPredCachePutGet(t *testing.T) {
	c := newPredCache(3)
	pred := &inference.PredictResponse{Labels: map[string]float64{"bread": 0.9}}
	c.put(1, cachedPrediction{pred: pred, outFile: "a.png", inFile: "a.jpg"})

	got, ok := c.get(1)
	if !ok {
		t.Fatal("expected hit for key 1")
	}
	if got.outFile != "a.png" || got.pred != pred {
		t.Errorf("got %+v, want outFile=a.png and same pred", got)
	}
	if _, ok := c.get(999); ok {
		t.Error("expected miss for absent key")
	}
}

func TestPredCacheEvictsLRU(t *testing.T) {
	c := newPredCache(2)
	c.put(1, cachedPrediction{outFile: "1"})
	c.put(2, cachedPrediction{outFile: "2"})
	// Touch 1 so 2 becomes least-recently-used.
	if _, ok := c.get(1); !ok {
		t.Fatal("1 should still be present")
	}
	// Insert a third → evicts 2 (the LRU), keeps 1 and 3.
	c.put(3, cachedPrediction{outFile: "3"})

	if _, ok := c.get(2); ok {
		t.Error("key 2 should have been evicted")
	}
	if _, ok := c.get(1); !ok {
		t.Error("key 1 should have survived (was touched)")
	}
	if _, ok := c.get(3); !ok {
		t.Error("key 3 should be present")
	}
}

func TestRenderBreadMessage(t *testing.T) {
	f64 := func(v float64) *float64 { return &v }
	img := "x"

	cases := []struct {
		name       string
		pred       *inference.PredictResponse
		minConf    float64
		wantFile   string
		wantSubstr string
	}{
		{
			name:       "no bread label",
			pred:       &inference.PredictResponse{Labels: map[string]float64{"cat": 0.9}, LabelsOrder: []string{"cat"}},
			minConf:    0.5,
			wantFile:   "in.jpg",
			wantSubstr: "isn't bread at all",
		},
		{
			name:       "bread below gate",
			pred:       &inference.PredictResponse{Labels: map[string]float64{"bread": 0.4}, LabelsOrder: []string{"bread"}},
			minConf:    0.5,
			wantFile:   "in.jpg",
			wantSubstr: "very mildly bread",
		},
		{
			// Same 0.4 bread now passes because the relaxed gate is 0.05 — this
			// is the "are you sure" behavior.
			name:       "bread passes relaxed gate with image",
			pred:       &inference.PredictResponse{Labels: map[string]float64{"bread": 0.4}, LabelsOrder: []string{"bread"}, Image: &img, Roundness: f64(0.8)},
			minConf:    0.05,
			wantFile:   "out.png",
			wantSubstr: "round",
		},
		{
			name:       "bread passes but no image",
			pred:       &inference.PredictResponse{Labels: map[string]float64{"bread": 0.9}, LabelsOrder: []string{"bread"}},
			minConf:    0.5,
			wantFile:   "in.jpg",
			wantSubstr: "shape dough",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, comment := renderBreadMessage("out.png", "in.jpg", tc.pred, tc.minConf)
			if file != tc.wantFile {
				t.Errorf("file = %q, want %q", file, tc.wantFile)
			}
			if !strings.Contains(comment, tc.wantSubstr) {
				t.Errorf("comment %q does not contain %q", comment, tc.wantSubstr)
			}
		})
	}
}
