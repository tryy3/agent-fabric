package catalog

import (
	"errors"
	"fmt"
)

var (
	ErrInferenceConnectionNotFound = errors.New("inference connection not found")
	ErrAssistantNotFound           = errors.New("assistant not found")
	ErrThreadNotFound              = errors.New("thread not found")
	ErrProjectNotFound             = errors.New("project not found")
)

type inferenceConnectionNotFoundError struct {
	id string
}

func (e inferenceConnectionNotFoundError) Error() string {
	return fmt.Sprintf("inference connection %q not found", e.id)
}

func (e inferenceConnectionNotFoundError) Is(target error) bool {
	return target == ErrInferenceConnectionNotFound
}

type assistantNotFoundError struct {
	id string
}

func (e assistantNotFoundError) Error() string {
	return fmt.Sprintf("assistant %q not found", e.id)
}

func (e assistantNotFoundError) Is(target error) bool {
	return target == ErrAssistantNotFound
}

func newInferenceConnectionNotFound(id string) error {
	return inferenceConnectionNotFoundError{id: id}
}

func newAssistantNotFound(id string) error {
	return assistantNotFoundError{id: id}
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

type projectNotFoundError struct {
	id string
}

func (e projectNotFoundError) Error() string {
	return fmt.Sprintf("project %q not found", e.id)
}

func (e projectNotFoundError) Is(target error) bool {
	return target == ErrProjectNotFound
}

func newProjectNotFound(id string) error {
	return projectNotFoundError{id: id}
}
