package assets

import "embed"

// Explicitly include __init__.py files (Go's embed excludes files starting with _)
//
//go:embed stellar-client
//go:embed stellar-client/stellar_client/__init__.py
//go:embed stellar-client/stellar_client/models/__init__.py
//go:embed stellar-client/stellar_client/resources/__init__.py
//go:embed stellar-client/stellar_client/utils/__init__.py
var Assets embed.FS
