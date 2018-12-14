package validation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	validator "gopkg.in/go-playground/validator.v9"
)

// Purely a helper for tests
type fieldErrorMeta struct {
	Tag             string
	StructNamespace string
}

type fieldErrorMetaArray []fieldErrorMeta

func (a fieldErrorMetaArray) Contains(err validator.FieldError) bool {
	for _, m := range a {
		if m.Tag == err.Tag() && m.StructNamespace == err.StructNamespace() {
			return true
		}
	}

	return false
}

func (a fieldErrorMetaArray) Check(t *testing.T, err error) {
	errs, ok := err.(validator.ValidationErrors)
	require.True(t, ok)
	require.Len(t, errs, len(a))
	for _, err := range errs {
		require.True(t, a.Contains(err))
	}
}

func TestRequiredNotValid(t *testing.T) {
	t.Parallel()
	type TheUndercity struct {
		UndergroundMap  map[string]string `validate:"required"`
		TheWarQuarter   interface{}       `validate:"required"`
		TheMagicQuarter interface{}       `validate:"required"`
	}

	uc := &TheUndercity{
		UndergroundMap:  nil,
		TheWarQuarter:   nil,
		TheMagicQuarter: (interface{})(map[string]int(nil)),
	}

	expected := fieldErrorMetaArray{
		{
			Tag:             "required",
			StructNamespace: "TheUndercity.UndergroundMap",
		},
		{
			Tag:             "required",
			StructNamespace: "TheUndercity.TheWarQuarter",
		},
		{
			Tag:             "required",
			StructNamespace: "TheUndercity.TheMagicQuarter",
		},
	}

	v := New()
	err := v.Validate(uc)
	expected.Check(t, err)
}

func TestDoubleDashNotValid(t *testing.T) {
	t.Parallel()
	type Ferry struct {
		Name string `validate:"no_double_dash"`
	}

	expected := fieldErrorMetaArray{
		{
			Tag:             "no_double_dash",
			StructNamespace: "Ferry.Name",
		},
	}

	testCases := []string{
		"--", // hyphen-minus (AKA dash)
		"Ferry--McFerryface",
		"--Collaroy",
		"Freshwater--",
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ensure no_double_dash fails on '%s'", tc), func(t *testing.T) {
			expected.Check(t, New().Validate(&Ferry{tc}))
		})
	}
}

func TestDoubleDashValid(t *testing.T) {
	t.Parallel()
	type NintendoGame struct {
		Name string `validate:"no_double_dash"`
	}

	testCases := []string{
		"Mario Kart: Double Dash",
		"- -",
		"-=-=-=-=-=-=-=-",
		"——", // Em dash (unicode 2013)
		"––", // En dash (unicode 2014)
		"−−", // Minus (unicode 2212)
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ensure no_double_dash succeeds on '%s'", tc), func(t *testing.T) {
			err := New().Validate(&NintendoGame{tc})
			require.NoError(t, err)
		})
	}
}

func TestHTTPSAddressValid(t *testing.T) {
	t.Parallel()
	testCases := []string{
		"http://atlassian.com/",
		"https://atlassian.com/",
		"https://atlassian.com",
		"https://www.atlassian.com",
		"https://www.atlassian.com/aboutus",
		"https://www.atlassian.com/about/contact",
		"https://www.atlassian.com/about/contact/",
		"https://atlassian.com/.au",

		// Provided by formvalidation.io
		"https://foo.com/blah_blah",
		"https://foo.com/blah_blah/",
		"https://foo.com/blah_blah_(wikipedia)",
		"https://foo.com/blah_blah_(wikipedia)_(again)",
		"https://www.example.com/wpstyle/?p=364",
		"https://www.example.com/foo/?bar=baz&inga=42&quux",
		"https://userid:password@example.com:8080",
		"https://userid:password@example.com:8080/",
		"https://userid@example.com",
		"https://userid@example.com/",
		"https://userid@example.com:8080",
		"https://userid@example.com:8080/",
		"https://userid:password@example.com",
		"https://userid:password@example.com/",
		"https://foo.com/blah_(wikipedia)#cite-1",
		"https://foo.com/blah_(wikipedia)_blah#cite-1",
		"https://foo.com/unicode_(✪)_in_parens",
		"https://foo.com/(something)?after=parens",
		"https://code.google.com/events/#&product=browser",
		"https://j.mp",
		"https://foo.bar/?q=Test%20URL-encoded%20stuff",
		"https://-.~_!$&'()*+,;=:%40:80%2f::::::@example.com",
		"https://1337.net",
		"https://a.b-c.de",
		"https://a.b--c.de/",
		"https://a.ba--c.de/",
		"https://a.xn--c.de/",
		"https://223.255.255.254",
		"http://www.atlassian.com./",
	}

	type WubbaLubbaDubDub struct {
		Endpoint string `validate:"aws_sns_topic_endpoint_url"`
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ensure aws_sns_topic_endpoint_url succeeds on '%s'", tc), func(t *testing.T) {
			err := New().Validate(&WubbaLubbaDubDub{Endpoint: tc})
			require.NoError(t, err)
		})
	}
}

func TestHTTPSAddressNotValid(t *testing.T) {
	t.Parallel()
	testCases := []string{
		"atlassian.com",
		"ftp://atlassian.com",
		"https://https://atlassian.com/",
		"https://https://https://atlassian.com/",
		"http://",
		"http://.",
		"http://..",
		"http://../",
		"http://?",
		"http://??",
		"http://??/",
		"http://#",
		"http://##",
		"http://##/",
		"http://atlassian.com?q=s p a c e s",
		"http://.atlassian.com/",
		"//",
		"//a",
		"///a",
		"///",
		"http:///a",
		"https:///a",
		"atlassian.com",
		"httq://atlassian",
		"http:// atlassian.com",
		":// atlassian com",
		"http://atlassian.com/stride(jira)micros voyager",
		"ftps://atlassian.com/",
		"http://-atlassian-.com/",
		"http://jira.atlassian-.com",
		"http://-stride.atlassian.com",

		// Unicode is supported in domain names, but we shouldn't have them... and luckily this library doesn't support
		// them.
		"https://مثال.إختبار.atlassian.com",
		"https://例子.测试.atlassian.com",
		"https://उदाहरण.परीक्षा.atlassian.com",
		"https://➡.atlassian.com/䨹",
		"https://⌘.atlassian.com",
		"https://⌘.atlassian.com/",
		"https://✪.atlassian.com/aboutus",
	}

	type LowOrbitIonCannon struct {
		Target string `validate:"aws_sns_topic_endpoint_url"`
	}

	expected := fieldErrorMetaArray{
		{
			Tag:             "aws_sns_topic_endpoint_url",
			StructNamespace: "LowOrbitIonCannon.Target",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("enusre aws_sns_topic_endpoint_url fails on '%s'", tc), func(t *testing.T) {
			expected.Check(t, New().Validate(&LowOrbitIonCannon{Target: tc}))
		})
	}
}
