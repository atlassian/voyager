package main

import (
	"flag"
	"os"

	"github.com/atlassian/smith/pkg/resources"
	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/pkg/orchestration"
	"github.com/pkg/errors"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

func main() {
	cmd.ExitOnError(innerMain())
}

func innerMain() error {
	outputFormat := flag.String("output-format", "yaml", "Print a CRD and exit (specify format: json or yaml)")
	resource := flag.String("resource", "state", "Select which crd to print (type: state or route)")
	flag.Parse()

	var crd *apiext_v1b1.CustomResourceDefinition
	switch *resource {
	case "state":
		crd = orchestration.StateCrd()
	default:
		return errors.Errorf("unsupported CRD type %q", *resource)
	}

	return resources.PrintCleanedObject(os.Stdout, *outputFormat, crd)
}
