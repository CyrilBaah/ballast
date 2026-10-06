# Changelog

All notable changes to Ballast are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Ballast has no
tagged releases yet, so entries are grouped by date.

## [Unreleased]

### Added

- Automatic captions for uploaded videos (`.mp4`, `.mov`, `.m4v`) on
  Apple-silicon Macs. They are made on your own Mac for free, using a speech
  model downloaded once after you agree.
  - A caption file is saved next to the original video as soon as it's
    ready, and one is placed next to the video in Drive once the video has
    arrived. The video itself is uploaded untouched.
  - Long videos are captioned in about 10-minute pieces, so memory stays
    flat and a restart picks up where it left off.
  - Captioning never slows or fails an upload. If captions can't be made,
    Ballast says why.
  - Settings has an on/off switch and an English/Automatic language choice.
- Pick and upload several files at once.
- Retry a cancelled upload, and delete finished, failed, or cancelled uploads
  from history.
- Ballast now keeps a log file at `ballast.log` in its app data folder
  (`~/Library/Application Support/ballast/` on macOS), rotated once it passes
  10 MB, so problems with an upload can be traced after the fact.
- Spec for automatic video summaries (`specs/006-video-summaries`): a free
  summary of every video with speech, made on your own Mac from its
  captions, with a message ready to share in a group chat.
- Spec for automatic video captions (`specs/005-automatic-video-captions`):
  captions made on your own Mac and saved next to the video in Drive, without
  changing the video.
- Draft spec for pausing, resuming, and removing in-flight transfers
  (`specs/004-in-flight-transfer-controls`).

### Changed

- Signing out partway through an upload no longer fails it. The upload holds
  its place and continues from its last confirmed byte after you sign back in.
- A brief problem refreshing your Google session is now retried instead of
  being treated as a sign-out.
- When Drive drops an upload's session, Ballast restarts it from the beginning
  by itself once, instead of stopping to ask.

### Fixed

- A single upload could leave several copies of the same file in Drive. Each
  upload now runs only one transfer at a time, an upload that is already
  running is no longer started a second time after signing in, and before
  restarting an upload from the beginning Ballast checks whether Drive already
  has the finished file and keeps it instead of uploading it again.
- Cancelling an upload that could not be cancelled no longer released its
  Drive session, which made the transfer restart from the beginning.

## 2026-08-15

### Added

- Redesigned interface: frameless window with a custom title bar, and new
  Home, Transfers, Journal, Field Lab, and Settings views built on a shared
  design system.
- Cancel action and live upload speed on the upload history screen.

### Fixed

- An upload stuck as pending is now marked as failed instead of being left
  as an invisible row.

## 2026-08-06

### Changed

- Upload progress is shown as a percentage instead of bytes.

### Fixed

- Restarting an upload left duplicate files in Drive; the old Drive session
  is now released before a restart.
- Stranded uploads are recovered, and paused and blocked uploads are labeled
  clearly.

## 2026-08-05

### Added

- Adaptive chunk sizing: chunk size grows while the connection is healthy and
  shrinks after failures, and is kept across restarts.
- First UI redesign with a design-token system, an upload history screen,
  your account name and picture, and your Drive storage quota.

### Fixed

- Flaky Go tests on Ubuntu and Windows CI, and flaky Playwright tests.

## 2026-08-03

### Added

- Google sign-in, local file and Drive folder picker, and basic upload to
  Google Drive.
- Resumable, crash-safe upload engine: uploads continue from the last
  confirmed byte after a dropped connection, app crash, or restart.

## 2026-08-02

### Added

- Project setup: Spec Kit, the project constitution, and the first feature
  spec.
