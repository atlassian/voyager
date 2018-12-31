package main

import (
	"context"
	"encoding/json"
	"flag"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/atlassian/ctrl/options"
	"github.com/atlassian/ctrl/process"
	"github.com/atlassian/voyager/cmd"
	"github.com/atlassian/voyager/pkg/util/logz"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultAddr            = ":8080"
	defaultShutdownTimeout = 3 * time.Second
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cmd.RunInterruptably(runWithContext)
}

func runWithContext(ctx context.Context) error {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	addr := fs.String("addr", defaultAddr, "Address to listen on")

	logOpts := options.LoggerOptions{}
	options.BindLoggerFlags(&logOpts, fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return errors.Wrap(err, "could not load environment config file")
	}

	logger := options.LoggerFromOptions(logOpts)
	defer logz.Sync(logger)

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(handler))

	logger.Error("Starting server", zap.String("address", *addr))
	server := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}
	return process.StartStopServer(ctx, server, defaultShutdownTimeout)
}

func handler(w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(os.Environ())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(err.Error())) // nolint
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data) // nolint
}
