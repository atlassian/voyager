package updater

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

const (
	testApiVersion = "objectupdater.test/v1"
	testKind       = "fakeKind"
	testNamespace  = "fakeNamespace"
	testName       = "fakeName"
)

type mockIndexer struct {
	mock.Mock
}

func (m *mockIndexer) Add(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *mockIndexer) Update(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *mockIndexer) Delete(obj interface{}) error {
	args := m.Called(obj)
	return args.Error(0)
}

func (m *mockIndexer) List() []interface{} {
	args := m.Called()
	return args.Get(0).([]interface{})
}

func (m *mockIndexer) ListKeys() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *mockIndexer) Get(obj interface{}) (item interface{}, exists bool, err error) {
	args := m.Called(obj)
	return args.Get(0), args.Bool(1), args.Error(2)
}

func (m *mockIndexer) GetByKey(key string) (item interface{}, exists bool, err error) {
	args := m.Called(key)
	return args.Get(0), args.Bool(1), args.Error(2)
}

func (m *mockIndexer) Replace(a []interface{}, b string) error {
	args := m.Called(a, b)
	return args.Error(0)
}

func (m *mockIndexer) Resync() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockIndexer) Index(indexName string, obj interface{}) ([]interface{}, error) {
	args := m.Called(indexName, obj)
	return args.Get(0).([]interface{}), args.Error(1)
}

func (m *mockIndexer) IndexKeys(indexName string, indexKey string) ([]string, error) {
	args := m.Called(indexName, indexKey)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockIndexer) ListIndexFuncValues(indexName string) []string {
	args := m.Called(indexName)
	return args.Get(0).([]string)
}

func (m *mockIndexer) ByIndex(indexName string, indexKey string) ([]interface{}, error) {
	args := m.Called(indexName, indexKey)
	return args.Get(0).([]interface{}), args.Error(1)
}

func (m *mockIndexer) GetIndexers() cache.Indexers {
	args := m.Called()
	return args.Get(0).(cache.Indexers)
}

func (m *mockIndexer) AddIndexers(newIndexers cache.Indexers) error {
	args := m.Called(newIndexers)
	return args.Error(0)
}

type mockClient struct {
	mock.Mock
}

func (m *mockClient) Create(ns string, obj runtime.Object) (runtime.Object, error) {
	args := m.Called(ns, obj)
	arg0, _ := args.Get(0).(runtime.Object)
	return arg0, args.Error(1)
}

func (m *mockClient) Update(ns string, obj runtime.Object) (runtime.Object, error) {
	args := m.Called(ns, obj)
	arg0, _ := args.Get(0).(runtime.Object)
	return arg0, args.Error(1)
}

func (m *mockClient) Delete(ns string, name string, options *meta_v1.DeleteOptions) error {
	args := m.Called(ns, name, options)
	return args.Error(0)
}

type mockSpecCheck struct {
	mock.Mock
}

func (m *mockSpecCheck) CompareActualVsSpec(logger *zap.Logger, spec runtime.Object, actual runtime.Object) (*unstructured.Unstructured, bool, string, error) {
	args := m.Called(logger, spec, actual)
	arg0, _ := args.Get(0).(*unstructured.Unstructured)
	return arg0, args.Bool(1), args.String(2), args.Error(3)
}

type testCase struct {
	fakeIndexer   *mockIndexer
	fakeClient    *mockClient
	fakeSpecCheck *mockSpecCheck

	objectUpdater *ObjectUpdater
}

func newTestCase() *testCase {
	fakeIndexer := new(mockIndexer)
	fakeClient := new(mockClient)
	fakeSpecCheck := new(mockSpecCheck)

	updater := &ObjectUpdater{
		ExistingObjectsIndexer: fakeIndexer,
		Client:                 fakeClient,
		SpecCheck:              fakeSpecCheck,
	}

	return &testCase{
		fakeIndexer:   fakeIndexer,
		fakeClient:    fakeClient,
		fakeSpecCheck: fakeSpecCheck,
		objectUpdater: updater,
	}
}

func fakeObject(apiVersion, kind, namespace, name string, data interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			},
			"spec": data,
		},
	}
}

func TestObjectUpdate(t *testing.T) {
	t.Parallel()

	tc := newTestCase()

	// given
	desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "desired")
	existingObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "existing")

	// when
	tc.fakeIndexer.On("Get", desiredObject).Return(existingObject, true, nil)
	tc.fakeSpecCheck.On("CompareActualVsSpec", mock.Anything, desiredObject, existingObject).Return(desiredObject, false, "", nil)
	tc.fakeClient.On("Update", mock.Anything, desiredObject).Return(desiredObject, nil)

	conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
	require.NoError(t, err)
	assert.False(t, conflict)
	assert.False(t, retriable)
	assert.Equal(t, desiredObject, result)
}

func TestObjectUpdateNoDifference(t *testing.T) {
	t.Parallel()

	tc := newTestCase()

	// given
	desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "same")
	existingObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "same")

	// when
	tc.fakeIndexer.On("Get", desiredObject).Return(existingObject, true, nil)
	tc.fakeSpecCheck.On("CompareActualVsSpec", mock.Anything, desiredObject, existingObject).Return(existingObject, true, "", nil)

	conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
	require.NoError(t, err)
	assert.False(t, conflict)
	assert.False(t, retriable)
	assert.Equal(t, desiredObject, result)
}

func TestErrorCompareActualVsSpec(t *testing.T) {
	t.Parallel()

	tc := newTestCase()

	// given
	desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "same")
	existingObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "same")

	// when
	tc.fakeIndexer.On("Get", desiredObject).Return(existingObject, true, nil)
	tc.fakeSpecCheck.On("CompareActualVsSpec", mock.Anything, desiredObject, existingObject).Return(nil, true, "", errors.New("error"))

	conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
	require.Error(t, err)
	assert.False(t, conflict)
	assert.False(t, retriable)
	assert.Nil(t, result)
}

func TestErrorGet(t *testing.T) {
	t.Parallel()

	tc := newTestCase()

	// given
	desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "same")

	// when
	tc.fakeIndexer.On("Get", desiredObject).Return(nil, false, errors.New("error"))

	conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
	require.Error(t, err)
	assert.False(t, conflict)
	assert.False(t, retriable)
	assert.Nil(t, result)
}

func TestObjectCreate(t *testing.T) {
	t.Parallel()

	tc := newTestCase()

	// given
	desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "desired")

	// when
	tc.fakeIndexer.On("Get", desiredObject).Return(nil, false, nil)
	tc.fakeClient.On("Create", mock.Anything, desiredObject).Return(desiredObject, nil)

	conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
	require.NoError(t, err)
	assert.False(t, conflict)
	assert.False(t, retriable)
	assert.Equal(t, desiredObject, result)
}

func TestObjectCreateErrorTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		Error             error
		ShouldBeConflict  bool
		ShouldBeRetriable bool
	}{
		"AlreadyExists":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonAlreadyExists}}, true, false},
		"BadRequest":           {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonBadRequest}}, false, false},
		"Expired":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonExpired}}, false, false},
		"Forbidden":            {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonForbidden}}, false, true},
		"InternalError":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonInternalError}}, false, true},
		"Invalid":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonInvalid}}, false, false},
		"MethodNotAllowed":     {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonMethodNotAllowed}}, false, false},
		"NotAcceptable":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonNotAcceptable}}, false, false},
		"ServerTimeout":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonServerTimeout}}, false, true},
		"ServiceUnavailable":   {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonServiceUnavailable}}, false, true},
		"Timeout":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonTimeout}}, false, true},
		"TooManyRequests":      {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonTooManyRequests}}, false, true},
		"Unauthorized":         {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonUnauthorized}}, false, true},
		"Unknown":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonUnknown}}, false, true},
		"UnsupportedMediaType": {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonUnsupportedMediaType}}, false, false},
		// Can not throw Conflict so not tested here
	}

	for name, c := range cases {
		shouldBeConflict := c.ShouldBeConflict
		shouldBeRetriable := c.ShouldBeRetriable
		errorToThrow := c.Error

		t.Run(name, func(t *testing.T) {
			tc := newTestCase()

			// given
			desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "desired")

			// when
			tc.fakeIndexer.On("Get", desiredObject).Return(nil, false, nil)
			tc.fakeClient.On("Create", mock.Anything, desiredObject).Return(nil, errorToThrow)

			conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
			require.Error(t, err)
			assert.Equal(t, shouldBeConflict, conflict, "conflict return is incorrect")
			assert.Equal(t, shouldBeRetriable, retriable, "retriable return is incorrect")
			assert.Nil(t, result)
		})
	}
}

func TestObjectUpdateErrorTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		Error             error
		ShouldBeConflict  bool
		ShouldBeRetriable bool
	}{
		"Conflict":             {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonConflict}}, true, false},
		"BadRequest":           {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonBadRequest}}, false, false},
		"Expired":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonExpired}}, false, false},
		"Forbidden":            {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonForbidden}}, false, true},
		"InternalError":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonInternalError}}, false, true},
		"Invalid":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonInvalid}}, false, false},
		"MethodNotAllowed":     {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonMethodNotAllowed}}, false, false},
		"NotAcceptable":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonNotAcceptable}}, false, false},
		"ServerTimeout":        {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonServerTimeout}}, false, true},
		"ServiceUnavailable":   {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonServiceUnavailable}}, false, true},
		"Timeout":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonTimeout}}, false, true},
		"TooManyRequests":      {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonTooManyRequests}}, false, true},
		"Unauthorized":         {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonUnauthorized}}, false, true},
		"Unknown":              {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonUnknown}}, false, true},
		"UnsupportedMediaType": {&api_errors.StatusError{meta_v1.Status{Reason: meta_v1.StatusReasonUnsupportedMediaType}}, false, false},
		// Can not throw AlreadyExists so not tested here
	}

	for name, c := range cases {
		shouldBeConflict := c.ShouldBeConflict
		shouldBeRetriable := c.ShouldBeRetriable
		errorToThrow := c.Error

		t.Run(name, func(t *testing.T) {
			tc := newTestCase()

			// given
			desiredObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "desired")
			existingObject := fakeObject(testApiVersion, testKind, testNamespace, testName, "existing")

			// when
			tc.fakeIndexer.On("Get", desiredObject).Return(existingObject, true, nil)
			tc.fakeSpecCheck.On("CompareActualVsSpec", mock.Anything, desiredObject, existingObject).Return(desiredObject, false, "", nil)
			tc.fakeClient.On("Update", mock.Anything, desiredObject).Return(nil, errorToThrow)

			conflict, retriable, result, err := tc.objectUpdater.CreateOrUpdate(zaptest.NewLogger(t), nil, desiredObject)
			require.Error(t, err)
			assert.Equal(t, shouldBeConflict, conflict, "conflict return is incorrect")
			assert.Equal(t, shouldBeRetriable, retriable, "retriable return is incorrect")
			assert.Nil(t, result)
		})
	}
}
