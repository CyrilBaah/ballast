#!/usr/bin/env bash
# Builds the helper programs Ballast runs as child processes -- the
# speech-to-text engine for captions (Feature 005) -- as static arm64
# binaries with Metal, into build/engines/. With --bundle <Ballast.app>,
# also copies them into the app bundle next to Ballast's own executable,
# where the app looks for them at runtime (research.md §8).
#
# Usage:
#   scripts/build-engines.sh                      # build into build/engines/
#   scripts/build-engines.sh --bundle build/bin/Ballast.app
set -euo pipefail

# Pinned so a rebuild always produces the same engine. Bump deliberately.
WHISPER_CPP_TAG="v1.9.4"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/build/engines"
SRC="$ROOT/build/engines/src"

bundle=""
if [[ "${1:-}" == "--bundle" ]]; then
	bundle="${2:?--bundle needs the path to Ballast.app}"
fi

if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" ]]; then
	echo "Engines are built for Apple-silicon Macs only (captions are not available elsewhere)." >&2
	exit 1
fi
for tool in cmake git xcrun; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "Missing '$tool'. Install it first (cmake: 'brew install cmake'; xcrun: 'xcode-select --install')." >&2
		exit 1
	fi
done

mkdir -p "$OUT" "$SRC"

build_whisper() {
	local dir="$SRC/whisper.cpp"
	if [[ ! -d "$dir" ]]; then
		git clone --depth 1 --branch "$WHISPER_CPP_TAG" https://github.com/ggml-org/whisper.cpp.git "$dir"
	fi
	cmake -S "$dir" -B "$dir/build" \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_OSX_ARCHITECTURES=arm64 \
		-DGGML_METAL=ON \
		-DGGML_METAL_EMBED_LIBRARY=ON \
		-DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_SERVER=OFF
	cmake --build "$dir/build" --config Release --target whisper-cli -j
	cp "$dir/build/bin/whisper-cli" "$OUT/whisper-cli"
	echo "Built $OUT/whisper-cli ($WHISPER_CPP_TAG)"
}

build_whisper

if [[ -n "$bundle" ]]; then
	dest="$bundle/Contents/MacOS"
	if [[ ! -d "$dest" ]]; then
		echo "Not an app bundle: $bundle" >&2
		exit 1
	fi
	cp "$OUT/whisper-cli" "$dest/whisper-cli"
	echo "Copied engines into $dest"
fi
