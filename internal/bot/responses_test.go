package bot

import "testing"

func TestMapConfidenceToSentiment(t *testing.T) {
	cases := []struct {
		conf  float64
		label string
		want  string
	}{
		{0.1, "bread", "seems bread, but I wouldn't trust it,"},
		{0.29, "bread", "seems bread, but I wouldn't trust it,"},
		{0.3, "bread", "not sure about bread,"},
		{0.35, "bread", "not sure about bread,"},
		{0.4, "bread", "bread is unlikely,"},
		{0.5, "bread", "slightly possible bread,"},
		{0.6, "bread", "moderately likely bread,"},
		{0.7, "bread", "probably bread,"},
		{0.8, "bread", "fairly confident in bread,"},
		{0.9, "bread", "pretty sure it's bread,"},
		{0.99, "bread", "pretty sure it's bread,"},
		{1.0, "bread", "Confirmed that it's bread,"},
		{1.5, "bread", "Confirmed that it's bread,"},
		// underscore -> space
		{0.85, "no_seeds", "fairly confident in no seeds,"},
		{0.2, "very_round_bread", "seems very round bread, but I wouldn't trust it,"},
	}
	for _, tc := range cases {
		if got := mapConfidenceToSentiment(tc.conf, tc.label); got != tc.want {
			t.Errorf("mapConfidenceToSentiment(%v, %q) = %q, want %q", tc.conf, tc.label, got, tc.want)
		}
	}
}

func TestMessageContentFromLabels(t *testing.T) {
	labels := []Label{
		{"bread", 0.87},
		{"no_seeds", 0.76},
		{"round", 0.4}, // below min_confidence 0.5 -> excluded
	}
	got := messageContentFromLabels(labels, 0.5)
	want := "This is certainly bread! fairly confident in bread, probably no seeds, "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMessageContentFromLabelsPreservesOrder(t *testing.T) {
	// Order of labels must be preserved in the output sentence.
	labels := []Label{
		{"round", 0.95},
		{"bread", 0.95},
	}
	got := messageContentFromLabels(labels, 0.5)
	want := "This is certainly bread! pretty sure it's round, pretty sure it's bread, "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMessageFromRoundness(t *testing.T) {
	if got := messageFromRoundness(nil); got != "I don't think this bread is round at all..." {
		t.Errorf("nil roundness = %q", got)
	}
	r := 0.8123
	got := messageFromRoundness(&r)
	want := "This bread seems 81.23% round. Anything over an 80% is pretty close to a sphere!"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
