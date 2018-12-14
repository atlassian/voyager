package validation

import (
	"reflect"
	"strings"

	"github.com/asaskevich/govalidator"
	validator "gopkg.in/go-playground/validator.v9"
)

// Don't panic()
type Validator struct {
	validate *validator.Validate
}

func New() *Validator {
	validate := validator.New()
	err := validate.RegisterValidation("no_double_dash", assertNoDoubleDashInString)
	if err != nil {
		panic(err) // static inputs above, so can only be unrecoverable coder error
	}
	err = validate.RegisterValidation("aws_sns_topic_endpoint_url", assertAWSSNSTopicEndpointURL)
	if err != nil {
		panic(err) // static inputs above, so can only be unrecoverable coder error
	}

	return &Validator{
		validate: validate,
	}
}

func (v *Validator) Validate(object interface{}) error {
	return v.validate.Struct(object)
}

func HasDoubleDash(s string) bool {
	return strings.Contains(s, "--")
}

func assertNoDoubleDashInString(fl validator.FieldLevel) bool {
	st := fl.Field()
	if st.Kind() != reflect.String {
		return false
	}
	if HasDoubleDash(st.Interface().(string)) {
		return false
	}
	return true
}

// Validate that the endpoint is a valid Endpoint request parameter for an AWS::SNS::Topic, AND is specifically a URL
// in the form of http:// or https://
//  - https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-sns-subscription.html
//  - https://docs.aws.amazon.com/sns/latest/api/API_Subscribe.html
func assertAWSSNSTopicEndpointURL(fl validator.FieldLevel) bool {
	field := fl.Field()
	if field.Kind() != reflect.String {
		return false
	}
	s := field.Interface().(string)
	if !govalidator.IsURL(s) {
		return false
	}
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") {
		return false
	}
	return true
}
