package v2

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/asapkey"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/ec2compute/common"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringplugin"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceType voyager.ResourceType = "EC2Compute"

	ec2ComputePlanName = "v2"
)

// HACK: Some tags the EC2 provider doesn't like, because it wants to
// set them itself... (NB handles business_unit/resource_owner separately)
// We only really worry about the tags that we're likely to set here
// (it's ok if the user errors out from the provider).
var forbiddenTags = map[string]struct{}{
	"environment":      struct{}{},
	"environment_type": struct{}{},
	"service_name":     struct{}{},
}

type userInputSpec struct {
	Service service `json:"service"`
}

type Docker struct {
	EnvVars map[string]string `json:"envVars"`
}

// fields that the auto wiring function manipulates
type partialSpec struct {
	Service        service               `json:"service"`
	Location       voyager.Location      `json:"location"`
	EC2            ec2Iam                `json:"ec2"`
	Tags           map[string]string     `json:"tags"`
	Notifications  notifications         `json:"notifications"`
	SecretEnvVars  map[string]string     `json:"secretEnvVars,omitempty"`
	Docker         Docker                `json:"docker"`
	AlarmEndpoints []oap.MicrosAlarmSpec `json:"alarmEndpoints"`
}

type service struct {
	ID              string `json:"id"`
	LoggingID       string `json:"loggingId"`
	SsamAccessLevel string `json:"ssamAccessLevel"`
}

type ec2Iam struct {
	IamRoleArn            string `json:"iamRoleArn"`
	IamInstanceProfileArn string `json:"iamInstanceProfileArn"`
}

type notifications struct {
	Email string `json:"email"`
}

// restrictedParameters contains the parts of the output compute spec users cannot set
// because we automatically generate them and don't allow overrides.
type restrictedParameters struct {
	Location voyager.Location `json:"location"`
	// SecretEnvVars is a pointer so we can do == comparisons against an empty object
	// (otherwise we will fail to compare maps).
	SecretEnvVars *map[string]string `json:"secretEnvVars,omitempty"`
	EC2           ec2Iam             `json:"ec2"`
}

func constructComputeParameters(origSpec *runtime.RawExtension, iamRoleRef, iamInstProfRef smith_v1.Reference, microsServiceName string, stateContext wiringplugin.StateContext, defaultEnvVars map[string]string) (*runtime.RawExtension, error) {
	// The user shouldn't be setting anything in our 'restrictedParameters', since
	// _we_ control it. So let's make sure they're not and fail ASAP.
	var parametersCheck restrictedParameters
	if err := json.Unmarshal(origSpec.Raw, &parametersCheck); err != nil {
		return nil, errors.Wrap(err, "can't unmarshal state spec into JSON object")
	}
	if parametersCheck != (restrictedParameters{}) {
		return nil, errors.Errorf("at least one autowired value not empty: %+v", parametersCheck)
	}

	// generate partialSpec

	var partialSpecData partialSpec
	// service param
	partialSpecData.Service = service{
		ID:              microsServiceName,
		LoggingID:       stateContext.ServiceProperties.LoggingID,
		SsamAccessLevel: stateContext.ServiceProperties.SSAMAccessLevel,
	}

	// --- location param
	partialSpecData.Location = stateContext.Location

	// --- ec2 param
	partialSpecData.EC2 = ec2Iam{
		IamRoleArn:            iamRoleRef.Ref(),
		IamInstanceProfileArn: iamInstProfRef.Ref(),
	}

	// --- tags params
	partialSpecData.Tags = make(map[string]string, len(stateContext.Tags))
	for k, v := range stateContext.Tags {
		if _, forbidden := forbiddenTags[string(k)]; !forbidden {
			partialSpecData.Tags[string(k)] = v
		}
	}

	// --- notificationProp params
	notificationProp := stateContext.ServiceProperties.Notifications
	partialSpecData.Notifications = notifications{
		Email: notificationProp.Email,
	}
	partialSpecData.AlarmEndpoints = oap.PagerdutyAlarmEndpoints(
		notificationProp.PagerdutyEndpoint.CloudWatch, notificationProp.LowPriorityPagerdutyEndpoint.CloudWatch)

	// --- default ASAP public key repo env vars
	asapKeyPublicRepositoryEnvVars := asapkey.GetPublicKeyRepoEnvVars(stateContext.Location)
	partialSpecData.Docker.EnvVars = make(map[string]string, len(asapKeyPublicRepositoryEnvVars)+len(defaultEnvVars))
	for k, v := range defaultEnvVars {
		partialSpecData.Docker.EnvVars[k] = v
	}
	for _, v := range asapKeyPublicRepositoryEnvVars {
		partialSpecData.Docker.EnvVars[v.Name] = v.Value
	}

	// convert partialSpec to map
	var partialSpecMap map[string]interface{}
	partialSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&partialSpecData)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var finalSpec map[string]interface{}
	if err = json.Unmarshal(origSpec.Raw, &finalSpec); err != nil {
		return nil, errors.Wrap(err, "failed to parse user spec")
	}

	wiringutil.StripJSONFields(finalSpec, common.StateComputeSpec{})

	// merge user spec and partial spec
	finalSpec, err = wiringutil.Merge(finalSpec, partialSpecMap)
	if err != nil {
		return nil, err
	}

	return util.ToRawExtension(finalSpec)
}

func WireUp(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) (*wiringplugin.WiringResult, bool, error) {
	if stateResource.Type != ResourceType {
		return nil, false, errors.Errorf("invalid resource type: %q", stateResource.Type)
	}

	if stateResource.Spec == nil {
		return nil, false, errors.New("resource spec must be provided")
	}

	userInput := userInputSpec{}
	if err := json.Unmarshal(stateResource.Spec.Raw, &userInput); err != nil {
		return nil, false, errors.Wrap(err, "failed to parse user spec")
	}

	return common.WireUp(userInput.Service.ID, ec2ComputePlanName, stateResource, context, constructComputeParameters)
}
