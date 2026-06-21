package adminv1

import _ "embed"

// OpenAPISpec is the generated OpenAPI document for the admin HTTP API, served
// at /openapi.yaml for Swagger UI and client tooling.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
