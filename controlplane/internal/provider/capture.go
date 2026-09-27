package provider

import "net/http"

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, vals := range h {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

func emitHopCapture(opts StreamChatOptions, c HopCapture) {
	if opts.OnCapture == nil {
		return
	}
	opts.OnCapture(c)
}
