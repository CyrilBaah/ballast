// Package captions makes a caption file for an uploaded video entirely on
// the user's Mac (Feature 005): it pulls the audio out with macOS's
// afconvert, transcribes it in ~10-minute pieces with a bundled whisper-cli,
// merges and cleans the pieces into one SubRip (.srt) file, saves a copy
// next to the original video, and hands it to the worker to put in Drive
// once the video has arrived. Nothing here touches the upload itself.
package captions
