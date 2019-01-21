// Code generated by go-swagger; DO NOT EDIT.

package resolve

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"

	models "github.com/atlassian/voyager/pkg/releases/deployinator/models"
)

// ResolveReader is a Reader for the Resolve structure.
type ResolveReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *ResolveReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewResolveOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewResolveBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewResolveNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 500:
		result := NewResolveInternalServerError()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewResolveOK creates a ResolveOK with default headers values
func NewResolveOK() *ResolveOK {
	return &ResolveOK{}
}

/*ResolveOK handles this case with default header values.

A map of resolved release groups found for the given service and location set.
*/
type ResolveOK struct {
	Payload *models.ResolutionResponseType
}

func (o *ResolveOK) Error() string {
	return fmt.Sprintf("[GET /v1/resolve][%d] resolveOK  %+v", 200, o.Payload)
}

func (o *ResolveOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ResolutionResponseType)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewResolveBadRequest creates a ResolveBadRequest with default headers values
func NewResolveBadRequest() *ResolveBadRequest {
	return &ResolveBadRequest{}
}

/*ResolveBadRequest handles this case with default header values.

Bad request
*/
type ResolveBadRequest struct {
	Payload *models.ErrorResponse
}

func (o *ResolveBadRequest) Error() string {
	return fmt.Sprintf("[GET /v1/resolve][%d] resolveBadRequest  %+v", 400, o.Payload)
}

func (o *ResolveBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewResolveNotFound creates a ResolveNotFound with default headers values
func NewResolveNotFound() *ResolveNotFound {
	return &ResolveNotFound{}
}

/*ResolveNotFound handles this case with default header values.

Cannot resolve the mappings for the given service and location set.
*/
type ResolveNotFound struct {
}

func (o *ResolveNotFound) Error() string {
	return fmt.Sprintf("[GET /v1/resolve][%d] resolveNotFound ", 404)
}

func (o *ResolveNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewResolveInternalServerError creates a ResolveInternalServerError with default headers values
func NewResolveInternalServerError() *ResolveInternalServerError {
	return &ResolveInternalServerError{}
}

/*ResolveInternalServerError handles this case with default header values.

Unknown error has occurred
*/
type ResolveInternalServerError struct {
	Payload *models.ErrorResponse
}

func (o *ResolveInternalServerError) Error() string {
	return fmt.Sprintf("[GET /v1/resolve][%d] resolveInternalServerError  %+v", 500, o.Payload)
}

func (o *ResolveInternalServerError) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.ErrorResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
