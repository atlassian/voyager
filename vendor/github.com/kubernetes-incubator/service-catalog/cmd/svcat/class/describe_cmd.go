/*
Copyright 2018 The Kubernetes Authors.

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

package class

import (
	"fmt"

	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/command"
	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/output"
	servicecatalog "github.com/kubernetes-incubator/service-catalog/pkg/svcat/service-catalog"
	"github.com/spf13/cobra"
)

type describeCmd struct {
	*command.Context
	lookupByUUID bool
	uuid         string
	name         string
}

// NewDescribeCmd builds a "svcat describe class" command
func NewDescribeCmd(cxt *command.Context) *cobra.Command {
	describeCmd := &describeCmd{Context: cxt}
	cmd := &cobra.Command{
		Use:     "class NAME",
		Aliases: []string{"classes", "cl"},
		Short:   "Show details of a specific class",
		Example: command.NormalizeExamples(`
  svcat describe class mysqldb
  svcat describe class -uuid 997b8372-8dac-40ac-ae65-758b4a5075a5
`),
		PreRunE: command.PreRunE(describeCmd),
		RunE:    command.RunE(describeCmd),
	}
	cmd.Flags().BoolVarP(
		&describeCmd.lookupByUUID,
		"uuid",
		"u",
		false,
		"Whether or not to get the class by UUID (the default is by name)",
	)
	return cmd
}

func (c *describeCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("a class name or uuid is required")
	}

	if c.lookupByUUID {
		c.uuid = args[0]
	} else {
		c.name = args[0]
	}

	return nil
}

func (c *describeCmd) Run() error {
	return c.describe()
}

func (c *describeCmd) describe() error {
	var class servicecatalog.Class
	var err error
	if c.lookupByUUID {
		class, err = c.App.RetrieveClassByID(c.uuid)
	} else {
		class, err = c.App.RetrieveClassByName(c.name, servicecatalog.ScopeOptions{
			Scope: servicecatalog.ClusterScope,
		})
	}
	if err != nil {
		return err
	}

	output.WriteClassDetails(c.Output, class)

	opts := servicecatalog.ScopeOptions{Scope: servicecatalog.AllScope}
	plans, err := c.App.RetrievePlans(class.GetName(), opts)
	if err != nil {
		return err
	}
	output.WriteAssociatedPlans(c.Output, plans)

	return nil
}
