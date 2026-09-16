# Dun Gateway Example

This is the first complete local Dun Gateway scenario:

```text
curl -> Dun :8080 -> security-header Wasm plugin -> backend :3000
```

## Run it

From the repository root, use two terminals.

Terminal 1:

```bash
go run ./examples/gateway/backend
```

Terminal 2:

```bash
go run ./cmd/dun check -config examples/gateway/dun.yaml
go run ./cmd/dun plugin validate -config examples/gateway/dun.yaml
go run ./cmd/dun run -config examples/gateway/dun.yaml
```

Terminal 3:

```bash
curl -i http://localhost:8080/api/hello
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

The response should include:

- `X-Security-Policy: default-src 'self'` from the Wasm extension;
- `X-Frame-Options: DENY` from the route configuration;
- `X-Content-Type-Options: nosniff` from the route configuration;
- a JSON response from the backend.

The configuration pins the security-header artifact with SHA-256. Run
`go run ./cmd/dun plugin inspect -config examples/gateway/dun.yaml` to inspect
its ABI, digest and declared capabilities.
