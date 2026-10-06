package captions

import "ballast/internal/modelfetch"

// SpeechModel is whisper.cpp's multilingual large-v3-turbo model (research.md
// §2), pinned to repository revision 5359861c. Its size and SHA-256 were
// confirmed on 2026-10-06 both by hashing a real download and against
// Hugging Face's x-linked-size / x-linked-etag for that revision.
var SpeechModel = modelfetch.Spec{
	URL:      "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-large-v3-turbo.bin",
	Size:     1_624_555_275,
	SHA256:   "1fc70f774d38eb169993ac391eea357ef47c88757ef72ee5943879b7e8e2bc69",
	FileName: "ggml-large-v3-turbo.bin",
}
