package servicecentral

import (
	"fmt"
	"regexp"

	"github.com/atlassian/voyager"
)

var (
	// essentially all tags start with "micros2:" and then have the key/value
	// separated by an equals sign.
	// We allow a larger set of characters in the value to match micros
	voyagerTagRegexp = regexp.MustCompile(`^micros2:([A-Za-z_\-0-9]{1,254})=([a-zA-Z0-9_ +=/.-]{1,254})$`)
)

// convertPlatformTags converts tags from the platform format (a map)
// to the desired format for service central, a slice of strings
func convertPlatformTags(tags map[voyager.Tag]string) []string {
	converted := make([]string, 0, len(tags))
	for k, v := range tags {
		converted = append(converted, fmt.Sprintf("micros2:%s=%s", k, v))
	}

	return converted
}

// parsePlatformTags filters out platform-related tags from service central format and
// builds a map of key/value pairs
func parsePlatformTags(tags []string) map[voyager.Tag]string {
	platformTags := map[voyager.Tag]string{}
	for _, v := range tags {
		parts := voyagerTagRegexp.FindStringSubmatch(v)
		if parts == nil {
			continue
		}

		platformTags[voyager.Tag(parts[1])] = parts[2]
	}

	return platformTags
}

// nonPlatformTags returns only tags that do not match the platform pattern
func nonPlatformTags(tags []string) []string {
	filtered := make([]string, 0, len(tags))
	for _, v := range tags {
		if !voyagerTagRegexp.MatchString(v) {
			filtered = append(filtered, v)
		}
	}

	return filtered
}
