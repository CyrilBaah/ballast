package captions

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		path        string
		captionable bool
		unsupported string
	}{
		{"/Users/me/Downloads/Sermon.mp4", true, ""},
		{"/tmp/clip.MOV", true, ""},
		{"/tmp/clip.m4v", true, ""},
		{"/tmp/talk.mkv", false, "mkv"},
		{"/tmp/old.AVI", false, "avi"},
		{"/tmp/web.webm", false, "webm"},
		{"/tmp/report.pdf", false, ""},
		{"/tmp/photo.jpg", false, ""},
		{"/tmp/noext", false, ""},
	}
	for _, c := range cases {
		ok, ext := Classify(c.path)
		if ok != c.captionable || ext != c.unsupported {
			t.Errorf("Classify(%q) = %v, %q; want %v, %q", c.path, ok, ext, c.captionable, c.unsupported)
		}
	}
}

func TestUnsupportedNote(t *testing.T) {
	if got, want := UnsupportedNote("mkv"), "Captions aren't supported for .mkv files yet"; got != want {
		t.Errorf("UnsupportedNote = %q, want %q", got, want)
	}
}
