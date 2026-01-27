
// Package demo provides the embedded demo HTML for WASM environments
package demo

import _ "embed"

//go:embed index.html
var HTML []byte
