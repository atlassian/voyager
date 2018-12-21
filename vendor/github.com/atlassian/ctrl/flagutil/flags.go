package flagutil

import (
	"errors"
	"flag"
	"fmt"
)

func ValidateFlags(flagset *flag.FlagSet, args []string) error {
	validator := flagValidator{
		args:    args,
		flagset: flagset,
	}
	for {
		seen, err := validator.validateNextFlag()
		if seen {
			continue
		}
		if err == nil {
			break
		}
		return err
	}
	return nil
}

type flagValidator struct {
	args    []string
	flagset *flag.FlagSet
}

// optional interface to indicate boolean flags that can be
// supplied without "=value" text
type boolFlag interface {
	IsBoolFlag() bool
}

// This method is based on the parseOne() method from https://golang.org/src/flag/flag.go,
// but was rewritten for extra validation rather than parsing flag values.
// nolint: gocyclo
func (v *flagValidator) validateNextFlag() (bool, error) {
	if len(v.args) == 0 {
		return false, nil
	}
	s := v.args[0]
	if len(s) < 2 || s[0] != '-' {
		return false, fmt.Errorf("invalid flag: %s", s)
	}
	numMinuses := 1
	if s[1] == '-' {
		numMinuses++
		if len(s) == 2 { // "--" terminates the flags
			v.args = v.args[1:]
			return false, errors.New("'--' flag with no name is not allowed")
		}
	}
	name := s[numMinuses:]
	if len(name) == 0 || name[0] == '-' || name[0] == '=' {
		return false, fmt.Errorf("bad flag syntax: %s", s)
	}

	// it's a flag. does it have an argument?
	v.args = v.args[1:]
	hasValue := false
	value := ""
	for i := 1; i < len(name); i++ { // equals cannot be first
		if name[i] == '=' {
			value = name[i+1:]
			hasValue = true
			name = name[0:i]
			break
		}
	}

	flag := v.flagset.Lookup(name)
	if flag == nil {
		return false, fmt.Errorf("undefined flag: -%s", name)
	}

	if fv, ok := flag.Value.(boolFlag); ok && fv.IsBoolFlag() { // special case: doesn't need an arg
		if len(v.args) > 0 {
			nextArg := v.args[0]
			if nextArg[0] != '-' {
				return false, fmt.Errorf("invalid value following flag -%s: %q; boolean flags must be passed as -flag=x", name, nextArg)
			}
		}
	} else {
		// It must have a value, which might be the next argument.
		if !hasValue && len(v.args) > 0 {
			// value is the next arg
			hasValue = true
			value, v.args = v.args[0], v.args[1:]
		}
		if !hasValue {
			return false, fmt.Errorf("flag needs an argument: -%s", name)
		}
	}
	_ = value
	return true, nil
}
