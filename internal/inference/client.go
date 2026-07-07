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
type PredictResponse struct {
	Image     *string            `json:"image"`
	Roundness *float64           `json:"roundness"`
	Labels    map[string]float64 `json:"labels"`
}

// predictRequest is the POST body: {"image": "<base64>"}.
type predictRequest struct {
	Image string `json:"image"`
}

// Predict sends the raw image bytes (base64-encoded) to the microservice and
// returns its prediction. A non-200 response is an error, matching the Python
// PredictionError behavior.
func (c *Client) Predict(ctx context.Context, imageBytes []byte) (*PredictResponse, error) {
	body, err := json.Marshal(predictRequest{
		Image: base64.StdEncoding.EncodeToString(imageBytes),
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
// returns the prediction. Convenience wrapper over Predict.
func (c *Client) PredictFile(ctx context.Context, path string) (*PredictResponse, error) {
	imageBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("inference: read image %q: %w", path, err)
	}
	return c.Predict(ctx, imageBytes)
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
