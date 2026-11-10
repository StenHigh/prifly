// Package examples carries the same operational references used in the repository
// into installed runners. It contains no authority or workflow execution code.
package examples

import "embed"

// Handbook includes the worked examples needed by the references. Programs in
// examples are data here; installing the handbook never executes them.
//
//go:embed operations/*.md authoring compile-parameters workflows workflow-composition nested-extension-points unified-composition light README.md troubleshooting.md
var Handbook embed.FS
