package agent

import (
	acp "github.com/coder/acp-go-sdk"
	"github.com/tryy3/agent-fabric/internal/runtime"
)

func modelConfigOptions(pin runtime.SessionPin) []acp.SessionConfigOption {
	options := make(acp.SessionConfigSelectOptionsUngrouped, 0, len(pin.Models))
	for _, m := range pin.Models {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		options = append(options, acp.SessionConfigSelectOption{
			Name:  name,
			Value: acp.SessionConfigValueId(m.ID),
		})
	}
	cat := acp.SessionConfigOptionCategoryModel
	return []acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{
			Type:         "select",
			Id:           acp.SessionConfigId("model"),
			Name:         "Model",
			Category:     &cat,
			CurrentValue: acp.SessionConfigValueId(pin.CurrentModel),
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &options},
		},
	}}
}
