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

// DefaultNoChaptersOutputPath derives the default output path for the rmch
// command from the input media path. It mirrors DefaultOutputPath exactly but
// inserts "-nochapters" before the extension:
//
//	My Course.mp4  ->  My Course-nochapters.mp4
//	video.mkv      ->  video-nochapters.mkv
//	lecture.01.mp4 ->  lecture.01-nochapters.mp4
//
// Files whose name has no extension (e.g. "VIDEO") get "-nochapters" appended.
// The path is normalized with filepath.Clean. The addch counterpart
// (DefaultOutputPath) keeps the existing "-chapters" suffix.
func DefaultNoChaptersOutputPath(inputPath string) string {
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	newname := stem + "-nochapters" + ext
	return filepath.Join(dir, newname)
}

// OutputExtension returns the lowercase extension (including the dot) of a path,
// or "" if the path has no extension.
func OutputExtension(p string) string {
	return strings.ToLower(filepath.Ext(p))
}

// CleanupFile removes a (possibly partial) output file, safely ignoring errors
// when the file does not exist. Directories are never removed: an adversarial
// --overwrite run whose output path is an existing directory must fail cleanly
// without deleting the user's directory.
func CleanupFile(path string) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// Deliberately ignore cleanup errors; the file may belong to another process.
		_ = err
	}
}

// WriteFileAtomic writes data to path atomically: the content is first written
// to a uniquely named temporary file created inside path's directory, then
// renamed over path. Rename within the same directory is atomic, so readers
// never observe a partially written file and an existing file is either fully
// replaced or left untouched. The temporary file starts with os.CreateTemp's
// exclusive 0600 mode and is chmod'ed to 0644 before the rename so the result
// is a readable text file on disk; it is removed if any step before the rename
// fails. A missing parent directory makes os.CreateTemp fail, so nothing is
// written (a getch sidecar never creates the directory it targets).
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".addch-sidecar-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = "" // renamed into place; the deferred removal has nothing to do
	return nil
}
