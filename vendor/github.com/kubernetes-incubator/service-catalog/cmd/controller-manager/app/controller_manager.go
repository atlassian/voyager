/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package app implements a server that runs the service catalog controllers.
package app

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	goruntime "runtime"
	"strconv"
	"time"

	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"

	"github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"github.com/kubernetes-incubator/service-catalog/pkg/kubernetes/pkg/util/configz"
	"github.com/kubernetes-incubator/service-catalog/pkg/metrics"
	"github.com/kubernetes-incubator/service-catalog/pkg/metrics/osbclientproxy"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	// The API groups for our API must be installed before we can use the
	// client to work with them.  This needs to be done once per process; this
	// is the point at which we handle this for the controller-manager
	// process.  Please do not remove.
	_ "github.com/kubernetes-incubator/service-catalog/pkg/api"

	"github.com/kubernetes-incubator/service-catalog/cmd/controller-manager/app/options"
	servicecatalogv1beta1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	settingsv1alpha1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/settings/v1alpha1"
	servicecataloginformers "github.com/kubernetes-incubator/service-catalog/pkg/client/informers_generated/externalversions"
	"github.com/kubernetes-incubator/service-catalog/pkg/controller"

	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog"
)

// NewControllerManagerCommand creates a *cobra.Command object with default
// parameters.
func NewControllerManagerCommand() *cobra.Command {
	s := options.NewControllerManagerServer()
	s.AddFlags(pflag.CommandLine)
	cmd := &cobra.Command{
		Use: "controller-manager",
		Long: `The service-catalog controller manager is a daemon that embeds
the core control loops shipped with the service catalog.`,
		Run: func(cmd *cobra.Command, args []string) {
		},
	}

	return cmd
}

const controllerManagerAgentName = "service-catalog-controller-manager"
const controllerDiscoveryAgentName = "service-catalog-controller-discovery"

var catalogGVR = schema.GroupVersionResource{Group: "servicecatalog.k8s.io", Version: "v1beta1", Resource: "clusterservicebrokers"}

// Run runs the service-catalog controller-manager; should never exit.
func Run(controllerManagerOptions *options.ControllerManagerServer) error {
	// TODO: what does this do

	// if c, err := configz.New("componentconfig"); err == nil {
	// 	c.Set(controllerManagerOptions.KubeControllerManagerConfiguration)
	// } else {
	// 	klog.Errorf("unable to register configz: %s", err)
	// }

	if controllerManagerOptions.Port > 0 {
		klog.Warning("program option --port is obsolete and ignored, specify --secure-port instead")
	}

	// Build the K8s kubeconfig / client / clientBuilder
	klog.V(4).Info("Building k8s kubeconfig")

	var err error
	var k8sKubeconfig *rest.Config
	if controllerManagerOptions.K8sAPIServerURL == "" && controllerManagerOptions.K8sKubeconfigPath == "" {
		k8sKubeconfig, err = rest.InClusterConfig()
	} else {
		k8sKubeconfig, err = clientcmd.BuildConfigFromFlags(
			controllerManagerOptions.K8sAPIServerURL,
			controllerManagerOptions.K8sKubeconfigPath)
	}
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client config: %v", err)
	}
	k8sKubeconfig.GroupVersion = &schema.GroupVersion{}

	k8sKubeconfig.ContentConfig.ContentType = controllerManagerOptions.ContentType
	// Override kubeconfig qps/burst settings from flags
	k8sKubeconfig.QPS = controllerManagerOptions.KubeAPIQPS
	k8sKubeconfig.Burst = int(controllerManagerOptions.KubeAPIBurst)
	k8sKubeClient, err := kubernetes.NewForConfig(
		rest.AddUserAgent(k8sKubeconfig, controllerManagerAgentName),
	)
	if err != nil {
		return fmt.Errorf("invalid Kubernetes API configuration: %v", err)
	}
	leaderElectionClient := kubernetes.NewForConfigOrDie(rest.AddUserAgent(k8sKubeconfig, "leader-election"))

	klog.V(4).Infof("Building service-catalog kubeconfig for url: %v\n", controllerManagerOptions.ServiceCatalogAPIServerURL)

	var serviceCatalogKubeconfig *rest.Config
	// Build the service-catalog kubeconfig / clientBuilder
	if controllerManagerOptions.ServiceCatalogAPIServerURL == "" && controllerManagerOptions.ServiceCatalogKubeconfigPath == "" {
		// explicitly fall back to InClusterConfig, assuming we're talking to an API server which does aggregation
		// (BuildConfigFromFlags does this, but gives a more generic warning message than we do here)
		klog.V(4).Infof("Using inClusterConfig to talk to service catalog API server -- make sure your API server is registered with the aggregator")
		serviceCatalogKubeconfig, err = rest.InClusterConfig()
	} else {
		serviceCatalogKubeconfig, err = clientcmd.BuildConfigFromFlags(
			controllerManagerOptions.ServiceCatalogAPIServerURL,
			controllerManagerOptions.ServiceCatalogKubeconfigPath)
	}
	if err != nil {
		// TODO: disambiguate API errors
		return fmt.Errorf("failed to get Service Catalog client configuration: %v", err)
	}
	serviceCatalogKubeconfig.Insecure = controllerManagerOptions.ServiceCatalogInsecureSkipVerify

	// Initialize SSL/TLS configuration.  Ensures we have a certificate and key to use.
	// This is the same code as what is done in the API Server.  By default, Helm created
	// cert and key for us, this just ensures the files are found and are readable and
	// creates self signed versions if not.
	if err := controllerManagerOptions.SecureServingOptions.MaybeDefaultWithSelfSignedCerts("" /*AdvertiseAddress*/, nil /*alternateDNS*/, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return fmt.Errorf("failed to establish SecureServingOptions %v", err)
	}

	klog.V(4).Info("Starting http server and mux")
	// Start http server and handlers
	go func() {
		mux := http.NewServeMux()
		apiAvailableChecker := checkAPIAvailableResources{
			controller.SimpleClientBuilder{
				ClientConfig: serviceCatalogKubeconfig,
			},
		}
		// liveness registered at /healthz indicates if the container is responding
		healthz.InstallHandler(mux, healthz.PingHealthz)

		// readiness registered at /healthz/ready indicates if traffic should be routed to this container
		healthz.InstallPathHandler(mux, "/healthz/ready", apiAvailableChecker)

		configz.InstallHandler(mux)
		metrics.RegisterMetricsAndInstallHandler(mux)

		if controllerManagerOptions.EnableProfiling {
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			if controllerManagerOptions.EnableContentionProfiling {
				goruntime.SetBlockProfileRate(1)
			}
		}
		server := &http.Server{
			Addr: net.JoinHostPort(controllerManagerOptions.SecureServingOptions.BindAddress.String(),
				strconv.Itoa(int(controllerManagerOptions.SecureServingOptions.BindPort))),
			Handler: mux,
		}
		klog.Fatal(server.ListenAndServeTLS(controllerManagerOptions.SecureServingOptions.ServerCert.CertKey.CertFile,
			controllerManagerOptions.SecureServingOptions.ServerCert.CertKey.KeyFile))
	}()

	// Create event broadcaster
	klog.V(4).Info("Creating event broadcaster")
	eventsScheme := runtime.NewScheme()
	// We use ConfigMapLock/EndpointsLock which emit events for ConfigMap/Endpoints and hence we need core/v1 types for it
	if err = corev1.AddToScheme(eventsScheme); err != nil {
		return err
	}
	// We also emit events for our own types
	if err = servicecatalogv1beta1.AddToScheme(eventsScheme); err != nil {
		return err
	}
	if err = settingsv1alpha1.AddToScheme(eventsScheme); err != nil {
		return err
	}

	eventBroadcaster := record.NewBroadcaster()
	loggingWatch := eventBroadcaster.StartLogging(klog.Infof)
	defer loggingWatch.Stop()
	recordingWatch := eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: k8sKubeClient.CoreV1().Events("")})
	defer recordingWatch.Stop()
	recorder := eventBroadcaster.NewRecorder(eventsScheme, v1.EventSource{Component: controllerManagerAgentName})

	// 'run' is the logic to run the controllers for the controller manager
	run := func(ctx context.Context) {
		serviceCatalogClientBuilder := controller.SimpleClientBuilder{
			ClientConfig: serviceCatalogKubeconfig,
		}

		// TODO: understand service account story for this controller-manager

		// if len(s.ServiceAccountKeyFile) > 0 && controllerManagerOptions.UseServiceAccountCredentials {
		// 	k8sClientBuilder = controller.SAControllerClientBuilder{
		// 		ClientConfig: restclient.AnonymousClientConfig(k8sKubeconfig),
		// 		CoreClient:   k8sKubeClient.Core(),
		// 		Namespace:    "kube-system",
		// 	}
		// } else {
		// 	k8sClientBuilder = rootClientBuilder
		// }

		err := StartControllers(controllerManagerOptions, k8sKubeconfig, serviceCatalogClientBuilder, recorder, ctx.Done())
		klog.Fatalf("error running controllers: %v", err)
		panic("unreachable")
	}

	if !controllerManagerOptions.LeaderElection.LeaderElect {
		run(context.TODO())
		panic("unreachable")
	}

	// Identity used to distinguish between multiple cloud controller manager instances
	id, err := os.Hostname()
	if err != nil {
		return err
	}

	klog.V(5).Infof("Using namespace %v for leader election lock", controllerManagerOptions.LeaderElectionNamespace)

	// Lock required for leader election
	rl, err := resourcelock.New(
		controllerManagerOptions.LeaderElection.ResourceLock,
		controllerManagerOptions.LeaderElectionNamespace,
		"service-catalog-controller-manager",
		leaderElectionClient.CoreV1(),
		resourcelock.ResourceLockConfig{
			Identity:      id + "-external-service-catalog-controller",
			EventRecorder: recorder,
		})
	if err != nil {
		return err
	}

	// Try and become the leader and start cloud controller manager loops
	leaderelection.RunOrDie(context.TODO(), leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: controllerManagerOptions.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: controllerManagerOptions.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   controllerManagerOptions.LeaderElection.RetryPeriod.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				klog.Fatalf("leaderelection lost")
			},
		},
	})
	panic("unreachable")
}

// getAvailableResources uses the discovery client to determine which API
// groups are available in the endpoint reachable from the given client and
// returns a map of them.
func getAvailableResources(clientBuilder controller.ClientBuilder, version schema.GroupVersion) (map[schema.GroupVersionResource]struct{}, error) {
	var apiResourceList *metav1.APIResourceList
	var clientError error

	// If apiserver is not running we should wait for some time and fail only then. This is particularly
	// important when we start apiserver and controller manager at the same time.
	err := wait.PollImmediate(time.Second, 60*time.Second, func() (bool, error) {
		var client clientset.Interface
		client, clientError = clientBuilder.Client(controllerDiscoveryAgentName)
		if clientError != nil {
			klog.Errorf("Failed to get api versions from server: %v", clientError)
			return false, nil
		}

		klog.V(4).Info("Created client for API discovery")

		discoveryClient := client.Discovery()
		apiResourceList, clientError = discoveryClient.ServerResourcesForGroupVersion(version.String())
		if clientError != nil {
			klog.Errorf("Failed to get supported resources from server: %v", clientError)
			return false, nil
		}

		return true, nil
	})

	// err will be nil or indicate timeout
	if err != nil {
		if clientError != nil {
			return nil, fmt.Errorf("failed to get api versions from server: %q, %q", err, clientError)
		}
		return nil, fmt.Errorf("failed to get api versions from server: %v", err)
	}

	allResources := map[schema.GroupVersionResource]struct{}{}
	for _, apiResource := range apiResourceList.APIResources {
		allResources[version.WithResource(apiResource.Name)] = struct{}{}
	}

	return allResources, nil
}

// StartControllers starts all the controllers in the service-catalog
// controller manager.
func StartControllers(s *options.ControllerManagerServer,
	coreKubeconfig *rest.Config,
	serviceCatalogClientBuilder controller.ClientBuilder,
	recorder record.EventRecorder,
	stop <-chan struct{}) error {

	// When Catalog Controller and Catalog API Server are started at the
	// same time with API Aggregation enabled, it may take some time before
	// Catalog registration shows up in API Server.  Attempt to get resources
	// every 10 seconds and quit after 3 minutes if unsuccessful.
	var availableResources map[schema.GroupVersionResource]struct{}
	err := wait.PollImmediate(10*time.Second, 3*time.Minute, func() (bool, error) {
		var err error
		availableResources, err = getAvailableResources(serviceCatalogClientBuilder, servicecatalogv1beta1.SchemeGroupVersion)
		if err != nil {
			return false, err
		}
		_, ok := availableResources[catalogGVR]
		return ok, nil
	},
	)

	if err != nil {
		if err == wait.ErrWaitTimeout {
			return fmt.Errorf("unable to start service-catalog controller: API GroupVersion %q is not available; found %#v", catalogGVR, availableResources)
		}
		return err
	}

	// Launch service-catalog controller
	coreKubeconfig = rest.AddUserAgent(coreKubeconfig, controllerManagerAgentName)
	coreClient, err := kubernetes.NewForConfig(coreKubeconfig)
	if err != nil {
		klog.Fatal(err)
	}
	klog.V(5).Infof("Creating shared informers; resync interval: %v", s.ResyncInterval)

	// Build the informer factory for service-catalog resources
	informerFactory := servicecataloginformers.NewSharedInformerFactory(
		serviceCatalogClientBuilder.ClientOrDie("shared-informers"),
		s.ResyncInterval,
	)
	// All shared informers are v1beta1 API level
	serviceCatalogSharedInformers := informerFactory.Servicecatalog().V1beta1()

	klog.V(5).Infof("Creating controller; broker relist interval: %v", s.ServiceBrokerRelistInterval)
	serviceCatalogController, err := controller.NewController(
		coreClient,
		serviceCatalogClientBuilder.ClientOrDie(controllerManagerAgentName).ServicecatalogV1beta1(),
		serviceCatalogSharedInformers.ClusterServiceBrokers(),
		serviceCatalogSharedInformers.ServiceBrokers(),
		serviceCatalogSharedInformers.ClusterServiceClasses(),
		serviceCatalogSharedInformers.ServiceClasses(),
		serviceCatalogSharedInformers.ServiceInstances(),
		serviceCatalogSharedInformers.ServiceBindings(),
		serviceCatalogSharedInformers.ClusterServicePlans(),
		serviceCatalogSharedInformers.ServicePlans(),
		osbclientproxy.NewClient,
		s.ServiceBrokerRelistInterval,
		s.OSBAPIPreferredVersion,
		recorder,
		s.ReconciliationRetryDuration,
		s.OperationPollingMaximumBackoffDuration,
		s.ClusterIDConfigMapName,
		s.ClusterIDConfigMapNamespace,
	)
	if err != nil {
		return err
	}

	klog.V(1).Info("Starting shared informers")
	informerFactory.Start(stop)

	klog.V(5).Info("Waiting for caches to sync")
	informerFactory.WaitForCacheSync(stop)

	klog.V(5).Info("Running controller")
	go serviceCatalogController.Run(s.ConcurrentSyncs, stop)

	select {}
}

// checkAPIAvailableResourcesServer is a HealthzChecker that makes sure the
// Service-Catalog APIServer is contactable.
type checkAPIAvailableResources struct {
	serviceCatalogClientBuilder controller.ClientBuilder
}

func (c checkAPIAvailableResources) Name() string {
	return "checkAPIAvailableResources"
}

func (c checkAPIAvailableResources) Check(_ *http.Request) error {
	klog.Info("Health-checking connection with service-catalog API server")
	availableResources, err := getAvailableResources(c.serviceCatalogClientBuilder, servicecatalogv1beta1.SchemeGroupVersion)
	if err != nil {
		return err
	}
	if _, ok := availableResources[catalogGVR]; !ok {
		return fmt.Errorf("failed to get API GroupVersion %q; found: %#v", catalogGVR, availableResources)
	}
	return nil
}
