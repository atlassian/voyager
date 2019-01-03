package main

import (
	"flag"
	"os"

	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/voyager/cmd"
	comp_crd "github.com/atlassian/voyager/pkg/composition/crd"
	"github.com/atlassian/voyager/pkg/formation"
	"github.com/atlassian/voyager/pkg/ops"
	"github.com/atlassian/voyager/pkg/orchestration"
	"github.com/pkg/errors"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

func main() {
	cmd.ExitOnError(innerMain())
}

func innerMain() error {
	outputFormat := flag.String("output-format", "yaml", "Print a CRD and exit (specify format: json or yaml)")
	resource := flag.String("resource", "state", "Select which CRD to print (state, route, sd, ld)")
	flag.Parse()

	var crd *apiext_v1b1.CustomResourceDefinition
	switch *resource {
	case "state":
		crd = orchestration.StateCrd()
	case "route":
		crd = ops.RouteCrd()
	case "sd":
		crd = comp_crd.ServiceDescriptorCrd()
	case "ld":
		crd = formation.LocationDescriptorCrd()
	default:
		return errors.Errorf("unsupported CRD %q", *resource)
	}

	return resources.PrintCleanedObject(os.Stdout, *outputFormat, crd)
}
