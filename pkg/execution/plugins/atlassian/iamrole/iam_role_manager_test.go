package iamrole

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/voyager"
	"github.com/atlassian/voyager/pkg/orchestration/wiring/wiringutil/oap"
	"github.com/atlassian/voyager/pkg/util/testutil"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const fixturesDir = "testdata"

func TestFixtures(t *testing.T) {
	t.Parallel()

	files, errRead := filepath.Glob(filepath.Join(fixturesDir, "*.policySnippets.yaml"))
	require.NoError(t, errRead)
	for _, file := range files {
		t.Run(string(EC2ComputeType)+"/"+file, func(t *testing.T) {
			testFixture(t, EC2ComputeType, file)
		})

		t.Run(string(KubeComputeType)+"/"+file, func(t *testing.T) {
			testFixture(t, KubeComputeType, file)
		})
	}
}

func TestGenerateTemplate(t *testing.T) {
	t.Parallel()
	var serviceNameHack voyager.ServiceName = "aNameLongerThan37CharactersNotTHEREYet"
	policyBytes, err := json.MarshalIndent([]IamPolicy{
		{
			PolicyName: "voyager-merge",
			PolicyDocument: IamPolicyDocument{
				ID:        "wee",
				Version:   "2",
				Statement: nil,
			},
		},
		defaultJSON(serviceNameHack),
	}, prettyPrintIndent, "  ")
	assert.NoError(t, err)
	managedPolicyBytes, err := json.Marshal([]string{
		"arn:aws:iam::123456789012:policy/micros-iam-DefaultServicePolicy-11210HMV0LWK",
	})
	assert.NoError(t, err)

	iamAssumeRoleStatementBytes, err := generateIamAssumeRoleStatements(EC2ComputeType, []string{
		"arn:aws:iam::123456789012:role/micros-server-iam-MicrosServer-UTMFBJ2IWZSK",
	})

	t.Run("ec2 compute type with instance profile", func(t *testing.T) {
		actual, err := buildTemplate(policyBytes, managedPolicyBytes, iamAssumeRoleStatementBytes, true)
		assert.NoError(t, err)

		fileName := filepath.Join(fixturesDir, "templated_policy_profile.json")

		testutil.ReadCompare(t, testutil.FileName(fileName), testutil.FileName("actual"), actual)
	})

	t.Run("ec2 compute type without instance profile", func(t *testing.T) {
		actual, err := buildTemplate(policyBytes, managedPolicyBytes, iamAssumeRoleStatementBytes, false)
		assert.NoError(t, err)

		fileName := filepath.Join(fixturesDir, "templated_policy_no_profile.json")

		testutil.ReadCompare(t, testutil.FileName(fileName), testutil.FileName("actual"), actual)
	})
}

func TestGenerateRoleInstance(t *testing.T) {
	t.Parallel()

	spec := &Spec{
		ServiceName:     "test-svc-app",
		OAPResourceName: "app-iamrole",
		ServiceEnvironment: oap.ServiceEnvironment{
			NotificationEmail: "an_owner@example.com",
			AlarmEndpoints: []oap.MicrosAlarmSpec{
				{
					Type:     "CloudWatch",
					Priority: "high",
					Endpoint: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/123",
					Consumer: "pagerduty",
				},
				{
					Type:     "CloudWatch",
					Priority: "low",
					Endpoint: "https://events.pagerduty.com/adapter/cloudwatch_sns/v1/456",
					Consumer: "pagerduty",
				},
			},
			Tags: map[voyager.Tag]string{
				"business_unit":    "some_unit",
				"environment":      "ddev",
				"environment_type": "dev",
				"platform":         "voyager",
				"resource_owner":   "an_owner",
				"service_name":     "test-svc",
			},
			PrimaryVpcEnvironment: &oap.VPCEnvironment{
				AppSubnets:         []string{"subnet-93baa4e7", "subnet-8b11e2ee"},
				PrivateDNSZone:     "domain.dev.atl-inf.io",
				PrivatePaasDNSZone: "ap-southeast-2.dev.paas-inf.net",
				Region:             "ap-southeast-2",
				VPCID:              "vpc-c545a8a0",
				Zones:              []string{"ap-southeast-2a", "ap-southeast-2b"},
			},
		},
		AssumeRoles: []string{
			"arn:aws:iam::123456789012:role/micros-server-iam-MicrosServer-UTMFBJ2IWZSK",
			"arn:aws:iam::123456789012:role/other-role",
		},
	}

	t.Run("EC2 compute", func(t *testing.T) {
		spec.ComputeType = EC2ComputeType
		spec.CreateInstanceProfile = true
		spec.ManagedPolicies = []string{"arn:aws:iam::123456789012:policy/micros-iam-DefaultServicePolicy-11210HMV0LWK"}

		actualSI, err := generateRoleInstance(spec)
		require.NoError(t, err)

		verifyServiceInstance(t, "iam_role_service_instance_ec2_compute.yaml", actualSI)
	})

	t.Run("Kube compute", func(t *testing.T) {
		spec.ComputeType = KubeComputeType
		spec.CreateInstanceProfile = false
		spec.ManagedPolicies = nil

		actualSI, err := generateRoleInstance(spec)
		require.NoError(t, err)

		verifyServiceInstance(t, "iam_role_service_instance_kube_compute.yaml", actualSI)
	})
}

func verifyServiceInstance(t *testing.T, expectedDataFileName string, actualSI *sc_v1b1.ServiceInstance) {
	expectedData, err := ioutil.ReadFile(filepath.Join(fixturesDir, expectedDataFileName))
	require.NoError(t, err)
	var expectedSI sc_v1b1.ServiceInstance
	err = yaml.UnmarshalStrict(expectedData, &expectedSI)
	require.NoError(t, err)

	// compare templateBody and the rest separately, because templateBody is JSON so whitespaces don't matter
	// this is just a convenience step, fix templateBody differences first
	testutil.JSONCompare(t, getTemplateBodyAsJSONObject(t, &expectedSI), getTemplateBodyAsJSONObject(t, actualSI))

	// compare everything
	testutil.YAMLCompare(t, expectedSI, actualSI)
}

func testFixture(t *testing.T, computeType ComputeType, file string) {
	rawSpec := map[string]interface{}{
		"tags": map[string]interface{}{
			"test": "1",
		},
		"computeType": computeType,
		"environment": "pdev",
		"serviceId":   "never",
		"assumeRoles": []string{"arn:aws:iam::123456789012:role/micros-server-iam-MicrosServer-UTMFBJ2IWZSK"},
	}

	if computeType == EC2ComputeType {
		rawSpec["managedPolicies"] = []string{"arn:aws:iam::123456789012:policy/micros-iam-DefaultServicePolicy-11210HMV0LWK"}
	}

	// Read the policy snippets and write it into the rawspec
	policySnippets, err := ioutil.ReadFile(file)
	require.NoError(t, err)
	spec := Spec{}
	err = yaml.UnmarshalStrict(policySnippets, &spec)
	require.NoError(t, err)
	rawSpec["policySnippets"] = spec.PolicySnippets

	var resultFilePrefix string
	{
		_, filename := filepath.Split(file)
		bundleFileName := strings.Split(filename, ".")
		resultFilePrefix = filepath.Join(fixturesDir, strings.Join(bundleFileName[:len(bundleFileName)-2], "."))
	}

	// Try to run/compare both 'valid output' (service_instance_spec) and error file.
	iamRolePlugin, err := New()
	require.NoError(t, err)

	context := smith_plugin.Context{
		Dependencies: createDependencies(),
	}

	processResult := iamRolePlugin.Process(rawSpec, &context)

	resultFilePostFix := ".iam_template_ec2_compute.json"
	if computeType == KubeComputeType {
		resultFilePostFix = ".iam_template_kube_compute.json"
	}

	filename := resultFilePrefix + resultFilePostFix
	data, errSuccess := ioutil.ReadFile(filename)
	if errSuccess == nil {
		require.Equal(t, smith_plugin.ProcessResultSuccessType, processResult.StatusType())

		// turn the processResult into a serviceInstance
		serviceInstance := processResult.(*smith_plugin.ProcessResultSuccess).Object.(*sc_v1b1.ServiceInstance)
		validateServiceInstance(t, serviceInstance, data, filename)
	}

	data, errFailure := ioutil.ReadFile(resultFilePrefix + ".error")
	if errFailure == nil {
		require.Equal(t, smith_plugin.ProcessResultFailureType, processResult.StatusType())
		require.EqualError(t, processResult.(*smith_plugin.ProcessResultFailure).Error, strings.TrimSpace(string(data)))
	}

	if errFailure != nil && errSuccess != nil {
		t.Errorf("Must have either error or service_instance_spec file for input %q (%+v, %+v)", file, errFailure, errSuccess)
	}
}

func validateServiceInstance(t *testing.T, serviceInstance *sc_v1b1.ServiceInstance, expectedOutputBundle []byte, filename string) {
	// Check fixed aspects of ServiceInstance object
	assert.Equal(t, "ServiceInstance", serviceInstance.TypeMeta.Kind)
	assert.Equal(t, "servicecatalog.k8s.io/v1beta1", serviceInstance.TypeMeta.APIVersion)
	assert.Equal(t, cloudformationServiceID, serviceInstance.Spec.PlanReference.ClusterServiceClassExternalID)
	assert.Equal(t, cloudformationPlanID, serviceInstance.Spec.PlanReference.ClusterServicePlanExternalID)

	// Now check parameterised template
	var expectedTemplateBody map[string]interface{}
	require.NoError(t, json.Unmarshal(expectedOutputBundle, &expectedTemplateBody))
	testutil.JSONCompare(t, expectedTemplateBody, getTemplateBodyAsJSONObject(t, serviceInstance))

	// And rest of parameters
	expectedParams := oap.ServiceInstanceSpec{
		ServiceName: "never",
		Resource: oap.RPSResource{
			Type:       "cloudformation",
			Attributes: nil,
		},
	}
	actualParams := oap.ServiceInstanceSpec{}
	require.NoError(t, json.Unmarshal(serviceInstance.Spec.Parameters.Raw, &actualParams))
	actualParams.Resource.Attributes = nil
	testutil.JSONCompare(t, expectedParams, actualParams)
}

func getTemplateBodyAsJSONObject(t *testing.T, si *sc_v1b1.ServiceInstance) *map[string]interface{} {
	var parameters oap.ServiceInstanceSpec
	err := yaml.UnmarshalStrict(si.Spec.Parameters.Raw, &parameters)
	require.NoError(t, err)
	var attributes CfnAttributes
	err = yaml.UnmarshalStrict(parameters.Resource.Attributes, &attributes)
	require.NoError(t, err)
	var templateBody map[string]interface{}
	err = json.Unmarshal([]byte(attributes.TemplateBody), &templateBody)
	require.NoError(t, err)
	return &templateBody
}

func createDependencies() map[smith_v1.ResourceName]smith_plugin.Dependency {
	// This creates an empty map. The reason it does so is because while the
	// plugin *should* be passed dependencies, the dependencies are ignored.
	return map[smith_v1.ResourceName]smith_plugin.Dependency{}
}
