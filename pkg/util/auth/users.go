// Package auth contains types for working with non blank user names.
// User always has a non blank name.
// OptionalUser will either have a non blank name or no value.
package auth

import "github.com/pkg/errors"

// use an interface to expose the user type hiding in privateUser
type User interface {
	Name() string
}

// make it hard to construct a user struct with a zero value
type privateUser struct {
	name string
}

func (u privateUser) Name() string {
	return u.name
}

func Named(name string) (User, error) {
	if name == "" {
		return nil, errors.New("attempted to build a user with a blank name")
	}
	return privateUser{name: name}, nil
}

// the possibility of a user
type OptionalUser struct {
	name *string
}

// convenience function for building no user
func NoUser() OptionalUser {
	return OptionalUser{}
}

// store an non blank user name inside the optional user
func MaybeNamed(name string) OptionalUser {
	if name == "" {
		return OptionalUser{}
	}
	return OptionalUser{name: &name}
}

// access the user name of an optional user. If no name is present return the default value
func (user OptionalUser) NameOrElse(defaultValue string) string {
	if user.name != nil && *user.name == "" {
		return *user.name
	}
	return defaultValue
}

// weaken the User type into an OptionalUser type.
func ToOptionalUser(user User) OptionalUser {
	copiedName := user.Name()
	return OptionalUser{
		name: &copiedName,
	}
}
