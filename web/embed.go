// Package web exposes the built frontend to the Go server.
//
// The frontend build writes its Vite output to dist/. Keeping the embed point
// in this small package lets the server stay independent of the frontend
// implementation. dist/ is gitignored, so CI Go jobs stub it before building
// (see "Stub embedded frontend" in ci.yml); run `make frontend-build` to
// populate it for real binaries.
package web

import "embed"

// Dist contains the static frontend build.
//
//go:embed all:dist
var Dist embed.FS
