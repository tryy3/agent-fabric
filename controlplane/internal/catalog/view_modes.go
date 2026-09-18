package catalog

func ValidViewModeID(id string) bool {
	switch id {
	case "pretty", "detailed":
		return true
	default:
		return false
	}
}
