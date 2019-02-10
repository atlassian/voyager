package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	ctrlApp "github.com/atlassian/ctrl/app"
	"github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/composition"
	"github.com/atlassian/voyager/pkg/options"
	"github.com/atlassian/voyager/pkg/util/crash"
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	defer crash.LogPanicAsJSON()
	ctrlApp.CancelOnInterrupt(ctx, cancelFunc)

	err := runWithContext(ctx)

	if err != nil && errors.Cause(err) != context.Canceled {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runWithContext(context context.Context) error {
	sd, err := LoadSD("cmd/validator/testdata/sd.yaml")
	if err != nil {
		return err
	}

	// Composition
	clusterLocation, err := LoadLocation("cmd/validator/testdata/location.yaml")
	sdTransformer := composition.NewServiceDescriptorTransformer(clusterLocation.ClusterLocation())
	formationObjectResults, err := composition.ProcessSD(sd, sdTransformer)
	if err != nil {
		return err
	}

	fmt.Println(Print(json.MarshalIndent(composition.Locations(formationObjectResults), "", "  ")))

	return nil
}

func Print(b []byte, err error) string {
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func LoadLocation(filename string) (*options.Location, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	location := &options.Location{}
	err = yaml.UnmarshalStrict(bytes, location)
	if err != nil {
		return nil, err
	}
	return location, nil
}

func LoadSD(filename string) (*v1.ServiceDescriptor, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	sd := &v1.ServiceDescriptor{}
	err = yaml.UnmarshalStrict(bytes, sd)
	if err != nil {
		return nil, err
	}
	return sd, nil
}
