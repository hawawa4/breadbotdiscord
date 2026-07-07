package bot

import (
	"fmt"
	"strings"
)

// mapConfidenceToSentiment translates a confidence value into a sentiment
// phrase for a label. Ports map_confidence_to_sentiment exactly, including the
// underscore→space replacement in the label.
func mapConfidenceToSentiment(confidence float64, label string) string {
	label = strings.ReplaceAll(label, "_", " ")
	switch {
	case confidence < 0.3:
		return fmt.Sprintf("%s, need help", label)
	case confidence < 0.4:
		return fmt.Sprintf("not sure about %s", label)
	case confidence < 0.5:
		return fmt.Sprintf("%s is unlikely", label)
	case confidence < 0.6:
		return fmt.Sprintf("slightly possible %s", label)
	case confidence < 0.7:
		return fmt.Sprintf("moderately likely %s", label)
	case confidence < 0.8:
		return fmt.Sprintf("probably %s", label)
	case confidence < 0.9:
		return fmt.Sprintf("fairly confident in %s", label)
	case confidence < 1.0:
		return fmt.Sprintf("pretty sure it's %s", label)
	default:
		return fmt.Sprintf("Confirmed that it's %s", label)
	}
}

// messageContentFromLabels builds the "This is certainly bread! ..." sentence,
// appending a sentiment phrase for each label at or above minConfidence.
//
// Ports get_message_content_from_labels. Label order matters (it appears in the
// output), so labels must be provided in the microservice's JSON order — see
// orderedLabels / the ordered decode in bread.go.
func messageContentFromLabels(labels []Label, minConfidence float64) string {
	var sb strings.Builder
	sb.WriteString("This is certainly bread! ")
	for _, l := range labels {
		if l.Confidence >= minConfidence {
			sb.WriteString(mapConfidenceToSentiment(l.Confidence, l.Name))
			sb.WriteString(" ")
		}
	}
	return sb.String()
}

// messageFromRoundness renders the roundness sentence. A nil roundness yields
// the "not round at all" line. Ports get_message_from_roundness, including the
// two-decimal percentage formatting.
func messageFromRoundness(roundness *float64) string {
	if roundness == nil {
		return "I don't think this bread is round at all..."
	}
	return fmt.Sprintf("This bread seems %.2f%% round. Anything over an 80%% is pretty close to a sphere!", *roundness*100)
}

// Label is a single (name, confidence) pair, preserving the order labels
// arrive in from the microservice.
type Label struct {
	Name       string
	Confidence float64
}
