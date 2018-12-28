package replication

import (
	"context"
	"fmt"
	"testing"
	"time"

	apis_composition "github.com/atlassian/voyager/pkg/apis/composition"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	authn_v1 "k8s.io/api/authentication/v1"
	authz_v1 "k8s.io/api/authorization/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreclient_fake "k8s.io/client-go/kubernetes/fake"
	kube_testing "k8s.io/client-go/testing"
)

func admitAuthzWithContextAndLogger(t *testing.T, admissionRequest *admissionv1beta1.AdmissionRequest, resultStatus authz_v1.SubjectAccessReviewStatus) (*admissionv1beta1.AdmissionResponse, []kube_testing.Action, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	nsClient := coreclient_fake.NewSimpleClientset()
	// Return stub SubjectAccessReview result
	nsClient.PrependReactor("create", "subjectaccessreviews", func(action kube_testing.Action) (bool, runtime.Object, error) {
		createAction := action.(kube_testing.CreateAction)
		obj := createAction.GetObject()
		sar := obj.(*authz_v1.SubjectAccessReview).DeepCopy()
		sar.Status = resultStatus
		return true, sar, nil
	})

	ac := AdmissionContext{
		AuthzClient: nsClient.AuthorizationV1().SubjectAccessReviews(),
	}

	response, err := ac.servicedescriptorAuthzAdmitFunc(ctx, zaptest.NewLogger(t), admissionv1beta1.AdmissionReview{Request: admissionRequest})
	return response, nsClient.Actions(), err
}

func assertSubjectAccessReview(t *testing.T, allowed bool, reason string) {
	ar, actions, err := admitAuthzWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		UserInfo: authn_v1.UserInfo{
			Username: "user",
			Groups:   []string{"group1"},
			Extra:    map[string]authn_v1.ExtraValue{"foo": {"bar"}},
		},
		Operation: admissionv1beta1.Create,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: "my-service",
			},
			TypeMeta: meta_v1.TypeMeta{
				Kind: "servicedescriptor",
			},
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{},
			},
		}),
	}, authz_v1.SubjectAccessReviewStatus{
		Allowed: allowed,
		Reason:  reason,
	})

	require.NoError(t, err)
	assert.Equal(t, allowed, ar.Allowed)
	assert.Equal(t, reason, ar.Result.Message)

	require.Len(t, actions, 1)
	reviewRequest := actions[0].(kube_testing.CreateAction).GetObject().(*authz_v1.SubjectAccessReview)
	assert.Equal(t, "user", reviewRequest.Spec.User)
	assert.Equal(t, []string{"group1"}, reviewRequest.Spec.Groups)
	assert.Equal(t, map[string]authz_v1.ExtraValue{"foo": {"bar"}}, reviewRequest.Spec.Extra)
	assert.Equal(t, &authz_v1.ResourceAttributes{
		Group:     apis_composition.GroupName,
		Resource:  comp_v1.ServiceDescriptorResourcePlural,
		Namespace: meta_v1.NamespaceNone, // cluster-scoped
		Name:      "my-service",
		Verb:      k8s.ServiceDescriptorClaimVerb,
	}, reviewRequest.Spec.ResourceAttributes)
}

func TestAuthzCreateAllow(t *testing.T) {
	t.Parallel()
	assertSubjectAccessReview(t, true, `RBAC: allowed by ClusterRoleBinding "paas:composition:servicedescriptor:foo:crud" of ClusterRole "paas:composition:servicedescriptor:foo:crud"" to Group "paas-foo-dl-dev"`)
}

func TestAuthzCreateForbid(t *testing.T) {
	t.Parallel()
	assertSubjectAccessReview(t, false, `RBAC: user not allowed to create ServiceDescriptors with name "my-service"`)
}

func assertOperationRejected(t *testing.T, operation admissionv1beta1.Operation) {
	ar, actions, err := admitAuthzWithContextAndLogger(t, &admissionv1beta1.AdmissionRequest{
		Operation: operation,
		Resource:  sdResource,
		Object: objectToRawExtension(t, &comp_v1.ServiceDescriptor{
			Spec: comp_v1.ServiceDescriptorSpec{
				Locations: []comp_v1.ServiceDescriptorLocation{},
			},
		}),
	}, authz_v1.SubjectAccessReviewStatus{})
	require.NoError(t, err)
	require.Equal(t, int32(400), ar.Result.Code)
	require.Equal(t, fmt.Sprintf("unsupported operation %q", string(operation)), ar.Result.Message)
	require.Empty(t, actions)
}

func TestAuthzRejectsUpdate(t *testing.T) {
	t.Parallel()
	assertOperationRejected(t, admissionv1beta1.Update)
}

func TestAuthzRejectsDelete(t *testing.T) {
	t.Parallel()
	assertOperationRejected(t, admissionv1beta1.Delete)
}
