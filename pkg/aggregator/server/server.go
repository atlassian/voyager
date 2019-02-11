package server

import (
	"github.com/atlassian/voyager/pkg/aggregator/server/options"
	"github.com/spf13/cobra"
)

func NewServerCommand(o *options.AggregatorServerOptions, stopCh <-chan struct{}) *cobra.Command {
	cmd := &cobra.Command{
		Short: "Launch a aggregator API server",
		Long:  "Launch a aggregator API server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := Run(o, stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	fs := cmd.Flags()

	o.AddFlags(fs)
	return cmd
}

func Run(o *options.AggregatorServerOptions, stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New()
	if err != nil {
		return err
	}
	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}
