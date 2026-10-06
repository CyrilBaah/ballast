// Command fakellama stands in for llama-server in tests. It serves /health
// and /v1/chat/completions on the given --host/--port. FAKELLAMA_MODE
// picks the behaviour: ok (default), badjson, oom, loadfail, hang.
// Replies come from FAKELLAMA_SUMMARY / FAKELLAMA_NOTES (JSON strings);
// every request body is appended to FAKELLAMA_LOG, one per line.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	flag.String("m", "", "model")
	flag.Int("c", 0, "context")
	host := flag.String("host", "127.0.0.1", "host")
	port := flag.String("port", "8080", "port")
	flag.Int("ngl", 0, "gpu layers")
	flag.Int("np", 1, "parallel")
	flag.Parse()

	switch os.Getenv("FAKELLAMA_MODE") {
	case "oom":
		fmt.Fprintln(os.Stderr, "ggml_metal_buffer_alloc: failed to allocate buffer: out of memory")
		os.Exit(1)
	case "loadfail":
		fmt.Fprintln(os.Stderr, "llama_model_load: error loading model: invalid magic")
		os.Exit(1)
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"ok"}`)
	})
	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if p := os.Getenv("FAKELLAMA_LOG"); p != "" {
			f, _ := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			f.Write(append(compact(body), '\n'))
			f.Close()
		}
		mode := os.Getenv("FAKELLAMA_MODE")
		if mode == "hang" {
			time.Sleep(time.Hour)
		}
		content := os.Getenv("FAKELLAMA_SUMMARY")
		if !strings.Contains(string(body), "share_message") {
			content = os.Getenv("FAKELLAMA_NOTES")
		}
		if mode == "badjson" {
			content = "{not json"
		}
		resp := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}}
		json.NewEncoder(w).Encode(resp)
	})
	if err := http.ListenAndServe(*host+":"+*port, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compact(b []byte) []byte {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return b
	}
	out, _ := json.Marshal(v)
	return out
}
