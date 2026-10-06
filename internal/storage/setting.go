package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// Setting keys (Feature 005 data-model.md). A missing key means its default.
const (
	settingCaptionsEnabled     = "captions_enabled"
	settingCaptionLanguage     = "caption_language"
	settingCaptionModelConsent = "caption_model_consent"
	settingSummariesEnabled    = "summaries_enabled"
	settingSummaryModelConsent = "summary_model_consent"
)

// Consent is the user's answer to a one-time model-download prompt.
type Consent string

const (
	ConsentUnasked  Consent = "unasked"
	ConsentAccepted Consent = "accepted"
	ConsentDeclined Consent = "declined"
)

// settingDefaults are what a key reads as before it has ever been set.
var settingDefaults = map[string]string{
	settingCaptionsEnabled:     "true",
	settingCaptionLanguage:     "en",
	settingCaptionModelConsent: string(ConsentUnasked),
	settingSummariesEnabled:    "true",
	settingSummaryModelConsent: string(ConsentUnasked),
}

// GetSetting returns key's stored value, its default if it has never been
// set, or "" for a key with no default.
func (d *DB) GetSetting(key string) (string, error) {
	var v string
	err := d.conn.QueryRow(`SELECT value FROM setting WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return settingDefaults[key], nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: get setting %s: %w", key, err)
	}
	return v, nil
}

// SetSetting stores value under key, replacing any earlier value.
func (d *DB) SetSetting(key, value string) error {
	if _, err := d.conn.Exec(`
		INSERT INTO setting (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value); err != nil {
		return fmt.Errorf("storage: set setting %s: %w", key, err)
	}
	return nil
}

// CaptionsEnabled reports whether automatic captions are on (default on).
func (d *DB) CaptionsEnabled() (bool, error) {
	v, err := d.GetSetting(settingCaptionsEnabled)
	return v == "true", err
}

// SetCaptionsEnabled turns automatic captions on or off.
func (d *DB) SetCaptionsEnabled(on bool) error {
	v := "false"
	if on {
		v = "true"
	}
	return d.SetSetting(settingCaptionsEnabled, v)
}

// CaptionLanguage returns "en" (default) or "auto".
func (d *DB) CaptionLanguage() (string, error) {
	return d.GetSetting(settingCaptionLanguage)
}

// SetCaptionLanguage stores the caption language; only "en" and "auto"
// are offered (research.md §11).
func (d *DB) SetCaptionLanguage(lang string) error {
	if lang != "en" && lang != "auto" {
		return fmt.Errorf("storage: unsupported caption language %q", lang)
	}
	return d.SetSetting(settingCaptionLanguage, lang)
}

// CaptionModelConsent returns the answer to the speech-model download prompt.
func (d *DB) CaptionModelConsent() (Consent, error) {
	v, err := d.GetSetting(settingCaptionModelConsent)
	return Consent(v), err
}

// SetCaptionModelConsent records the answer to the speech-model download prompt.
func (d *DB) SetCaptionModelConsent(c Consent) error {
	return d.SetSetting(settingCaptionModelConsent, string(c))
}

// SummariesEnabled reports whether automatic summaries are on (default
// on, Feature 006 FR-012).
func (d *DB) SummariesEnabled() (bool, error) {
	v, err := d.GetSetting(settingSummariesEnabled)
	return v == "true", err
}

// SetSummariesEnabled turns automatic summaries on or off.
func (d *DB) SetSummariesEnabled(on bool) error {
	v := "false"
	if on {
		v = "true"
	}
	return d.SetSetting(settingSummariesEnabled, v)
}

// SummaryModelConsent returns the answer to the summary-model download prompt.
func (d *DB) SummaryModelConsent() (Consent, error) {
	v, err := d.GetSetting(settingSummaryModelConsent)
	return Consent(v), err
}

// SetSummaryModelConsent records the answer to the summary-model download prompt.
func (d *DB) SetSummaryModelConsent(c Consent) error {
	return d.SetSetting(settingSummaryModelConsent, string(c))
}
