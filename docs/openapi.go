package docs

import _ "embed"

// OpenAPIV2 is the current authenticated Watch and Studio API contract.
//
//go:embed openapi-v2.yaml
var OpenAPIV2 string
