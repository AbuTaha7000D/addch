package main

import (
	"testing"
)

func TestIsMP4Family(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".mp4", true},
		{".m4v", true},
		{".mov", true},
		{".3gp", true},
		{".3g2", true},
		{".MP4", true},
		{".mkv", false},
		{".webm", false},
		{".avi", false},
		{"", false},
		{".mp3", false},
	}
	for _, c := range cases {
		if got := isMP4Family(c.ext); got != c.want {
			t.Errorf("isMP4Family(%q) = %v, want %v", c.ext, got, c.want)
		}
	}
}
