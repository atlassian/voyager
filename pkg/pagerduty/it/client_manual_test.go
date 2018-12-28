package it

import (
	"os"
	"strings"
	"testing"

	pagerdutyClient "github.com/PagerDuty/go-pagerduty"
	"github.com/atlassian/voyager/pkg/pagerduty"
	"github.com/atlassian/voyager/pkg/util/auth"
	"github.com/atlassian/voyager/pkg/util/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	testServiceName = "voyager-NCC-74656"
	testUserName    = "jeffreyPenkcombus"
	testUserEmail   = testUserName + "@atlassian.com"
)

// This test is tagged in Bazel as manual, and therefore won't be run with the unit-tests. This is useful code for
// manually testing that your code works, not just testing against request/responses saved into test-data.
//
// Running:
// To make this test work you need to set a pagerduty token as the environment variable PD_KEY
func TestPagerduty(t *testing.T) {
	pdClient := envPDClient(t)
	testClient := client(t, pdClient)

	// test user should not be present
	user := findUser(t, pdClient, testUserEmail)
	require.Nil(t, user)

	authUser, err := auth.Named(testUserName)
	require.NoError(t, err)

	_, err = testClient.FindOrCreate(testServiceName, authUser, testUserEmail)
	require.NoError(t, err)

	err = testClient.Delete(testServiceName)
	require.NoError(t, err)

	// user should be present now
	user = findUser(t, pdClient, testUserEmail)
	require.NotNil(t, user)

	// Users can only be deleted if they have not associated escalation policies, and escalation policies can only be
	// deleted if they have not associated services, so if the user can be deleted we've deleted all the required data
	err = pdClient.DeleteUser(user.ID)
	require.NoError(t, err)
}

func envPDClient(t *testing.T) *pagerdutyClient.Client {
	pdKey := os.Getenv("PD_KEY")
	require.NotEmpty(t, pdKey)
	return pagerdutyClient.NewClient(pdKey)
}

func client(t *testing.T, pdClient *pagerdutyClient.Client) *pagerduty.Client {
	logger := zaptest.NewLogger(t)
	client, err := pagerduty.New(logger, pdClient, uuid.Default())
	require.NoError(t, err)
	return client
}

func findUser(t *testing.T, client *pagerdutyClient.Client, email string) *pagerdutyClient.User {
	q := pagerdutyClient.ListUsersOptions{
		Query: email,
	}
	listResp, err := client.ListUsers(q)
	require.NoError(t, err)
	for _, foundUser := range listResp.Users {
		if strings.EqualFold(foundUser.Email, email) {
			return &foundUser
		}
	}
	return nil
}
