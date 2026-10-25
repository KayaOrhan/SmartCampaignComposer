package docs

import _ "embed"

// OpenAPI is the canonical specification, served without runtime filesystem access.
//
//go:embed openapi.json
var OpenAPI string
