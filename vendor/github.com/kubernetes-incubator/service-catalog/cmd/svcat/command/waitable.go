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

package command

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// HasWaitFlags represents a command that supports --wait.
type HasWaitFlags interface {
	// ApplyWaitFlags validates and persists the wait related flags.
	//   --wait
	//   --timeout
	//   --interval
	ApplyWaitFlags() error
}

// Waitable adds support to a command for the --wait flags.
type Waitable struct {
	Wait        bool
	rawTimeout  string
	Timeout     *time.Duration
	rawInterval string
	Interval    time.Duration
}

// NewWaitable initializes a new waitable command.
func NewWaitable() *Waitable {
	return &Waitable{}
}

// AddWaitFlags adds the wait related flags.
//   --wait
//   --timeout
//   --interval
func (c *Waitable) AddWaitFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&c.Wait, "wait", false,
		"Wait until the operation completes.")
	cmd.Flags().StringVar(&c.rawTimeout, "timeout", "5m",
		"Timeout for --wait, specified in human readable format: 30s, 1m, 1h. Specify -1 to wait indefinitely.")
	cmd.Flags().StringVar(&c.rawInterval, "interval", "1s",
		"Poll interval for --wait, specified in human readable format: 30s, 1m, 1h")
}

// ApplyWaitFlags validates and persists the wait related flags.
//   --wait
//   --timeout
//   --interval
func (c *Waitable) ApplyWaitFlags() error {
	if !c.Wait {
		return nil
	}

	if c.rawTimeout != "-1" {
		timeout, err := time.ParseDuration(c.rawTimeout)
		if err != nil {
			return fmt.Errorf("invalid --timeout value (%s)", err)
		}
		c.Timeout = &timeout
	}

	interval, err := time.ParseDuration(c.rawInterval)
	if err != nil {
		return fmt.Errorf("invalid --interval value (%s)", err)
	}
	c.Interval = interval

	return nil
}
