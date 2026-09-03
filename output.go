package main

import (
	"path/filepath"
	"strings"
)

// defaultOutputPath derives the default output path from the input video path.
//
// The output is placed in the same directory as the input, keeps the base name
// and extension, and inserts "-chapters" before the extension:
//
//	My Course.mp4  ->  My Course-chapters.mp4
//	video.mkv      ->  video-chapters.mkv
//	lecture.01.mp4 ->  lecture.01-chapters.mp4
//
// Files whose name has no extension (e.g. "VIDEO") get "-chapters" appended.
// The path is normalized with filepath.Clean.
func defaultOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	newname := stem + "-chapters" + ext
	return filepath.Join(dir, newname)
}

// outputExtension returns the lowercase extension (including the dot) of a path,
// or "" if the path has no extension.
func outputExtension(p string) string {
	return strings.ToLower(filepath.Ext(p))
}

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
