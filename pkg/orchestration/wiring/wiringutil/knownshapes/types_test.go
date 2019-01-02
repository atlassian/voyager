package knownshapes

import "github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"

var (
	_ wiringplugin.Shape = &BindableEnvironmentVariables{}
	_ wiringplugin.Shape = &BindableIamAccessible{}
)
