package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsSupportedMediaExt(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".mp4", true},
		{".MP4", true},
		{".m4v", true},
		{".MOV", true},
		{".mkv", true},
		{".m4a", true},
		{".avi", false},
		{".flv", false},
		{".wav", false},
		{".txt", false},
		{"mp4", false},
		{"", false},
		{".mp3", false},
	}
	for _, c := range cases {
		if got := IsSupportedMediaExt(c.ext); got != c.want {
			t.Errorf("IsSupportedMediaExt(%q) = %v, want %v", c.ext, got, c.want)
		}
	}
}

func TestIsGeneratedOutput(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"video-chapters.mp4", true},
		{"video-nochapters.mkv", true},
		{"lecture.01-chapters.mp4", true},
		{"my-course.mp4", false},
		{"Video-CHAPTERS.mp4", true},
		{"video-noCHApTers.MKV", true},
		{"طلاب.mp4", false},
		{"notes-chapters.txt", true},
		{"chaptering.mp4", false},
	}
	for _, c := range cases {
		if got := IsGeneratedOutput(c.path); got != c.want {
			t.Errorf("IsGeneratedOutput(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestSidecarPath(t *testing.T) {
	cases := []struct {
		media, want string
	}{
		{filepath.Join("dir", "video.mp4"), filepath.Join("dir", "video.txt")},
		{filepath.Join("dir", "My Course.mkv"), filepath.Join("dir", "My Course.txt")},
		{filepath.Join("a", "b", "lecture.01.MOV"), filepath.Join("a", "b", "lecture.01.txt")},
		{"single.m4a", "single.txt"},
	}
	for _, c := range cases {
		if got := SidecarPath(c.media); got != c.want {
			t.Errorf("SidecarPath(%q) = %q, want %q", c.media, got, c.want)
		}
	}
}

func TestFindCandidatesShallow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mp4"))
	writeFile(t, filepath.Join(dir, "a.txt"))
	writeFile(t, filepath.Join(dir, "b.mkv"))
	writeFile(t, filepath.Join(dir, "b.txt"))
	// Video in a subdirectory must be ignored in shallow mode.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "nested.mp4"))
	writeFile(t, filepath.Join(sub, "nested.txt"))
	// Non-media file with a sidecar must be ignored.
	writeFile(t, filepath.Join(dir, "notes.txt"))
	writeFile(t, filepath.Join(dir, "notes.pdf"))

	candidates, err := FindCandidates(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(candidates), candidates)
	}
	want := []Candidate{
		{MediaPath: filepath.Join(dir, "a.mp4"), SidecarPath: filepath.Join(dir, "a.txt")},
		{MediaPath: filepath.Join(dir, "b.mkv"), SidecarPath: filepath.Join(dir, "b.txt")},
	}
	for i, c := range candidates {
		if c != want[i] {
			t.Errorf("candidate[%d] = %+v, want %+v", i, c, want[i])
		}
	}
}

func TestFindCandidatesRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "top.mp4"))
	writeFile(t, filepath.Join(dir, "top.txt"))
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nested, "deep.mov"))
	writeFile(t, filepath.Join(nested, "deep.txt"))
	deep := filepath.Join(nested, "deeper")
	if err := os.Mkdir(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(deep, "deepest.m4a"))
	writeFile(t, filepath.Join(deep, "deepest.txt"))
	// No sidecar -> excluded.
	writeFile(t, filepath.Join(nested, "orphan.mkv"))

	candidates, err := FindCandidates(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 {
		t.Fatalf("got %d candidates, want 3: %+v", len(candidates), candidates)
	}
}

func TestFindCandidatesGeneratedExclusion(t *testing.T) {
	dir := t.TempDir()
	// Generated outputs with valid sidecars must still be excluded.
	writeFile(t, filepath.Join(dir, "vid-chapters.mp4"))
	writeFile(t, filepath.Join(dir, "vid-chapters.txt"))
	writeFile(t, filepath.Join(dir, "vid-nochapters.mkv"))
	writeFile(t, filepath.Join(dir, "vid-nochapters.txt"))
	writeFile(t, filepath.Join(dir, "lecture.01-chapters.m4v"))
	writeFile(t, filepath.Join(dir, "lecture.01-chapters.txt"))
	// A normal file must survive.
	writeFile(t, filepath.Join(dir, "keep.mp4"))
	writeFile(t, filepath.Join(dir, "keep.txt"))

	candidates, err := FindCandidates(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].MediaPath != filepath.Join(dir, "keep.mp4") {
		t.Fatalf("got %+v, want only keep.mp4", candidates)
	}
}

func TestFindCandidatesMissingSidecar(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "no-sidecar.mp4"))
	writeFile(t, filepath.Join(dir, "with-sidecar.mkv"))
	writeFile(t, filepath.Join(dir, "with-sidecar.txt"))

	candidates, err := FindCandidates(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].MediaPath != filepath.Join(dir, "with-sidecar.mkv") {
		t.Fatalf("got %+v, want only with-sidecar.mkv", candidates)
	}
}

func TestFindCandidatesOrphanSidecar(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "orphan.txt"))

	candidates, err := FindCandidates(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0", len(candidates))
	}
}

func TestFindCandidatesUnsupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old.avi"))
	writeFile(t, filepath.Join(dir, "old.txt"))
	writeFile(t, filepath.Join(dir, "flash.flv"))
	writeFile(t, filepath.Join(dir, "flash.txt"))
	writeFile(t, filepath.Join(dir, "audio.wav"))
	writeFile(t, filepath.Join(dir, "audio.txt"))
	writeFile(t, filepath.Join(dir, "https.mp4"))
	writeFile(t, filepath.Join(dir, "https.txt"))

	candidates, err := FindCandidates(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].MediaPath != filepath.Join(dir, "https.mp4") {
		t.Fatalf("got %+v, want only https.mp4", candidates)
	}
}

func TestFindCandidatesUnicodeAndSpaces(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "فيديو تجريبي.mkv"))
	writeFile(t, filepath.Join(dir, "فيديو تجريبي.txt"))
	writeFile(t, filepath.Join(dir, "دورة قواعد.mp4"))
	writeFile(t, filepath.Join(dir, "دورة قواعد.txt"))

	candidates, err := FindCandidates(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(candidates), candidates)
	}
	if candidates[0].SidecarPath != filepath.Join(dir, "دورة قواعد.txt") ||
		candidates[1].SidecarPath != filepath.Join(dir, "فيديو تجريبي.txt") {
		t.Errorf("unexpected sidecars: %+v", candidates)
	}
}

func TestFindCandidatesDeterministicOrdering(t *testing.T) {
	dir := t.TempDir()
	names := []string{"z.mp4", "a.mp4", "m.mkv", "b.mp4"}
	for _, n := range names {
		stem := n[:len(n)-4]
		writeFile(t, filepath.Join(dir, n))
		writeFile(t, filepath.Join(dir, stem+".txt"))
	}

	run := func() []Candidate {
		candidates, err := FindCandidates(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		return candidates
	}

	first := run()
	if len(first) != 4 {
		t.Fatalf("got %d candidates, want 4", len(first))
	}
	want := []string{"a.mp4", "b.mp4", "m.mkv", "z.mp4"}
	for i, c := range first {
		if filepath.Base(c.MediaPath) != want[i] {
			t.Errorf("candidate[%d] = %s, want %s", i, filepath.Base(c.MediaPath), want[i])
		}
	}
	// Re-running must be stable.
	second := run()
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("unstable ordering across runs: %+v vs %+v", first, second)
		}
	}
}

func TestFindCandidatesMissingDir(t *testing.T) {
	if _, err := FindCandidates(filepath.Join(t.TempDir(), "absent"), false); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func TestFindMediaFilesShallow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.mp4"))
	writeFile(t, filepath.Join(dir, "a.txt"))
	writeFile(t, filepath.Join(dir, "b.mkv"))
	writeFile(t, filepath.Join(dir, "b.txt"))
	// Media in a subdirectory must be ignored in shallow mode.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "nested.mp4"))
	// Non-media files and sidecars alone must be ignored.
	writeFile(t, filepath.Join(dir, "notes.txt"))
	writeFile(t, filepath.Join(dir, "notes.pdf"))
	// Unsupported extension must be ignored even without a sidecar.
	writeFile(t, filepath.Join(dir, "old.avi"))

	files, err := FindMediaFiles(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d media files, want 2: %+v", len(files), files)
	}
	want := []string{filepath.Join(dir, "a.mp4"), filepath.Join(dir, "b.mkv")}
	for i, f := range files {
		if f != want[i] {
			t.Errorf("media[%d] = %s, want %s", i, f, want[i])
		}
	}
}

func TestFindMediaFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "top.mp4"))
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nested, "deep.mov"))
	deep := filepath.Join(nested, "deeper")
	if err := os.Mkdir(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(deep, "deepest.m4a"))
	// Sidecar-like files and unsupported extensions never count, recursive or not.
	writeFile(t, filepath.Join(nested, "notes.txt"))

	files, err := FindMediaFiles(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d media files, want 3: %+v", len(files), files)
	}
	want := []string{
		filepath.Join(nested, "deep.mov"),
		filepath.Join(deep, "deepest.m4a"),
		filepath.Join(dir, "top.mp4"),
	}
	for i, f := range files {
		if f != want[i] {
			t.Errorf("media[%d] = %s, want %s", i, f, want[i])
		}
	}
}

func TestFindMediaFilesGeneratedExclusion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "vid-chapters.mp4"))
	writeFile(t, filepath.Join(dir, "vid-nochapters.mkv"))
	writeFile(t, filepath.Join(dir, "lecture.01-chapters.m4v"))
	// A normal file must survive.
	writeFile(t, filepath.Join(dir, "keep.mp4"))

	files, err := FindMediaFiles(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != filepath.Join(dir, "keep.mp4") {
		t.Fatalf("got %+v, want only keep.mp4", files)
	}
}

func TestFindMediaFilesUnsupportedExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old.avi"))
	writeFile(t, filepath.Join(dir, "flash.flv"))
	writeFile(t, filepath.Join(dir, "audio.wav"))
	writeFile(t, filepath.Join(dir, "https.mp4"))

	files, err := FindMediaFiles(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != filepath.Join(dir, "https.mp4") {
		t.Fatalf("got %+v, want only https.mp4", files)
	}
}

func TestFindMediaFilesUnicodeAndSpaces(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "فيديو تجريبي.mkv"))
	writeFile(t, filepath.Join(dir, "دورة قواعد.mp4"))

	files, err := FindMediaFiles(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d media files, want 2: %+v", len(files), files)
	}
	if files[0] != filepath.Join(dir, "دورة قواعد.mp4") ||
		files[1] != filepath.Join(dir, "فيديو تجريبي.mkv") {
		t.Errorf("unexpected ordering: %+v", files)
	}
}

func TestFindMediaFilesDeterministicOrdering(t *testing.T) {
	dir := t.TempDir()
	names := []string{"z.mp4", "a.mp4", "m.mkv", "b.mp4"}
	for _, n := range names {
		writeFile(t, filepath.Join(dir, n))
	}

	run := func() []string {
		files, err := FindMediaFiles(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		return files
	}

	first := run()
	want := []string{"a.mp4", "b.mp4", "m.mkv", "z.mp4"}
	for i, f := range first {
		if filepath.Base(f) != want[i] {
			t.Errorf("media[%d] = %s, want %s", i, filepath.Base(f), want[i])
		}
	}
	second := run()
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("unstable ordering across runs: %+v vs %+v", first, second)
		}
	}
}

func TestFindMediaFilesMissingDir(t *testing.T) {
	if _, err := FindMediaFiles(filepath.Join(t.TempDir(), "absent"), false); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func TestFindMediaFilesDirectoryIsNotCandidate(t *testing.T) {
	dir := t.TempDir()
	// A directory with a media-like name must not be returned.
	if err := os.Mkdir(filepath.Join(dir, "video.mp4"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := FindMediaFiles(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("media-named directory must not be a candidate, got %+v", files)
	}
}

func TestFindCandidatesSymlinkMedia(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.mp4")
	writeFile(t, real)
	link := filepath.Join(dir, "linked.mp4")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	// The sidecar is derived from the link name and must exist on disk.
	writeFile(t, filepath.Join(dir, "linked.txt"))

	candidates, err := FindCandidates(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1 (symlinked media + existing sidecar)", len(candidates))
	}
	if candidates[0].MediaPath != link || candidates[0].SidecarPath != filepath.Join(dir, "linked.txt") {
		t.Errorf("unexpected candidate: %+v", candidates[0])
	}
}
