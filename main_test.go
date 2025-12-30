package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestServer() *http.ServeMux {
	// clear the store before each test
	inMemoryStore = make(map[string]map[string]ObjectData)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("GET /storage/v1/b/{bucket}/o", handleListObjects)
	mux.HandleFunc("GET /storage/v1/b/{bucket}/o/{object...}", handleGetObject)
	mux.HandleFunc("POST /storage/v1/b", handleCreateBucket)
	mux.HandleFunc("POST /upload/storage/v1/b/{bucket}/o", handleUploadObject)
	return mux
}

func TestHealthEndpoint(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got '%s'", w.Body.String())
	}
}

func TestCreateBucket(t *testing.T) {
	mux := setupTestServer()

	body := bytes.NewBufferString(`{"name": "test-bucket"}`)
	req := httptest.NewRequest("POST", "/storage/v1/b", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// check bucket was created
	if _, ok := inMemoryStore["test-bucket"]; !ok {
		t.Error("bucket was not created in store")
	}
}

func TestCreateDuplicateBucket(t *testing.T) {
	mux := setupTestServer()

	// create first bucket
	body := bytes.NewBufferString(`{"name": "dupe-bucket"}`)
	req := httptest.NewRequest("POST", "/storage/v1/b", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// try to create same bucket again
	body = bytes.NewBufferString(`{"name": "dupe-bucket"}`)
	req = httptest.NewRequest("POST", "/storage/v1/b", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409 for duplicate bucket, got %d", w.Code)
	}
}

func TestUploadAndGetObject(t *testing.T) {
	mux := setupTestServer()

	// create bucket first
	createBucket("my-bucket")

	// upload object
	content := `{"hello": "world"}`
	req := httptest.NewRequest("POST", "/upload/storage/v1/b/my-bucket/o?name=test.json", bytes.NewBufferString(content))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("upload: expected status 200, got %d", w.Code)
	}

	// get object back
	req = httptest.NewRequest("GET", "/storage/v1/b/my-bucket/o/test.json", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("get: expected status 200, got %d", w.Code)
	}
	if w.Body.String() != content {
		t.Errorf("get: expected '%s', got '%s'", content, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("get: expected content-type 'application/json', got '%s'", w.Header().Get("Content-Type"))
	}
}

func TestUploadToNonExistentBucket(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest("POST", "/upload/storage/v1/b/no-such-bucket/o?name=test.txt", bytes.NewBufferString("data"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGetNonExistentObject(t *testing.T) {
	mux := setupTestServer()
	createBucket("empty-bucket")

	req := httptest.NewRequest("GET", "/storage/v1/b/empty-bucket/o/no-such-file.txt", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestListObjects(t *testing.T) {
	mux := setupTestServer()
	createBucket("list-bucket")
	uploadObject("list-bucket", "file1.txt", []byte("content1"), "text/plain")
	uploadObject("list-bucket", "file2.txt", []byte("content2"), "text/plain")

	req := httptest.NewRequest("GET", "/storage/v1/b/list-bucket/o", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response GCSListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Kind != "storage#objects" {
		t.Errorf("expected kind 'storage#objects', got '%s'", response.Kind)
	}
	if len(response.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(response.Items))
	}
}

func TestListObjectsNonExistentBucket(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest("GET", "/storage/v1/b/ghost-bucket/o", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestReadManifest(t *testing.T) {
	// create temp manifest file
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yml")
	content := `buckets:
  test-bucket:
    files:
      - path: ./data.json
        content-type: application/json
`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	manifest, err := readManifest(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	if len(manifest.Buckets) != 1 {
		t.Errorf("expected 1 bucket, got %d", len(manifest.Buckets))
	}

	bucket, ok := manifest.Buckets["test-bucket"]
	if !ok {
		t.Fatal("expected 'test-bucket' in manifest")
	}

	if len(bucket.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(bucket.Files))
	}

	if bucket.Files[0].ContentType != "application/json" {
		t.Errorf("expected content-type 'application/json', got '%s'", bucket.Files[0].ContentType)
	}
}

func TestProcessManifest(t *testing.T) {
	inMemoryStore = make(map[string]map[string]ObjectData)

	// create temp dir with a data file
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, []byte(`{"test": true}`), 0o644); err != nil {
		t.Fatalf("failed to write data file: %v", err)
	}

	manifest := &Manifest{
		Buckets: map[string]Bucket{
			"manifest-bucket": {
				Files: []File{
					{Path: dataPath, ContentType: "application/json"},
				},
			},
		},
	}

	if err := processManifest(manifest); err != nil {
		t.Fatalf("failed to process manifest: %v", err)
	}

	// check bucket exists
	bucket, ok := inMemoryStore["manifest-bucket"]
	if !ok {
		t.Fatal("bucket was not created")
	}

	// check object exists
	obj, ok := bucket["data.json"]
	if !ok {
		t.Fatal("object was not created")
	}

	if obj.ContentType != "application/json" {
		t.Errorf("expected content-type 'application/json', got '%s'", obj.ContentType)
	}

	body, _ := io.ReadAll(bytes.NewReader(obj.Data))
	if string(body) != `{"test": true}` {
		t.Errorf("unexpected object content: %s", string(body))
	}
}

func TestDefaultContentType(t *testing.T) {
	mux := setupTestServer()
	createBucket("ct-bucket")

	// upload without content-type header
	req := httptest.NewRequest("POST", "/upload/storage/v1/b/ct-bucket/o?name=binary.bin", bytes.NewBufferString("binary stuff"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// check default content type was set
	obj := inMemoryStore["ct-bucket"]["binary.bin"]
	if obj.ContentType != "application/octet-stream" {
		t.Errorf("expected default content-type 'application/octet-stream', got '%s'", obj.ContentType)
	}
}
