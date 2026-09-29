package catalog_test

import (
	"slices"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestKindCapabilities(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind string
		want []string
	}{
		{catalog.KindSearXNG, []string{catalog.CapabilityWebSearch}},
		{catalog.KindLinkup, []string{catalog.CapabilityWebSearch, catalog.CapabilityFetchPage}},
		{catalog.KindGetMD, []string{catalog.CapabilityFetchPage}},
		{catalog.KindCrawl4AI, []string{catalog.CapabilityFetchPage}},
		{"unknown", nil},
	}
	for _, tc := range cases {
		got := catalog.KindCapabilities(tc.kind)
		if !slices.Equal(got, tc.want) {
			t.Fatalf("KindCapabilities(%q) = %#v, want %#v", tc.kind, got, tc.want)
		}
	}
}
