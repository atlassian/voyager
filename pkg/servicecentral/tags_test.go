package servicecentral

import (
	"testing"

	"github.com/atlassian/voyager"
	"github.com/stretchr/testify/assert"
)

func TestParsesTagsFromServiceCentralCorrectly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		inputTags    []string
		expectedTags map[voyager.Tag]string
	}{
		{
			"nothing mapped",
			[]string{"not", "mapped"},
			map[voyager.Tag]string{}},
		{
			"mix of tags",
			[]string{"skipthis", "micros2:foo=bar"},
			map[voyager.Tag]string{"foo": "bar"},
		},
		{
			"valid and invalid",
			[]string{"micros2:blah=something", "micros:notvalid=skipme"},
			map[voyager.Tag]string{"blah": "something"},
		},
		{
			"skip invalid",
			[]string{"micros2:=nokey", "micros2:novalue="},
			map[voyager.Tag]string{},
		},
		{
			"special chars",
			[]string{"micros2:some_key-other====this+works/but_is-silly"},
			map[voyager.Tag]string{"some_key-other": "===this+works/but_is-silly"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := parsePlatformTags(tc.inputTags)
			assert.Equal(t, tc.expectedTags, actual)
		})
	}
}

func TestConvertsTagsToServiceCentralFormatCorrectly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		inputTags map[voyager.Tag]string
		expected  []string
	}{
		{
			"Single value",
			map[voyager.Tag]string{
				"foo": "bar",
			},
			[]string{"micros2:foo=bar"},
		},
		{
			"Multiple values",
			map[voyager.Tag]string{
				"foo":       "bar",
				"other":     "baz",
				"something": "else",
			},
			[]string{"micros2:foo=bar", "micros2:other=baz", "micros2:something=else"},
		},
		{
			"Empty map",
			map[voyager.Tag]string{},
			[]string{},
		},
		{
			"nil map",
			nil,
			[]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := convertPlatformTags(tc.inputTags)
			assert.ElementsMatch(t, tc.expected, actual)
		})
	}
}

func TestFiltersNonPlatformTagsCorrectly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		inputTags []string
		expected  []string
	}{
		{
			"Only SC",
			[]string{"random", "things"},
			[]string{"random", "things"},
		},
		{
			"Mix of values",
			[]string{"sc entry", "micros2:foo=bar"},
			[]string{"sc entry"},
		},
		{
			"Platform only",
			[]string{"micros2:foo=bar", "micros2:blah=something"},
			[]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := nonPlatformTags(tc.inputTags)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
