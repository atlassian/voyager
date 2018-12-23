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

package binding

import (
	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/command"
	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/output"
	"github.com/spf13/cobra"
)

type getCmd struct {
	*command.Namespaced
	*command.Formatted
	name string
}

// NewGetCmd builds a "svcat get bindings" command
func NewGetCmd(cxt *command.Context) *cobra.Command {
	getCmd := &getCmd{
		Namespaced: command.NewNamespaced(cxt),
		Formatted:  command.NewFormatted(),
	}
	cmd := &cobra.Command{
		Use:     "bindings [NAME]",
		Aliases: []string{"binding", "bnd"},
		Short:   "List bindings, optionally filtered by name or namespace",
		Example: command.NormalizeExamples(`
  svcat get bindings
  svcat get bindings --all-namespaces
  svcat get binding wordpress-mysql-binding
  svcat get binding -n ci concourse-postgres-binding
`),
		PreRunE: command.PreRunE(getCmd),
		RunE:    command.RunE(getCmd),
	}

	getCmd.AddNamespaceFlags(cmd.Flags(), true)
	getCmd.AddOutputFlags(cmd.Flags())
	return cmd
}

func (c *getCmd) Validate(args []string) error {
	if len(args) > 0 {
		c.name = args[0]
	}

	return nil
}

func (c *getCmd) Run() error {
	if c.name == "" {
		return c.getAll()
	}

	return c.get()
}

func (c *getCmd) getAll() error {
	bindings, err := c.App.RetrieveBindings(c.Namespace)
	if err != nil {
		return err
	}

	output.WriteBindingList(c.Output, c.OutputFormat, bindings)
	return nil
}

func (c *getCmd) get() error {
	binding, err := c.App.RetrieveBinding(c.Namespace, c.name)
	if err != nil {
		return err
	}

	output.WriteBinding(c.Output, c.OutputFormat, *binding)
	return nil
}
