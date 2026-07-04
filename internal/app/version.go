// Package app app carries build-time version metadata injected via -ldflags.
package app

const (
	BinaryName  = "servy"
	ProductName = "Servy"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
