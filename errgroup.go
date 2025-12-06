package syro

import (
	"strings"
)

// ErrGroup is a helper struct for cases when a single function
// could have multiple errors which should be accumulated,
// instead of returning the first one.
// TODO: add safety for concurrency
type ErrGroup struct {
	ID          string
	errors      []error
	withNewline bool
}

func NewErrGroup() *ErrGroup {
	return &ErrGroup{
		errors:      make([]error, 0),
		withNewline: false,
		ID:          "",
	}
}

func (eg *ErrGroup) WithID(id string) *ErrGroup {
	if eg != nil {
		eg.ID = id
	}
	return eg
}

func (eg *ErrGroup) WithNewline(v bool) *ErrGroup {
	if eg != nil {
		eg.withNewline = v
	}
	return eg
}

func (eg *ErrGroup) Add(err error) {
	if eg != nil && err != nil {
		eg.errors = append(eg.errors, err)
	}
}

func (eg *ErrGroup) Errors() []error {
	if eg != nil {
		return eg.errors
	}

	return nil
}

// Error implements the error interface. It returns a concatenated string of all
// non-nil ErrGroup, each separated by a semicolon.
//
// TODO: write tests for this method
func (eg *ErrGroup) Error() string {
	if eg == nil {
		return ""
	}

	if len(eg.errors) == 0 {
		return ""
	}

	sb := strings.Builder{}
	if eg.ID != "" {
		sb.WriteString(eg.ID)
		sb.WriteString(": ")
	}

	// Track whether we’ve written any errors (for semicolon separation)
	first := true
	for _, err := range eg.errors {
		if err != nil {
			if !first {
				sb.WriteString("; ")
				if eg.withNewline {
					sb.WriteString("\n")
				}
			}
			sb.WriteString(err.Error())
			first = false
		}
	}

	return sb.String()
}

func (eg *ErrGroup) Len() int {
	if eg == nil {
		return 0
	}

	return len(eg.errors)
}

// Return the error only if at least one of them happened. This is done because
// the ErrGroup is not nil when created, but it may be empty.
func (eg *ErrGroup) ToErr() error {
	if eg == nil {
		return nil
	}

	if eg.Len() == 0 {
		return nil
	}

	return eg
}
