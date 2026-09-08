package main

import (
	"strings"
)

// isMP4Family reports whether an output extension is part of the MP4 family of
// containers that support the +faststart flag.
func isMP4Family(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov", ".3gp", ".3g2":
		return true
	default:
		return false
	}
}
