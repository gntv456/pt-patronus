// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package definitions

import "embed"

// FS contains the bundled site definitions used when no external definitions
// directory is available, such as single-binary deployments.
//
//go:embed *.yaml
var FS embed.FS
