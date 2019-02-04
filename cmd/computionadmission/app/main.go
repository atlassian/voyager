package app

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/atlassian/voyager/cmd"
	"k8s.io/klog"
)

const (
	fakeServiceName = "voyager/computionadmission"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	klog.InitFlags(nil)
	cmd.RunInterruptably(func(ctx context.Context) error {
		server, err := NewServerFromFlags(fakeServiceName, flag.CommandLine, os.Args[1:])
		if err != nil {
			return err
		}

		return server.Run(ctx)
	})
}
