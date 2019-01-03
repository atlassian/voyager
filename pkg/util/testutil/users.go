package testutil

import "github.com/atlassian/voyager/pkg/util/auth"

func Named(name string) auth.User {
	user, err := auth.Named(name)
	if err != nil {
		panic(err)
	}
	return user
}
