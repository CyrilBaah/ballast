// Package summaries writes a summary of every video with speech, entirely
// on the user's Mac and at no cost (Feature 006): once captions
// (Feature 005) hand over a transcript, a bundled llama-server runs a free
// open model that returns the summary as JSON in a fixed shape. Ballast
// then checks every quote against the transcript, routes doubtful
// references to a "Check before sharing" note, enforces the share-message
// rules, saves a Markdown copy next to the video, and -- once the video has
// arrived -- a Google Doc next to it in Drive. Nothing here touches the
// upload or the captions.
package summaries
