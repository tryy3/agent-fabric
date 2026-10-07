// Fake OpenAI-compatible server for verification: no real LLM, deterministic reply.
// Usage: go run ./fake-llm/main.go -addr 127.0.0.1:8098
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type msg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func text(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	}
	return ""
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8098", "listen address")
	flag.Parse()
	http.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"fake-model","object":"model"}]}`)
	})
	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream   bool  `json:"stream"`
			Messages []msg `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		last := ""
		for _, m := range req.Messages {
			if m.Role == "user" {
				last = text(m.Content)
			}
		}
		reply := "FAKE-REPLY: " + last
		log.Printf("chat stream=%v reply=%q", req.Stream, reply)
		if !req.Stream {
			w.Header().Set("content-type", "application/json")
			b, _ := json.Marshal(reply)
			fmt.Fprintf(w, `{"id":"c1","object":"chat.completion","model":"fake-model","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`, b)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, w2 := range strings.SplitAfter(reply, " ") {
			b, _ := json.Marshal(w2)
			fmt.Fprintf(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"model\":\"fake-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%s}}]}\n\n", b)
			fl.Flush()
		}
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"model\":\"fake-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":5,\"total_tokens\":10}}\n\ndata: [DONE]\n\n")
		fl.Flush()
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}
