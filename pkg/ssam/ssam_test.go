package ssam

import (
	"testing"

	"github.com/atlassian/voyager"
	"github.com/stretchr/testify/require"
)

func TestValidateServiceNameWithDelimiterBadCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		BadServiceName voyager.ServiceName
	}{
		{"foo-dl"},
		{"dl-foo"},
		{"foo-dl-bar"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.BadServiceName), func(t *testing.T) {
			badName := tc.BadServiceName
			err := ValidateServiceName(badName)
			require.Error(t, err)
		})
	}
}

func TestValidateServiceNameWithDelimiterGoodCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		GoodServiceName voyager.ServiceName
	}{
		{"noodle"},
		{"dlmanager"},
		{"managerdl"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.GoodServiceName), func(t *testing.T) {
			goodName := tc.GoodServiceName
			err := ValidateServiceName(goodName)
			require.NoError(t, err)
		})
	}
}
