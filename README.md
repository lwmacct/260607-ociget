# OCIGet

OCIGet is a web UI and HTTP API for OCI images and artifacts. It browses OCI/Docker image filesystems and presents OCI artifact payloads as directly downloadable blobs.

The local store contains metadata only. It does not keep layer blobs or extracted file contents, so the source registry (or a registry proxy such as Harbor) must be reachable when a file is downloaded.

## Quick Start

Start the Go API server:

```bash
go run . server
```

In a second terminal, install frontend dependencies and start Vite:

```bash
pnpm install
pnpm dev
```

Open <http://localhost:40249>. By default:

- the API server listens on `:40248`;
- Vite serves the frontend on `:40249`;
- Vite proxies `/api` and direct image download URLs to the API server.

The production frontend can be built with `pnpm build`. When serving the generated files with the Go server, set `FRONTEND_DIR` to the build directory, for example `FRONTEND_DIR=dist go run . server`.

## Browser

The default OCI reference in the browser is:

```text
ghcr.io/lwmacct/260607-ociget:latest
```

The browser supports:

- image and artifact references from Docker Hub, GHCR, Harbor, or another OCI-compatible registry;
- platform selection such as `linux/amd64` or `linux/arm64`;
- platform discovery before opening an image;
- directory browsing with file metadata, permissions, and timestamps;
- single-file downloads and multi-file tar downloads;
- OCI artifact metadata and direct downloads of every payload descriptor;
- registries that require the insecure HTTP option.

Images and artifacts have separate views. Image layers are interpreted as filesystem tar archives. Artifact payloads retain their registry bytes and are never treated as directories or decompressed by OCIGet. ORAS file names are read from the `org.opencontainers.image.title` annotation.

## Direct Downloads

The compact root URL format is:

```text
/<image-ref>/-/<path>
```

The first `/-/` separates the complete image reference from the path inside the image. For example:

```bash
wget "http://localhost:40248/ghcr.io/lwmacct/260607-ociget:latest/-/usr/local/bin/app"
```

A registry proxy is expressed as part of the image reference. For example:

```bash
wget "http://localhost:40248/harbor.example.com/ghcr.io/lwmacct/260607-ociget:latest/-/usr/local/bin/app"
```

Supported query parameters are:

| Parameter | Example | Description |
| --- | --- | --- |
| `platform` | `linux/amd64` | Select an image platform. |
| `insecure` | `1` | Allow an insecure registry connection. |
| `refresh` | `1` | Resolve a mutable tag again instead of using its cached reference mapping. |

The download endpoint supports `GET`, `HEAD`, byte `Range` requests, `ETag`, and `Last-Modified` headers.

## HTTP API

Interactive OpenAPI documentation is available at <http://localhost:40248/api/docs>, with the machine-readable specification at <http://localhost:40248/api/openapi.json>.

### Resolve an OCI object

`POST /api/objects` resolves an image or artifact and creates an immutable metadata index. The response contains `kind` (`image` or `artifact`) and an `objectId` bound to the reference, platform, registry access mode, and resolved manifest digest. Artifact responses also contain `artifactType`, annotations, and payload descriptors.

```bash
curl -X POST http://localhost:40248/api/objects \
  -H 'content-type: application/json' \
  -d '{
    "ref": "ghcr.io/lwmacct/260607-ociget:latest",
    "platform": "linux/amd64"
  }'
```

Use `"refresh": true` to resolve a mutable tag again. Set `"insecure": true` for an insecure registry.

### List a directory

```bash
curl "http://localhost:40248/api/objects/<objectId>/entries?path=/usr/local/bin"
```

### Download one file

```bash
curl -OJ \
  "http://localhost:40248/api/objects/<objectId>/file?path=/usr/local/bin/app"
```

### Download several files as tar

```bash
curl -X POST \
  -H 'content-type: application/json' \
  -d '{"paths":["/usr/local/bin/app","/etc/os-release"]}' \
  -o image-files.tar \
  "http://localhost:40248/api/objects/<objectId>/archive"
```

### Download an artifact payload

Use a digest returned in the artifact object's `blobs` array. The digest is checked against the immutable object index before OCIGet opens the registry blob.

```bash
curl -OJ \
  "http://localhost:40248/api/objects/<objectId>/blobs/<blobDigest>"
```

Artifact downloads support `GET`, `HEAD`, byte `Range` requests, `ETag`, registry media types, and annotation-derived filenames.

### System endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/health` | Service health and version. |
| `GET` | `/api/meta` | Service name, version, listen address, and docs path. |
| `GET` | `/api/objects/platforms?ref=<oci-ref>` | List platforms published by an image. |

## Metadata Store

The default metadata directory is `.local/oci-metadata`:

```text
.local/oci-metadata/
├── objects/            # Immutable image or artifact metadata indexes
├── locks/              # Per-object build locks
├── staging/            # Temporary metadata builds
└── object-refs.json    # Reference-to-objectId mappings
```

The store never contains layer, extracted file, or artifact payload content. A cached index can be reused for browsing, but every download reopens the corresponding layer or blob from the recorded registry reference.

`oci-store.ref-ttl` controls how often a mutable reference such as `:latest` is resolved again. A digest-pinned reference is naturally stable; use `refresh=1` when an immediate re-resolution is required.

## Configuration

Copy `config/config.example.yaml` to `config/config.yaml` and adjust the server settings:

```yaml
server:
  http:
    listen: ":40248"
    tls:
      enabled: false
  oci-store:
    dir: ".local/oci-metadata"
    ref-ttl: "5m"
```

The equivalent CLI options are shown by `go run . server --help`. The relevant options are:

```text
--http.listen
--http.tls.*
--oci-store.dir
--oci-store.ref-ttl
```

## Build and Docker

Build the frontend and backend:

```bash
pnpm build
go build -o bin/app .
```

The supplied `Dockerfile` expects `bin/app` and `dist` to exist before the image is built. The container serves the frontend from `/app/web` and stores runtime data below `/app/data`.

Run the server explicitly in a built image:

```bash
docker run --rm -p 40248:40248 your-image app server
```

## Development Checks

```bash
go test ./...
go test -race ./...
pnpm test
pnpm build
git diff --check
```
