package summaries

import (
	"errors"
	"testing"
)

func TestCleanShareMessage(t *testing.T) {
	in := "*Sermon Summary* 🙏\n1️⃣ *We are one family* (1 Peter 2:9–10) [04:28]\n✅ Love God (at 1:02:03)\n\n\nAmen! 🎉"
	got, err := CleanShareMessage(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "*Sermon Summary*\n1 *We are one family* (1 Peter 2:9–10)\nLove God\n\nAmen!"
	if got != want {
		t.Fatalf("CleanShareMessage =\n%q\nwant\n%q", got, want)
	}
}

func TestCleanShareMessageKeepsVerseNumbers(t *testing.T) {
	got, _ := CleanShareMessage("Romans 8:28 and Genesis 41:38–45")
	if got != "Romans 8:28 and Genesis 41:38–45" {
		t.Fatalf("verse references were mangled: %q", got)
	}
}

func TestCleanShareMessageRejectsTranscriptNotes(t *testing.T) {
	if _, err := CleanShareMessage("Note: the transcript was unclear here."); !errors.Is(err, ErrUnusable) {
		t.Fatalf("err = %v, want ErrUnusable", err)
	}
	if _, err := CleanShareMessage("   "); !errors.Is(err, ErrUnusable) {
		t.Fatalf("empty message err = %v, want ErrUnusable", err)
	}
}
