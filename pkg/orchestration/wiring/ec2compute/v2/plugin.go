package v2

import (
	"encoding/json"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/voyager"
	orch_v1 "github.com/atlassian/voyager/pkg/apis/orchestration/v1"
	compute_common "github.com/atlassian/voyager/pkg/orchestration/wiring/compute"
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
	"environment":      {},
	"environment_type": {},
	"service_name":     {},
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

func constructComputeParameters(origSpec *runtime.RawExtension, iamRoleRef, iamInstProfRef smith_v1.Reference, microsServiceName string, stateContext wiringplugin.StateContext) (*runtime.RawExtension, bool /* external */, bool /* retriable */, error) {
	// The user shouldn't be setting anything in our 'restrictedParameters', since
	// _we_ control it. So let's make sure they're not and fail ASAP.
	var parametersCheck restrictedParameters
	if err := json.Unmarshal(origSpec.Raw, &parametersCheck); err != nil {
		return nil, false, false, errors.Wrap(err, "can't unmarshal state spec into JSON object")
	}
	if parametersCheck != (restrictedParameters{}) {
		// User provided something weird in the spec
		return nil, true, false, errors.Errorf("at least one autowired value not empty: %+v", parametersCheck)
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
	sharedDefaultEnvVars := compute_common.GetSharedDefaultEnvVars(stateContext.Location)
	partialSpecData.Docker.EnvVars = make(map[string]string, len(sharedDefaultEnvVars))
	for _, v := range sharedDefaultEnvVars {
		partialSpecData.Docker.EnvVars[v.Name] = v.Value
	}

	// convert partialSpec to map
	var partialSpecMap map[string]interface{}
	partialSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&partialSpecData)
	if err != nil {
		return nil, false, false, errors.WithStack(err)
	}

	var finalSpec map[string]interface{}
	if err = json.Unmarshal(origSpec.Raw, &finalSpec); err != nil {
		return nil, false, false, errors.Wrap(err, "failed to parse user spec")
	}

	wiringutil.StripJSONFields(finalSpec, common.StateComputeSpec{})

	// merge user spec and partial spec
	finalSpec, err = wiringutil.Merge(finalSpec, partialSpecMap)
	if err != nil {
		return nil, false, false, err
	}

	rawExtension, err := util.ToRawExtension(finalSpec)
	if err != nil {
		return nil, false, false, err
	}

	return rawExtension, false, false, nil
}

func WireUp(stateResource *orch_v1.StateResource, context *wiringplugin.WiringContext) wiringplugin.WiringResult {
	if stateResource.Type != ResourceType {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Errorf("invalid resource type: %q", stateResource.Type),
		}
	}

	if stateResource.Spec == nil {
		return &wiringplugin.WiringResultFailure{
			Error: errors.New("resource spec must be provided"),
		}
	}

	userInput := userInputSpec{}
	if err := json.Unmarshal(stateResource.Spec.Raw, &userInput); err != nil {
		return &wiringplugin.WiringResultFailure{
			Error: errors.Wrap(err, "failed to parse user spec"),
		}
	}

	return common.WireUp(userInput.Service.ID, ec2ComputePlanName, stateResource, context, constructComputeParameters)
}
