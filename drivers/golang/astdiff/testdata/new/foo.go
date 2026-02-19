package testmod

import "context"

// Functions

// DoWork: signature changed (added opts param).
func DoWork(ctx context.Context, name string, opts map[string]string) (string, error) {
	return "", nil
}

// SimpleFunc: unchanged.
func SimpleFunc() {
}

// HelperFunc renamed to HelperFunction (same signature).
func HelperFunction(a, b int) int {
	return a + b
}

// OldOnly removed (not present in new).

// Variadic: unchanged.
func Variadic(args ...string) int {
	return len(args)
}

// FuzzyRenamed: similar to OldOnly but different name and added param.
// Should NOT match OldOnly in Pass 5 (different param types).

// Types

// Config renamed to Settings (same fields).
type Settings struct {
	Host    string
	Port    int
	Timeout int
	secret  string
}

// Handler: method signature changed.
type Handler interface {
	Handle(ctx context.Context, req string, opts ...string) (string, error)
	Close() error
}

// Token: underlying type changed.
type Token int

// Methods

// Settings.Validate: follows type rename, same signature.
func (s *Settings) Validate() error {
	return nil
}

// Settings.Apply: follows type rename, signature changed.
func (s *Settings) Apply(target string, force bool) (bool, error) {
	return false, nil
}

// Constants and variables

const MaxRetries int = 3

const UntypedConst = "hello"

var ErrNotFound error

var DefaultConfig Settings

// Cross-kind: was var, now func.
func ComputeHash(data string) string {
	return ""
}

// Added in new (not breaking).
func NewFeature() {}
