package check

import (
	"errors"
	"fmt"
	"strings"
)

func JoinErrors(errs []error) error {
	switch {
	case len(errs) == 0:
		return nil
	case len(errs) < 5:
		return join(errs)
	default:
		return fmt.Errorf("%d errors found, listing first 5:\n%s", len(errs), join(errs[0:4]))
	}
}

func join(errs []error) error {
	var s []string
	for _, err := range errs {
		s = append(s, err.Error())
	}
	return errors.New(strings.Join(s, ";\n"))
}

func joinWithSeparator(errs []error, separator string) error {
	var s []string
	for _, err := range errs {
		s = append(s, err.Error())
	}
	return errors.New(strings.Join(s, separator))
}
