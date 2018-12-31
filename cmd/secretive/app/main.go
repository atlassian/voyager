package app

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/voyager/cmd"
)

func Main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	homeDir := func() string {
		if h := os.Getenv("HOME"); h != "" {
			return h
		}
		return os.Getenv("USERPROFILE") // windows
	}

	a := App{}
	logOpts := options.LoggerOptions{}

	if home := homeDir(); home != "" {
		flag.StringVar(&a.Kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&a.Kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.BoolVar(&a.Local, "local", false, "run outside the kubernetes cluster")
	flag.StringVar(&a.ConfigFile, "config", "config.yaml", "config file")
	options.BindLoggerFlags(&logOpts, flag.CommandLine)
	flag.Parse()

	a.Logger = options.LoggerFromOptions(logOpts)

	return a.Run(ctx)
}
