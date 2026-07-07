package inference

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPredictSuccess(t *testing.T) {
	raw := []byte("fake-image-bytes")
	annotated := []byte("annotated!")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != predictPath {
			t.Errorf("path = %q, want %q", r.URL.Path, predictPath)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		// Verify the request carries base64 of the raw bytes.
		var req predictRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("bad request body: %v", err)
		}
		if req.Image != base64.StdEncoding.EncodeToString(raw) {
			t.Errorf("image payload mismatch")
		}

		roundness := 0.812
		img := base64.StdEncoding.EncodeToString(annotated)
		json.NewEncoder(w).Encode(PredictResponse{
			Image:     &img,
			Roundness: &roundness,
			Labels:    map[string]float64{"bread": 0.9, "round": 0.7},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	res, err := c.Predict(context.Background(), raw)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if res.Roundness == nil || *res.Roundness != 0.812 {
		t.Errorf("roundness = %v, want 0.812", res.Roundness)
	}
	if res.Labels["bread"] != 0.9 {
		t.Errorf("labels[bread] = %v, want 0.9", res.Labels["bread"])
	}

	// SaveImage should decode back to the annotated bytes.
	out := filepath.Join(t.TempDir(), "predictions", "loaf.png")
	if err := res.SaveImage(out); err != nil {
		t.Fatalf("SaveImage: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read saved image: %v", err)
	}
	if string(got) != string(annotated) {
		t.Errorf("saved image = %q, want %q", got, annotated)
	}
}

func TestPredictNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.Predict(context.Background(), []byte("x")); err == nil {
		t.Error("expected error on non-200 response")
	}
}

func TestPredictNullFields(t *testing.T) {
	// The microservice may return nulls when it finds no bread.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"image": null, "roundness": null, "labels": null}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	res, err := c.Predict(context.Background(), []byte("x"))
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if res.Image != nil || res.Roundness != nil || res.Labels != nil {
		t.Errorf("expected all-nil fields, got %+v", res)
	}
	if err := res.SaveImage(filepath.Join(t.TempDir(), "x.png")); err == nil {
		t.Error("SaveImage should error when image is nil")
	}
}

func TestPredictFile(t *testing.T) {
	raw := []byte("file-bytes-here")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req predictRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		if req.Image != base64.StdEncoding.EncodeToString(raw) {
			t.Error("PredictFile did not send file bytes")
		}
		json.NewEncoder(w).Encode(PredictResponse{})
	}))
	defer srv.Close()

	p := filepath.Join(t.TempDir(), "img.jpg")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewClient(srv.URL)
	if _, err := c.PredictFile(context.Background(), p); err != nil {
		t.Fatalf("PredictFile: %v", err)
	}
}
