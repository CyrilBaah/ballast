# Findings: Embedded Caption Track (User Story 4)

**Feature**: [spec.md](./spec.md) | **Method**: [research.md §13](./research.md) | **Date**: 2026-10-06

**Question**: Can Ballast put captions *inside* the video, as a selectable
caption track, without re-encoding, so they appear in Google Drive's
player with no manual "attach captions" step, and does Drive's player
show such a track?

## 1. Can it be done without touching picture or sound? Yes (measured)

Test video: `Aksum.mp4` (13 min 23 s, 71.8 MB, H.264 + AAC), with the
caption file Ballast's own pipeline produced for it (`Aksum.srt`, 125 cues).

```sh
ffmpeg -i Aksum.mp4 -i Aksum.srt -map 0 -map 1 -c copy -c:s mov_text \
       -metadata:s:s:0 language=eng "Aksum (captions embedded).mp4"
```

| Check | Result |
|---|---|
| Time to make the copy | **0.26 s** |
| Size | 71,764,037 → 71,777,978 bytes (**+14 KB**) |
| Video stream MD5 (original vs copy) | `c0db937e…` = `c0db937e…`: **identical** |
| Audio stream MD5 (original vs copy) | `7dd5834c…` = `7dd5834c…`: **identical** |
| Streams in the copy | h264 video, aac audio, **mov_text subtitle (eng)** |

Copying the streams loses nothing, is near-instant, and adds almost no
size. The one cost is that it writes a **full second copy of the video**:
13 GB more disk for a 13 GB sermon, plus the time to write it (bounded by
disk speed, probably a minute or two for 13 GB on an SSD; not measured
here). Note that the copy is a different file from the original, so the
spec's "original file is uploaded untouched" promise (FR-001) would need
the user's explicit opt-in.

## 2. Does Google Drive's player show the embedded track? Not yet tested

**Pending, and needs the maintainer.** It needs the copy uploaded to
Drive and opened in Drive's web player.

To finish:
1. Upload `Aksum (captions embedded).mp4` (saved in this session's
   scratchpad; recreate it with the command above) to a test Drive folder.
2. Wait for Drive to finish processing it, then open it in Drive's player.
3. Record, with a screenshot, whether the CC button offers an English
   track from the file itself, without anything attached through ⋮ ›
   Manage caption tracks.
4. Write the answer below.

**Answer**: _pending_

## Recommendation (to be confirmed by §2)

- **If Drive shows the track**: offer an opt-in setting such as "Also
  upload a copy with captions built in". This would be a new, separately
  specified feature, because it changes the uploaded file and doubles the
  disk needed during upload.
- **If Drive doesn't show it**: keep the current design (a separate `.srt`
  next to the video). Embedding would only help when the video is
  downloaded and played elsewhere, such as VLC or QuickTime, which isn't
  worth a second full copy of every video.
