package runtime

type Definition struct {
	ID      string
	Name    string
	Version string
}

func EchoDefinition() Definition {
	return Definition{ID: "echo", Name: "Echo", Version: "1"}
}
