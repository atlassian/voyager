package svccatadmission

import (
	"context"
	"net/http"
	"net/url"

	"github.com/atlassian/voyager/pkg/admission"
	"github.com/atlassian/voyager/pkg/execution/svccatadmission/rps"
	"github.com/atlassian/voyager/pkg/pkiutil"
	"github.com/atlassian/voyager/pkg/servicecentral"
	"github.com/atlassian/voyager/pkg/util"
	"github.com/atlassian/voyager/pkg/util/uuid"
	"github.com/go-chi/chi"
	"go.uber.org/zap"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
)

type SvcCatAdmission struct {
	Logger            *zap.Logger
	HTTPClient        *http.Client
	ServiceCentralURL *url.URL
	RPSURL            *url.URL
	ASAPClientConfig  pkiutil.ASAP
}

func (s *SvcCatAdmission) SetupAdmissionWebhooks(r *chi.Mux) {
	scHTTPClient := util.HTTPClient()
	scClient := servicecentral.NewServiceCentralClient(s.Logger, scHTTPClient, s.ASAPClientConfig, s.ServiceCentralURL)

	rpsHTTPClient := util.HTTPClient()
	rpsCache := rps.NewRPSCache(s.Logger, rps.NewRPSClient(s.Logger, rpsHTTPClient, s.ASAPClientConfig, s.RPSURL))

	r.Post("/externalid", admission.AdmitFuncHandlerFunc("externalid",
		func(ctx context.Context, logger *zap.Logger, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
			return ExternalUUIDAdmitFunc(ctx, uuid.Default(), scClient, rpsCache, admissionReview)
		}))
	r.Post("/micros", admission.AdmitFuncHandlerFunc("micros",
		func(ctx context.Context, logger *zap.Logger, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
			return MicrosAdmitFunc(ctx, scClient, admissionReview)
		}))
	r.Post("/asapkey", admission.AdmitFuncHandlerFunc("asapkey",
		func(ctx context.Context, logger *zap.Logger, admissionReview admissionv1beta1.AdmissionReview) (*admissionv1beta1.AdmissionResponse, error) {
			return AsapKeyAdmitFunc(ctx, admissionReview)
		}))
}
