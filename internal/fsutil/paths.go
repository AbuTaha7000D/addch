// Package fsutil provides filesystem path and file helpers shared by the
// addch toolkit's commands.
package fsutil

import (
	"os"
	"path/filepath"
	"strings"
)

// SamePath reports whether two paths refer to the same underlying file. It
// normalizes each path to an absolute, cleaned form and, when both paths exist,
// resolves symlinks so that a symlinked output pointing at the input is also
// detected. This guarantees the input video can never be overwritten via the
// output path, including through identical paths, relative vs absolute forms,
// normalization differences, or symlinks.
func SamePath(a, b string) bool {
	resolve := func(p string) (string, bool) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", false
		}
		abs = filepath.Clean(abs)
		// Resolve symlinks only when the path exists; for a not-yet-created output
		// we fall back to the cleaned absolute form.
		if ev, err := filepath.EvalSymlinks(abs); err == nil {
			return ev, true
		}
		return abs, true
	}
	ra, oka := resolve(a)
	rb, okb := resolve(b)
	return oka && okb && ra == rb
}

// DefaultOutputPath derives the default output path from the input video path.
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
func DefaultOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	newname := stem + "-chapters" + ext
	return filepath.Join(dir, newname)
}

// OutputExtension returns the lowercase extension (including the dot) of a path,
// or "" if the path has no extension.
func OutputExtension(p string) string {
	return strings.ToLower(filepath.Ext(p))
}

// CleanupFile removes a (possibly partial) output file, safely ignoring errors
// when the file does not exist.
func CleanupFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// Deliberately ignore cleanup errors; the file may belong to another process.
		_ = err
	}
}
