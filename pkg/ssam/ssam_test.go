package ssam

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateServiceNameWithDelimiterBadCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		BadServiceName string
	}{
		{"foo-dl"},
		{"dl-foo"},
		{"foo-dl-bar"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ValidateServiceName with ServiceName as %q", tc.BadServiceName), func(t *testing.T) {
			badName := tc.BadServiceName
			err := ValidateServiceName(badName)
			require.Error(t, err)
		})
	}
}

func TestValidateServiceNameWithDelimiterGoodCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		GoodServiceName string
	}{
		{"noodle"},
		{"dlmanager"},
		{"managerdl"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ValidateService with ServiceName as %q", tc.GoodServiceName), func(t *testing.T) {
			goodName := tc.GoodServiceName
			err := ValidateServiceName(goodName)
			require.NoError(t, err)
		})
	}
}
