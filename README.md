# GCS Mock Service

A lightweight mock server for Google Cloud Storage. Currently it only supports basic store and list operations. The service stores everything in memory, so data is lost when the container stops.

## Running it

By default it runs on `0.0.0.0:4443`, so this just works:
```bash
docker run -p 4443:4443 shourieg/gcs-mock-service:latest
```

Change the port if you need to:
```bash
docker run -p 8080:8080 shourieg/gcs-mock-service:latest ./gcs-mock-service --port 8080
```

Or bind to a specific host:
```bash
docker run -p 4443:4443 shourieg/gcs-mock-service:latest ./gcs-mock-service --host 127.0.0.1
```

Health check:
```bash
curl http://localhost:4443/health
```

## Using the API

Create a bucket:
```bash
curl -X POST http://localhost:4443/storage/v1/b \
  -H "Content-Type: application/json" \
  -d '{"name": "my-bucket"}'
```

Upload something:
```bash
curl -X POST "http://localhost:4443/upload/storage/v1/b/my-bucket/o?name=test.json" \
  -H "Content-Type: application/json" \
  -d '{"foo": "bar"}'
```

Get it back:
```bash
curl http://localhost:4443/storage/v1/b/my-bucket/o/test.json
```

List everything in a bucket:
```bash
curl http://localhost:4443/storage/v1/b/my-bucket/o
```

## Preloading data with a manifest

Sometimes you want data ready before your tests run. Create a `manifest.yml`:

```yaml
buckets:
  test-bucket:
    files:
      - path: ./fixtures/config.json
        content-type: application/json
      - path: ./fixtures/data.csv
        content-type: text/csv
```

Then mount it:
```bash
docker run -p 4443:4443 \
  -v $(pwd)/manifest.yml:/app/manifest.yml \
  -v $(pwd)/fixtures:/app/fixtures \
  shourieg/gcs-mock-service:latest \
  ./gcs-mock-service --manifest /app/manifest.yml
```

The object name in GCS will be the filename (last part of the path).

## Docker Compose

Basic setup:

```yaml
services:
  gcs-mock:
    image: shourieg/gcs-mock-service:latest
    ports:
      - "4443:4443"
```

With manifest and custom port:

```yaml
services:
  gcs-mock:
    image: shourieg/gcs-mock-service:latest
    ports:
      - "9000:9000"
    volumes:
      - ./manifest.yml:/app/manifest.yml
      - ./fixtures:/app/fixtures
    command: ["./gcs-mock-service", "--manifest", "/app/manifest.yml", "--port", "9000"]

  your-app:
    build: .
    environment:
      - GCS_ENDPOINT=http://gcs-mock:9000
    depends_on:
      - gcs-mock
```

## CLI flags

- `--host` - defaults to `0.0.0.0`
- `--port` - defaults to `4443`
- `--manifest` - path to manifest file (optional)

## What's not supported

No auth, no resumable uploads, no versioning, no ACLs. It's just a basic mock for now.
