package testmod

import "context"

// Functions

func DoWork(ctx context.Context, name string) (string, error) {
	return "", nil
}

func SimpleFunc() {
}

func HelperFunc(a, b int) int {
	return a + b
}

func OldOnly(x string) string {
	return x
}

func Variadic(args ...string) int {
	return len(args)
}

// Types

type Config struct {
	Host    string
	Port    int
	Timeout int
	secret  string
}

type Handler interface {
	Handle(ctx context.Context, req string) (string, error)
	Close() error
}

type Token string

type unexportedType struct{}

// Methods

func (c *Config) Validate() error {
	return nil
}

func (c *Config) Apply(target string) (bool, error) {
	return false, nil
}

func (u *unexportedType) Hidden() {}

// Constants and variables

const MaxRetries int = 3

const UntypedConst = "hello"

var ErrNotFound error

var DefaultConfig Config

// Cross-kind: this is a var in old, will become func in new
var ComputeHash string
