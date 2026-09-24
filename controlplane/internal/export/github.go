package export

import (
	"context"
	"fmt"
)

type GitHub struct{}

func (GitHub) Method() Method {
	return Method{
		ID:      MethodGitHub,
		Label:   "GitHub",
		Enabled: false,
		Reason:  "coming soon",
	}
}

func (GitHub) Export(context.Context, Request) (Result, error) {
	return Result{}, fmt.Errorf("%w: %s", ErrDisabled, MethodGitHub)
}
