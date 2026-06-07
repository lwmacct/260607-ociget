# Web App Skeleton

## Run

```bash
go run . server
npm install
npm run dev
```

Backend listens on `:40248` by default. Vite proxies `/api` to the backend.

Download a file from an image:

```bash
wget "http://localhost:40248/download?image=<image>&path=<path>"
```

For multi-platform images, the default platform is `linux/amd64`. Override it with:

```bash
wget "http://localhost:40248/download?image=<image>&path=<path>&platform=linux/arm64"
```
