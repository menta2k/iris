# API contract & Swagger UI

The Iris backend serves its generated OpenAPI 3 document at:

```
GET /openapi.yaml
```

So with the backend running on its default port (`:8080`):

```
http://localhost:8080/openapi.yaml
```

## Where the contract comes from

The OpenAPI document is **generated**, not hand-written. The source of truth is
the protobuf service definition:

```
backend/api/iris/admin/v1/iris-admin-api.proto
```

The generated artifacts (including `openapi.yaml`) live next to it under
`backend/api/iris/admin/v1/` and are produced by the backend's code-generation
toolchain (`buf` — see `backend/buf.gen.yaml` and the `backend/Makefile`). The
backend embeds the generated `openapi.yaml` and serves it at `/openapi.yaml`.

Because the spec is generated from the proto, treat the `.proto` file as the
place to make API changes; regenerate to update the served document.

## Viewing it in Swagger UI

Run the standalone Swagger UI container and point it at the live document:

```sh
docker run --rm -p 8081:8080 \
  -e SWAGGER_JSON_URL=http://localhost:8080/openapi.yaml \
  swaggerapi/swagger-ui
```

Then open <http://localhost:8081> in a browser.

Notes:

- The backend must be running and reachable at `http://localhost:8080`.
- If you are on Linux and the backend runs on the host (not in Docker), you may
  need `--add-host=host.docker.internal:host-gateway` and use
  `http://host.docker.internal:8080/openapi.yaml` so the Swagger UI container can
  reach the host. On Docker Desktop (macOS/Windows) `host.docker.internal`
  resolves automatically.
- Alternatively, paste the URL `http://localhost:8080/openapi.yaml` into the
  "Explore" bar of any hosted Swagger UI, or open the saved
  `backend/api/iris/admin/v1/openapi.yaml` file directly in an editor that
  renders OpenAPI.
