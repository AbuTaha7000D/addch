package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Candidate pairs a media file that is eligible for chapter embedding with the
// exact sidecar file that must provide its chapters.
type Candidate struct {
	// MediaPath is the path to the discovered media file.
	MediaPath string
	// SidecarPath is the path to the matching "<stem>.txt" sidecar file.
	SidecarPath string
}

// supportedMediaExtensions are the containers empirically verified to carry
// embedded chapters via stream copy without re-encoding.
var supportedMediaExtensions = map[string]bool{
	".mp4": true,
	".m4v": true,
	".mov": true,
	".mkv": true,
	".m4a": true,
}

// IsSupportedMediaExt reports whether ext is one of the supported media
// extensions. The match is case-insensitive, so ".MP4", ".Mkv", and ".MOV" are
// all accepted.
func IsSupportedMediaExt(ext string) bool {
	return supportedMediaExtensions[strings.ToLower(ext)]
}

// generatedSuffixes are the suffixes the toolkit itself appends (or strips)
// when producing output files, so any file carrying them can never be a
// candidate input.
var generatedSuffixes = []string{"-chapters", "-nochapters"}

// IsGeneratedOutput reports whether path represents a generated output file:
// one whose filename stem (the name without its final extension) ends with
// "-chapters" or "-nochapters". The match is case-insensitive.
//
//	video-chapters.mp4        -> true
//	video-nochapters.mkv      -> true
//	lecture.01-chapters.mp4   -> true
//	my-course.mp4             -> false
//	طلاب-لا فصول.mp4          -> false
func IsGeneratedOutput(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	stem := strings.TrimSuffix(name, strings.ToLower(filepath.Ext(name)))
	for _, suffix := range generatedSuffixes {
		if strings.HasSuffix(stem, suffix) {
			return true
		}
	}
	return false
}

// SidecarPath derives the sidecar path for a media file by replacing its final
// extension with ".txt":
//
//	/path/to/video.mp4  -> /path/to/video.txt
//	My Course.mkv       -> My Course.txt
func SidecarPath(mediaPath string) string {
	dir := filepath.Dir(mediaPath)
	base := filepath.Base(mediaPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+".txt")
}

// isRegularFile reports whether path exists and is a regular file. Symlinks
// are followed, so a symlink resolving to a regular file is accepted.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// addCandidate appends an eligible media/sidecar pair to candidates. The file
// must already have satisfied the supported-extension and non-generated checks;
// here we additionally require that the sidecar exists as a regular file.
func addCandidate(candidates []Candidate, media string, sidecar string) []Candidate {
	if isRegularFile(sidecar) {
		return append(candidates, Candidate{MediaPath: media, SidecarPath: sidecar})
	}
	return candidates
}

// FindCandidates discovers media/video files in dir that have an existing
// "<stem>.txt" sidecar. When recursive is false, only the files directly inside
// dir are inspected; when true, the whole directory tree rooted at dir is
// walked. Subdirectories are never treated as candidates, generated outputs
// (filenames ending in "-chapters" or "-nochapters") are unconditionally
// excluded, and the returned slice is sorted lexicographically by MediaPath so
// execution order is deterministic across platforms.
func FindCandidates(dir string, recursive bool) ([]Candidate, error) {
	if recursive {
		return findCandidatesRecursive(dir)
	}
	return findCandidatesShallow(dir)
}

func findCandidatesShallow(dir string) ([]Candidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var candidates []Candidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		media := filepath.Join(dir, entry.Name())
		if !candidateMedia(media) {
			continue
		}
		candidates = addCandidate(candidates, media, SidecarPath(media))
	}
	return sortCandidates(candidates), nil
}

func findCandidatesRecursive(dir string) ([]Candidate, error) {
	var candidates []Candidate
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !candidateMedia(path) {
			return nil
		}
		candidates = addCandidate(candidates, path, SidecarPath(path))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sortCandidates(candidates), nil
}

// candidateMedia reports whether path is a regular file eligible for discovery:
// it must have a supported media extension and must not be a generated output.
func candidateMedia(path string) bool {
	return isRegularFile(path) &&
		IsSupportedMediaExt(filepath.Ext(path)) &&
		!IsGeneratedOutput(path)
}

func sortCandidates(candidates []Candidate) []Candidate {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].MediaPath < candidates[j].MediaPath
	})
	return candidates
}
