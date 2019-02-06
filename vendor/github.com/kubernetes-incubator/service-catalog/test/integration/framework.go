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

package integration

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"testing"
	"time"

	restfullog "github.com/emicklei/go-restful/log"
	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/util/wait"

	restclient "k8s.io/client-go/rest"

	genericserveroptions "k8s.io/apiserver/pkg/server/options"

	"github.com/kubernetes-incubator/service-catalog/cmd/apiserver/app/server"
	_ "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/install"
	_ "github.com/kubernetes-incubator/service-catalog/pkg/apis/settings/install"
	servicecatalogclient "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	"k8s.io/apimachinery/pkg/runtime"
)

func init() {
	rand.Seed(time.Now().UnixNano())
	// silence the go-restful webservices swagger logger
	restfullog.SetLogger(log.New(ioutil.Discard, "[restful]", log.LstdFlags|log.Lshortfile))
}

type TestServerConfig struct {
	etcdServerList []string
	emptyObjFunc   func() runtime.Object
}

// NewTestServerConfig is a default constructor for the standard test-apiserver setup
func NewTestServerConfig() *TestServerConfig {
	return &TestServerConfig{
		etcdServerList: []string{"http://localhost:2379"},
	}
}

func withConfigGetFreshApiserverAndClient(
	t *testing.T,
	serverConfig *TestServerConfig,
) (servicecatalogclient.Interface,
	*restclient.Config,
	func(),
) {
	stopCh := make(chan struct{})
	serverFailed := make(chan struct{})

	certDir, _ := ioutil.TempDir("", "service-catalog-integration")
	secureServingOptions := genericserveroptions.NewSecureServingOptions()

	var etcdOptions *server.EtcdOptions
	etcdOptions = server.NewEtcdOptions()
	etcdOptions.StorageConfig.ServerList = serverConfig.etcdServerList
	etcdOptions.EtcdOptions.StorageConfig.Prefix = fmt.Sprintf("%s-%08X", server.DefaultEtcdPathPrefix, rand.Int31())
	options := &server.ServiceCatalogServerOptions{
		GenericServerRunOptions: genericserveroptions.NewServerRunOptions(),
		AdmissionOptions:        genericserveroptions.NewAdmissionOptions(),
		SecureServingOptions:    secureServingOptions.WithLoopback(),
		EtcdOptions:             etcdOptions,
		AuthenticationOptions:   genericserveroptions.NewDelegatingAuthenticationOptions(),
		AuthorizationOptions:    genericserveroptions.NewDelegatingAuthorizationOptions(),
		AuditOptions:            genericserveroptions.NewAuditOptions(),
		DisableAuth:             true,
		StandaloneMode:          true, // this must be true because we have no kube server for integration.
		ServeOpenAPISpec:        true,
	}

	// get a random free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Errorf("failed to listen on 127.0.0.1:0")
	}

	// Set the listener here, usually the Listener is created later in the framework for us
	options.SecureServingOptions.Listener = ln
	options.SecureServingOptions.BindPort = ln.Addr().(*net.TCPAddr).Port

	t.Logf("Server started on port %v", options.SecureServingOptions.BindPort)

	secureAddr := fmt.Sprintf("https://localhost:%d", options.SecureServingOptions.BindPort)

	shutdownServer := func() {
		t.Logf("Shutting down server on port: %d", options.SecureServingOptions.BindPort)
		close(stopCh)
	}

	// start the server in the background
	go func() {
		options.SecureServingOptions.ServerCert.CertDirectory = certDir
		if err := server.RunServer(options, stopCh); err != nil {
			close(serverFailed)
			t.Logf("Error in bringing up the server: %v", err)
		}
	}()

	if err := waitForApiserverUp(secureAddr, serverFailed); err != nil {
		t.Fatalf("%v", err)
	}

	config := &restclient.Config{QPS: 50, Burst: 100}
	config.Host = secureAddr
	config.Insecure = true
	config.CertFile = secureServingOptions.ServerCert.CertKey.CertFile
	config.KeyFile = secureServingOptions.ServerCert.CertKey.KeyFile
	clientset, err := servicecatalogclient.NewForConfig(config)
	if nil != err {
		t.Fatal("can't make the client from the config", err)
	}
	t.Logf("Test client will use API Server URL of %v", secureAddr)
	return clientset, config, shutdownServer
}

func getFreshApiserverAndClient(
	t *testing.T,
	newEmptyObj func() runtime.Object,
) (servicecatalogclient.Interface, *restclient.Config, func()) {
	serverConfig := &TestServerConfig{
		etcdServerList: []string{"http://localhost:2379"},
		emptyObjFunc:   newEmptyObj,
	}
	client, clientConfig, shutdownFunc := withConfigGetFreshApiserverAndClient(t, serverConfig)
	return client, clientConfig, shutdownFunc
}

func waitForApiserverUp(serverURL string, stopCh <-chan struct{}) error {
	interval := 1 * time.Second
	timeout := 30 * time.Second
	startWaiting := time.Now()
	tries := 0
	return wait.PollImmediate(interval, timeout,
		func() (bool, error) {
			select {
			// we've been told to stop, so no reason to keep going
			case <-stopCh:
				return true, fmt.Errorf("apiserver failed")
			default:
				klog.Infof("Waiting for : %#v", serverURL)
				tr := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				c := &http.Client{Transport: tr}
				_, err := c.Get(serverURL)
				if err == nil {
					klog.Infof("Found server after %v tries and duration %v",
						tries, time.Since(startWaiting))
					return true, nil
				}
				tries++
				return false, nil
			}
		},
	)
}
