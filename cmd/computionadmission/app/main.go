package app

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"time"

	"github.com/atlassian/voyager/cmd"
)

const (
	fakeServiceName = "voyager/computionadmission"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(func(ctx context.Context) error {
		server, err := NewServerFromFlags(fakeServiceName, flag.CommandLine, os.Args[1:])
		if err != nil {
			return err
		}

		return server.Run(ctx)
	})
}
