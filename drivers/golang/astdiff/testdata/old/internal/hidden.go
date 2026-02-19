package hidden

// InternalFunc should be skipped by ParseExports.
func InternalFunc() string {
	return "hidden"
}
