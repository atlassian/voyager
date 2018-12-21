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

package instance

import (
	"fmt"

	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/command"
	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/output"
	"github.com/kubernetes-incubator/service-catalog/cmd/svcat/parameters"
	"github.com/spf13/cobra"
)

type provisonCmd struct {
	*command.Namespaced
	*command.Waitable

	instanceName string
	externalID   string
	className    string
	planName     string
	rawParams    []string
	jsonParams   string
	params       interface{}
	rawSecrets   []string
	secrets      map[string]string
}

// NewProvisionCmd builds a "svcat provision" command
func NewProvisionCmd(cxt *command.Context) *cobra.Command {
	provisionCmd := &provisonCmd{
		Namespaced: command.NewNamespaced(cxt),
		Waitable:   command.NewWaitable(),
	}
	cmd := &cobra.Command{
		Use:   "provision NAME --plan PLAN --class CLASS",
		Short: "Create a new instance of a service",
		Example: command.NormalizeExamples(`
  svcat provision wordpress-mysql-instance --class mysqldb --plan free -p location=eastus -p sslEnforcement=disabled
  svcat provision wordpress-mysql-instance --external-id a7c00676-4398-11e8-842f-0ed5f89f718b --class mysqldb --plan free
  svcat provision wordpress-mysql-instance --class mysqldb --plan free -s mysecret[dbparams]
  svcat provision secure-instance --class mysqldb --plan secureDB --params-json '{
    "encrypt" : true,
    "firewallRules" : [
        {
            "name": "AllowSome",
            "startIPAddress": "75.70.113.50",
            "endIPAddress" : "75.70.113.131"
        }
    ]
  }'
`),
		PreRunE: command.PreRunE(provisionCmd),
		RunE:    command.RunE(provisionCmd),
	}
	provisionCmd.AddNamespaceFlags(cmd.Flags(), false)
	cmd.Flags().StringVar(&provisionCmd.externalID, "external-id", "",
		"The ID of the instance for use with the OSB SB API (Optional)")
	cmd.Flags().StringVar(&provisionCmd.className, "class", "",
		"The class name (Required)")
	cmd.MarkFlagRequired("class")
	cmd.Flags().StringVar(&provisionCmd.planName, "plan", "",
		"The plan name (Required)")
	cmd.MarkFlagRequired("plan")
	cmd.Flags().StringSliceVarP(&provisionCmd.rawParams, "param", "p", nil,
		"Additional parameter to use when provisioning the service, format: NAME=VALUE. Cannot be combined with --params-json, Sensitive information should be placed in a secret and specified with --secret")
	cmd.Flags().StringSliceVarP(&provisionCmd.rawSecrets, "secret", "s", nil,
		"Additional parameter, whose value is stored in a secret, to use when provisioning the service, format: SECRET[KEY]")
	cmd.Flags().StringVar(&provisionCmd.jsonParams, "params-json", "",
		"Additional parameters to use when provisioning the service, provided as a JSON object. Cannot be combined with --param")
	provisionCmd.AddWaitFlags(cmd)

	return cmd
}

func (c *provisonCmd) Validate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("an instance name is required")
	}
	c.instanceName = args[0]

	var err error

	if c.jsonParams != "" && len(c.rawParams) > 0 {
		return fmt.Errorf("--params-json cannot be used with --param")
	}

	if c.jsonParams != "" {
		c.params, err = parameters.ParseVariableJSON(c.jsonParams)
		if err != nil {
			return fmt.Errorf("invalid --params-json value (%s)", err)
		}
	} else {
		c.params, err = parameters.ParseVariableAssignments(c.rawParams)
		if err != nil {
			return fmt.Errorf("invalid --param value (%s)", err)
		}
	}

	c.secrets, err = parameters.ParseKeyMaps(c.rawSecrets)
	if err != nil {
		return fmt.Errorf("invalid --secret value (%s)", err)
	}

	return nil
}

func (c *provisonCmd) Run() error {
	return c.Provision()
}

func (c *provisonCmd) Provision() error {
	instance, err := c.App.Provision(c.Namespace, c.instanceName, c.externalID, c.className, c.planName, c.params, c.secrets)
	if err != nil {
		return err
	}

	if c.Wait {
		fmt.Fprintln(c.Output, "Waiting for the instance to be provisioned...")
		finalInstance, err := c.App.WaitForInstance(instance.Namespace, instance.Name, c.Interval, c.Timeout)
		if err == nil {
			instance = finalInstance
		}

		// Always print the instance because the provision did succeed,
		// and just print any errors that occurred while polling
		output.WriteInstanceDetails(c.Output, instance)
		return err
	}

	output.WriteInstanceDetails(c.Output, instance)
	return nil
}
