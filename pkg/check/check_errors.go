package check

type CheckError struct {
	label string
	err   error
}

var _ error = &CheckError{}

func (c *CheckError) Error() string {
	return c.err.Error()
}
