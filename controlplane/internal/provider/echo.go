package provider

import acp "github.com/coder/acp-go-sdk"

func PromptText(blocks []acp.ContentBlock) string {
	var out string
	for _, b := range blocks {
		if b.Text != nil {
			out += b.Text.Text
		}
	}
	return out
}

func Echo(text string) string {
	return text
}
