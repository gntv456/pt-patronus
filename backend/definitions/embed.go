package definitions

import "embed"

// FS contains the bundled site definitions used when no external definitions
// directory is available, such as single-binary deployments.
//
//go:embed *.yaml
var FS embed.FS
