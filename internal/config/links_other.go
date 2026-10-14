//go:build !unix

package config

import "io/fs"

// linkCount reports a single link where the platform doesn't expose one.
func linkCount(fs.FileInfo) uint64 { return 1 }
