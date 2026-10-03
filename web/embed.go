// Package web exposes the built frontend to the Go server.
//
// The frontend build writes its Vite output to dist/. Keeping the embed point
// in this small package lets the server stay independent of the frontend
// implementation and also makes `go run ./cmd/pocketfuse` work from a checkout
// that only contains the placeholder page.
package web

import "embed"

// Dist contains the static frontend build.
//
//go:embed dist
var Dist embed.FS
