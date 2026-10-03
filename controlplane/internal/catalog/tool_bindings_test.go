package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

func TestDecodeToolBindingsDefaultsToInherit(t *testing.T) {
	got, err := catalog.DecodeToolBindings(json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.WebSearch.Mode != catalog.BindingInherit || got.FetchPage.Mode != catalog.BindingInherit {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeToolBindingsIntegration(t *testing.T) {
	raw := json.RawMessage(`{"toolBindings":{"webSearch":{"mode":"integration","integrationId":"ti_1"},"fetchPage":{"mode":"disabled"}}}`)
	got, err := catalog.DecodeToolBindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebSearch.Mode != catalog.BindingIntegration || got.WebSearch.IntegrationID != "ti_1" {
		t.Fatalf("webSearch = %+v", got.WebSearch)
	}
	if got.FetchPage.Mode != catalog.BindingDisabled {
		t.Fatalf("fetchPage = %+v", got.FetchPage)
	}
}

func TestResolveToolBinding(t *testing.T) {
	cases := []struct {
		name    string
		plane   string
		binding catalog.CapabilityBinding
		wantID  string
		wantOff bool
		wantErr bool
	}{
		{
			name:    "inherit with default",
			plane:   "ti_default",
			binding: catalog.CapabilityBinding{Mode: catalog.BindingInherit},
			wantID:  "ti_default",
		},
		{
			name:    "inherit without default disables",
			binding: catalog.CapabilityBinding{Mode: catalog.BindingInherit},
			wantOff: true,
		},
		{
			name:    "disabled",
			plane:   "ti_default",
			binding: catalog.CapabilityBinding{Mode: catalog.BindingDisabled},
			wantOff: true,
		},
		{
			name:    "override",
			plane:   "ti_default",
			binding: catalog.CapabilityBinding{Mode: catalog.BindingIntegration, IntegrationID: "ti_other"},
			wantID:  "ti_other",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := catalog.ResolveToolBinding(catalog.CapabilityWebSearch, tc.plane, tc.binding)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Disabled != tc.wantOff || got.IntegrationID != tc.wantID {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestValidateSettingsToolBindings(t *testing.T) {
	_, err := catalog.MergeSettings(json.RawMessage(`{}`), json.RawMessage(`{"toolBindings":{"webSearch":{"mode":"integration"}}}`))
	if err == nil {
		t.Fatal("expected validation error for missing integrationId")
	}
}
