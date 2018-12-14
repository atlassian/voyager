package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	username = "an_owner"
)

func TestRequestUserWorksOnNormalUser(t *testing.T) {
	t.Parallel()

	// given
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)
	reqWithAuthInfo := req.WithContext(
		CreateContextWithAuthInfo(context.Background(), authInfo(username)))

	// when
	_, usernameErr := RequestUser(reqWithAuthInfo)

	// then
	require.NoError(t, usernameErr)
}

func TestRequestUserErrorsOnMissingAuthInfo(t *testing.T) {
	t.Parallel()

	// given
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)

	// when
	_, usernameErr := RequestUser(req)

	// then
	require.Error(t, usernameErr)
	require.True(t, IsErrAuthInfoMissing(usernameErr))
}

func TestMaybeRequestUserWorksOnNormalUser(t *testing.T) {
	t.Parallel()

	// given
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)
	reqWithAuthInfo := req.WithContext(
		CreateContextWithAuthInfo(context.Background(), authInfo(username)))

	// when
	user := MaybeRequestUser(reqWithAuthInfo)

	// then
	require.Equal(t, username, *user.name)
}

func TestMaybeRequestUserWorksOnMissingAuthInfo(t *testing.T) {
	t.Parallel()

	// given
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)

	// when
	user := MaybeRequestUser(req)

	// then
	require.Nil(t, user.name)
}

func authInfo(username string) AggregatorUserInfo {
	return AggregatorUserInfo{
		User:   username,
		Groups: []string{"grp1"},
	}
}
