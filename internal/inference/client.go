// Package inference is the HTTP client for the bread-detection microservice.
//
// It ports src/inference/predict.py: POST base64 image bytes to
// {base}/predict/predict, decode a {image?, roundness?, labels?} response, and
// optionally save the annotated image. The dead ResultsMapper is not ported.
package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// requestTimeout matches the Python client's 30s timeout.
const requestTimeout = 30 * time.Second

// predictPath is the microservice endpoint (note the doubled segment, matching
// the Python client).
const predictPath = "/predict/predict"

// Client calls the bread-detection microservice.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a Client targeting baseURL (e.g. http://localhost:8000).
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// PredictResponse is the microservice response. All fields are optional: image
// is a base64-encoded annotated image, roundness is a fraction, labels maps
// label name to confidence.
//
// LabelsOrder preserves the order labels appear in the JSON. The Python
// implementation iterates labels in dict (== JSON) order when building the
// response sentence, so order is behaviorally significant; a plain Go map would
// randomize it.
type PredictResponse struct {
	Image       *string            `json:"image"`
	Roundness   *float64           `json:"roundness"`
	Labels      map[string]float64 `json:"labels"`
	LabelsOrder []string           `json:"-"`
}

// UnmarshalJSON decodes the response and additionally records the label key
// order as it appears in the raw JSON (json.Decoder token stream).
func (r *PredictResponse) UnmarshalJSON(data []byte) error {
	// Decode the standard fields via an alias to avoid recursion.
	type alias PredictResponse
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = PredictResponse(a)

	// Extract label order from the raw JSON.
	order, err := labelKeyOrder(data)
	if err != nil {
		return err
	}
	r.LabelsOrder = order
	return nil
}

// labelKeyOrder returns the keys of the top-level "labels" object in the order
// they appear in data. Returns nil if labels is absent or null.
func labelKeyOrder(data []byte) ([]string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	raw, ok := top["labels"]
	if !ok {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	// null labels → no order.
	if tok == nil {
		return nil, nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, nil
	}
	var order []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, nil
		}
		order = append(order, key)
		// Skip the value.
		if err := skipValue(dec); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// skipValue consumes one JSON value (which for label values is always a scalar,
// but handle nested containers defensively) from the decoder.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); ok && (delim == '{' || delim == '[') {
		depth := 1
		for depth > 0 {
			t, err := dec.Token()
			if err != nil {
				return err
			}
			if d, ok := t.(json.Delim); ok {
				if d == '{' || d == '[' {
					depth++
				} else {
					depth--
				}
			}
		}
	}
	return nil
}

// OrderedLabel is a label/confidence pair in JSON order.
type OrderedLabel struct {
	Name       string
	Confidence float64
}

// OrderedLabels returns the labels in the order they appeared in the JSON
// response, matching the Python iteration order used to build the response
// sentence.
func (r *PredictResponse) OrderedLabels() []OrderedLabel {
	out := make([]OrderedLabel, 0, len(r.Labels))
	for _, name := range r.LabelsOrder {
		out = append(out, OrderedLabel{Name: name, Confidence: r.Labels[name]})
	}
	return out
}

// predictRequest is the POST body. Image is the base64-encoded bytes. Threshold
// is an OPTIONAL minimum confidence the service may use to widen/narrow what it
// returns; it is omitted when zero. The current breadvision service ignores it
// (it always returns every label and we filter client-side), but sending it is
// harmless and lets us relax the service-side filter if it ever gains support.
type predictRequest struct {
	Image     string  `json:"image"`
	Threshold float64 `json:"threshold,omitempty"`
}

// Predict sends the raw image bytes (base64-encoded) to the microservice and
// returns its prediction. A non-200 response is an error, matching the Python
// PredictionError behavior. threshold is passed through as an optional request
// hint (see predictRequest); pass 0 to omit it.
func (c *Client) Predict(ctx context.Context, imageBytes []byte, threshold float64) (*PredictResponse, error) {
	body, err := json.Marshal(predictRequest{
		Image:     base64.StdEncoding.EncodeToString(imageBytes),
		Threshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("inference: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+predictPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("inference: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inference: request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inference: unexpected status %d", res.StatusCode)
	}

	var out PredictResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("inference: decode response: %w", err)
	}
	return &out, nil
}

// PredictFile reads an image from path, sends it to the microservice, and
// returns the prediction. Convenience wrapper over Predict. threshold is passed
// through (0 to omit).
func (c *Client) PredictFile(ctx context.Context, path string, threshold float64) (*PredictResponse, error) {
	imageBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("inference: read image %q: %w", path, err)
	}
	return c.Predict(ctx, imageBytes, threshold)
}

// SaveImage base64-decodes the annotated image and writes it to outPath,
// creating parent directories. Errors if there is no image. Mirrors save_img.
func (r *PredictResponse) SaveImage(outPath string) error {
	if r.Image == nil {
		return fmt.Errorf("inference: no image to save")
	}
	decoded, err := base64.StdEncoding.DecodeString(*r.Image)
	if err != nil {
		return fmt.Errorf("inference: decode image: %w", err)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("inference: create dir %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(outPath, decoded, 0o644); err != nil {
		return fmt.Errorf("inference: write image %q: %w", outPath, err)
	}
	return nil
}
