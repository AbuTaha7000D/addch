package fsutil

import (
	"fmt"
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
// embedded chapters via stream copy without re-encoding. .mov and .m4v are
// deliberately absent: addch embeds chapters into both (verified with real
// FFmpeg/FFprobe, Phase 8.5.3), but rmch cannot strip them because the mov and
// ipod muxers reject the residual QuickTime text chapter track with "Tag text
// incompatible with output codec id '98314'" (rc 183), so a chaptered file
// never round-trips through the removal path.
var supportedMediaExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".m4a": true,
}

// IsSupportedMediaExt reports whether ext is one of the supported media
// extensions. The match is case-insensitive, so ".MP4", ".Mkv", and ".M4A" are
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
//	unicode-nochapters.mp4    -> false
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
// execution order is deterministic across platforms. A recursive walk that hits
// an entry-level traversal error (e.g. an unreadable subdirectory) aborts and
// returns the error rather than yielding an apparently-complete partial
// discovery.
func FindCandidates(dir string, recursive bool) ([]Candidate, error) {
	var paths []string
	if err := walkMediaFiles(dir, recursive, func(p string) { paths = append(paths, p) }); err != nil {
		return nil, err
	}
	var candidates []Candidate
	for _, media := range paths {
		candidates = addCandidate(candidates, media, SidecarPath(media))
	}
	return sortCandidates(candidates), nil
}

// FindMediaFiles discovers supported-media, non-generated, regular files under
// dir WITHOUT requiring a sidecar, sorted lexicographically. Shallow vs
// recursive behavior is identical to FindCandidates: when recursive is false
// only the files directly inside dir are inspected, when true the whole tree is
// walked, subdirectories are never candidates, and generated outputs (filenames
// ending in "-chapters" or "-nochapters") are unconditionally excluded. This is
// the discovery entry point for rmch, which has no sidecar concept. A recursive
// walk that hits an entry-level traversal error (e.g. an unreadable
// subdirectory) aborts and returns the error rather than yielding an
// apparently-complete partial discovery.
func FindMediaFiles(dir string, recursive bool) ([]string, error) {
	var out []string
	if err := walkMediaFiles(dir, recursive, func(p string) { out = append(out, p) }); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// walkMediaFiles visits every media candidate under dir — a regular file with a
// supported extension that is not a generated output — shallow or recursive,
// calling visit for each. Subdirectories are skipped in both modes. The shared
// traversal is what keeps the two discovery flavors' ordering and filtering
// identical. The root dir is validated up front so a missing or non-directory
// batch root is reported as an error instead of silently succeeding with zero
// results. An entry-level traversal error during a recursive walk (e.g. an
// unreadable subdirectory) aborts discovery and surfaces as an error, so an
// incomplete traversal is never presented as a successful, complete one.
func walkMediaFiles(dir string, recursive bool, visit func(path string)) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	if recursive {
		walkRoot := dir
		// A batch root that is itself a symlink to a directory must be resolved
		// before walking: filepath.WalkDir Lstats the root entry and treats a
		// symlink as a non-directory, so an unresolved symlink root would be
		// silently reported as empty. Resolving it makes recursion descend into
		// the target; reported paths then appear in the resolved (real) form.
		// Non-symlink roots are walked exactly as before.
		if li, err := os.Lstat(dir); err == nil && li.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(dir)
			if err != nil {
				return fmt.Errorf("cannot resolve symlinked batch root %q: %w", dir, err)
			}
			walkRoot = resolved
		}
		return filepath.WalkDir(walkRoot, walkVisit(visit))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if candidateMedia(path) {
			visit(path)
		}
	}
	return nil
}

// walkVisit builds the recursive traversal callback used by walkMediaFiles. It
// enforces the traversal-error policy: an error for an entry (for example an
// unreadable subdirectory) aborts the walk by returning a descriptive error —
// filepath.WalkDir stops and propagates it, so an incomplete traversal is
// never reported to the caller as a complete discovery. Directories are not
// candidates; every other regular, supported, non-generated file is forwarded
// to visit. The error check precedes the directory-skip so a failed
// directory is reported rather than glossed over.
func walkVisit(visit func(path string)) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("cannot traverse \"%s\": %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		if candidateMedia(path) {
			visit(path)
		}
		return nil
	}
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
