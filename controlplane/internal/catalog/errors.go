package catalog

import (
	"errors"
	"fmt"
)

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrAgentNotFound    = errors.New("agent not found")
	ErrThreadNotFound   = errors.New("thread not found")
)

type providerNotFoundError struct {
	id string
}

func (e providerNotFoundError) Error() string {
	return fmt.Sprintf("provider %q not found", e.id)
}

func (e providerNotFoundError) Is(target error) bool {
	return target == ErrProviderNotFound
}

type agentNotFoundError struct {
	id string
}

func (e agentNotFoundError) Error() string {
	return fmt.Sprintf("agent %q not found", e.id)
}

func (e agentNotFoundError) Is(target error) bool {
	return target == ErrAgentNotFound
}

func newProviderNotFound(id string) error {
	return providerNotFoundError{id: id}
}

func newAgentNotFound(id string) error {
	return agentNotFoundError{id: id}
}

type threadNotFoundError struct {
	id string
}

func (e threadNotFoundError) Error() string {
	return fmt.Sprintf("thread %q not found", e.id)
}

func (e threadNotFoundError) Is(target error) bool {
	return target == ErrThreadNotFound
}

func newThreadNotFound(id string) error {
	return threadNotFoundError{id: id}
}
