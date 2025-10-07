package syro

import (
	"strings"
)

// ErrGroup is a helper struct for cases when a single function
// could have multiple errors which should be accumulated,
// instead of returning the first one.
type ErrGroup struct {
	Errors []error
	Props  *ErrGroupProps
}

type ErrGroupProps struct {
	ID          string
	WithNewline bool
}

func NewErrGroup(ep ...ErrGroupProps) *ErrGroup {
	eg := &ErrGroup{
		Errors: make([]error, 0),
	}

	if len(ep) == 1 {
		eg.Props = &ep[0]
	}

	return eg
}

func (eg *ErrGroup) Add(err error) {
	if eg != nil && err != nil {
		eg.Errors = append(eg.Errors, err)
	}
}

func (eg *ErrGroup) GetErrors() []error {
	if eg != nil {
		return eg.Errors
	}

	return nil
}

// Error implements the error interface. It returns a concatenated string of all
// non-nil ErrGroup, each separated by a semicolon.
//
// TODO: write tests for this method
func (eg *ErrGroup) Error() string {
	if eg == nil || len(eg.Errors) == 0 {
		return ""
	}

	propsExist := eg.Props != nil

	sb := strings.Builder{}
	if propsExist && eg.Props.ID != "" {
		sb.WriteString(eg.Props.ID)
		sb.WriteString(": ")
	}

	// Track whether we’ve written any errors (for semicolon separation)
	first := true
	for _, err := range eg.Errors {
		if err != nil {
			if !first {
				sb.WriteString("; ")
				if propsExist && eg.Props.WithNewline {
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

	return len(eg.Errors)
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
