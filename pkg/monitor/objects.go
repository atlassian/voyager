package monitor

import (
	"encoding/json"
	"time"

	composition_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
)

const (
	pollDelay = 5 * time.Second
)

func buildServiceDescriptor(sdInput string) (*composition_v1.ServiceDescriptor, error) {
	var sd *composition_v1.ServiceDescriptor
	if err := json.Unmarshal([]byte(sdInput), &sd); err != nil {
		return nil, err
	}
	return sd, nil
}
