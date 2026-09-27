package catalog

func ValidViewModeID(id string) bool {
	switch id {
	case "pretty", "detailed", "raw":
		return true
	default:
		return false
	}
}
