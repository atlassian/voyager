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

package broker

import (
	"fmt"

	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/command"
	"github.com/kubernetes-incubator/service-catalog/pkg/svcat/service-catalog"
	"github.com/spf13/cobra"
)

type syncCmd struct {
	*command.Namespaced
	*command.Scoped
	name string
}

// NewSyncCmd builds a "svcat sync broker" command
func NewSyncCmd(cxt *command.Context) *cobra.Command {
	syncCmd := &syncCmd{
		Namespaced: command.NewNamespaced(cxt),
		Scoped:     command.NewScoped(),
	}
	rootCmd := &cobra.Command{
		Use:     "broker NAME",
		Short:   "Syncs service catalog for a service broker",
		Example: command.NormalizeExamples(`svcat sync broker asb`),
		PreRunE: command.PreRunE(syncCmd),
		RunE:    command.RunE(syncCmd),
	}
	syncCmd.AddScopedFlags(rootCmd.Flags(), false)
	syncCmd.AddNamespaceFlags(rootCmd.Flags(), false)
	return rootCmd
}

func (c *syncCmd) Validate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("a broker name is required")
	}
	c.name = args[0]
	return nil
}

func (c *syncCmd) Run() error {
	return c.sync()
}

func (c *syncCmd) sync() error {
	scopeOpts := servicecatalog.ScopeOptions{
		Scope:     c.Scope,
		Namespace: c.Namespace,
	}

	const retries = 3
	err := c.App.Sync(c.name, scopeOpts, retries)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.Output, "Synchronization requested for broker: %s\n", c.name)
	return nil
}
