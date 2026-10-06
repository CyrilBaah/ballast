package storage

import (
	"path/filepath"
	"testing"
)

func TestSettingDefaults(t *testing.T) {
	db := newTestDB(t)

	if on, err := db.CaptionsEnabled(); err != nil || !on {
		t.Errorf("CaptionsEnabled() = %v, %v; want true by default", on, err)
	}
	if lang, err := db.CaptionLanguage(); err != nil || lang != "en" {
		t.Errorf("CaptionLanguage() = %q, %v; want en by default", lang, err)
	}
	if c, err := db.CaptionModelConsent(); err != nil || c != ConsentUnasked {
		t.Errorf("CaptionModelConsent() = %q, %v; want unasked by default", c, err)
	}
	if v, err := db.GetSetting("no_such_key"); err != nil || v != "" {
		t.Errorf("GetSetting(unknown) = %q, %v; want empty", v, err)
	}
}

func TestSettingRoundTripAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	if err := db.SetCaptionsEnabled(false); err != nil {
		t.Fatalf("SetCaptionsEnabled: %v", err)
	}
	if err := db.SetCaptionLanguage("auto"); err != nil {
		t.Fatalf("SetCaptionLanguage: %v", err)
	}
	if err := db.SetCaptionModelConsent(ConsentAccepted); err != nil {
		t.Fatalf("SetCaptionModelConsent: %v", err)
	}
	// Overwriting an existing key replaces it rather than adding a row.
	if err := db.SetCaptionLanguage("en"); err != nil {
		t.Fatalf("SetCaptionLanguage again: %v", err)
	}
	db.Close()

	db, err = OpenAt(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	if on, _ := db.CaptionsEnabled(); on {
		t.Error("CaptionsEnabled after reopen = true, want false")
	}
	if lang, _ := db.CaptionLanguage(); lang != "en" {
		t.Errorf("CaptionLanguage after reopen = %q, want en", lang)
	}
	if c, _ := db.CaptionModelConsent(); c != ConsentAccepted {
		t.Errorf("CaptionModelConsent after reopen = %q, want accepted", c)
	}
}

func TestSetCaptionLanguageRejectsUnknown(t *testing.T) {
	db := newTestDB(t)
	if err := db.SetCaptionLanguage("fr"); err == nil {
		t.Fatal("SetCaptionLanguage(fr) succeeded; only en and auto are allowed")
	}
}
