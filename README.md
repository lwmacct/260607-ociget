# Web App Skeleton

## Run

```bash
go run . server
npm install
npm run dev
```

Backend listens on `:40248` by default. Vite proxies `/api` to the backend.

Open an image and materialize its filesystem:

```bash
curl -X POST http://localhost:40248/api/images \\
  -H 'content-type: application/json' \\
  -d '{"ref":"<image>","platform":"linux/amd64"}'
```

The response contains an immutable `imageId`. Browse and download files from that image:

```bash
curl "http://localhost:40248/api/images/<imageId>/entries?path=/usr/local/bin"
wget "http://localhost:40248/api/images/<imageId>/file?path=/usr/local/bin/app"
```
