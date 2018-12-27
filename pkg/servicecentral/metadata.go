package servicecentral

import (
	"encoding/json"

	creator_v1 "github.com/atlassian/voyager/pkg/apis/creator/v1"
)

const (
	// PagerDutyMetadataKey is the key we use to store the Metadata in the Misc field
	PagerDutyMetadataKey = "pagerduty"

	// BambooMetadataKey is the key we use to store a list of allowed builds
	BambooMetadataKey = "bamboo"
)

// GetPagerDutyMetadata reads the pagerduty metadata out of a service
func GetPagerDutyMetadata(serviceCentralData *ServiceData) (*creator_v1.PagerDutyMetadata, error) {
	var m creator_v1.PagerDutyMetadata
	found, err := unmarshalFromMiscData(serviceCentralData, PagerDutyMetadataKey, &m)
	if err != nil || !found {
		return nil, err
	}
	return &m, nil
}

// SetPagerDutyMetadata stores the metadata for pagerduty into a Service's metadata
func SetPagerDutyMetadata(serviceCentralData *ServiceData, m *creator_v1.PagerDutyMetadata) error {
	return setMetadata(serviceCentralData, PagerDutyMetadataKey, m)
}

// GetBambooMetadata reads the allowed builds metadata out of a service
func GetBambooMetadata(serviceCentralData *ServiceData) (*creator_v1.BambooMetadata, error) {
	var m creator_v1.BambooMetadata
	found, err := unmarshalFromMiscData(serviceCentralData, BambooMetadataKey, &m)
	if err != nil || !found {
		return nil, err
	}
	return &m, nil
}

// SetBambooMetadata stores the metadata for allowed builds into a Service's metadata
func SetBambooMetadata(serviceCentralData *ServiceData, m *creator_v1.BambooMetadata) error {
	return setMetadata(serviceCentralData, BambooMetadataKey, m)
}

func unmarshalFromMiscData(serviceCentralData *ServiceData, key string, res interface{}) (bool, error) {
	raw, err := GetMiscData(serviceCentralData, key)
	if err != nil {
		return false, err
	}
	if raw == "" {
		return false, nil
	}
	err = json.Unmarshal([]byte(raw), res)
	if err != nil {
		return false, err
	}
	return true, nil
}

func setMetadata(serviceCentralData *ServiceData, key string, m interface{}) error {
	if m == nil {
		return nil
	}
	bytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return SetMiscData(serviceCentralData, key, string(bytes))
}
