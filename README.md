# Web App Skeleton

## Run

```bash
go run . server
npm install
npm run dev
```

Backend listens on `:40248` by default. Vite proxies `/api` to the backend.

Open an image and build a local filesystem metadata index:

```bash
curl -X POST http://localhost:40248/api/images \\
  -H 'content-type: application/json' \\
  -d '{"ref":"<image>","platform":"linux/amd64"}'
```

The response contains an immutable `imageId`. Browse files from that metadata index; file bytes are read from the referenced registry when downloaded:

```bash
curl "http://localhost:40248/api/images/<imageId>/entries?path=/usr/local/bin"
```

For direct downloads, use the compact root URL form. The first `/-/` separates the OCI image reference from the file path:

```bash
wget "http://localhost:40248/ghcr.io/lwmacct/260607-ociget:latest/-/usr/local/bin/app"
```

Optional query parameters are `platform=linux/amd64`, `insecure=1`, and `refresh=1` (to resolve a mutable tag again).
