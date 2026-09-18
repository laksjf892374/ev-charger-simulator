package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"cposim/app"
)

const (
	defaultPort = "8080"
	// Until told otherwise, pushes go to the mock eMSP bundled in this same process.
	mockEMSPPath = "/emsp/ocpi/2.2.1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	emspBaseURL := os.Getenv("EMSP_BASE_URL")
	if emspBaseURL == "" {
		emspBaseURL = "http://127.0.0.1:" + port + mockEMSPPath
	}

	simulator, err := app.NewSimulator(app.Config{
		EMSPBaseURL: emspBaseURL,
		Out:         os.Stdout,
	})
	if err != nil {
		return fmt.Errorf("app.NewSimulator: %w", err)
	}

	// Listen before seeding: seeding pushes to the eMSP, which by default is this very process.
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("net.Listen: %w", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- http.Serve(listener, simulator.Handler())
	}()

	if err := simulator.Start(); err != nil {
		return fmt.Errorf("simulator.Start: %w", err)
	}

	if err := simulator.Seed(); err != nil {
		return fmt.Errorf("simulator.Seed: %w", err)
	}

	fmt.Fprintf(os.Stdout, "CPO simulator listening on :%s, pushing to %s\n", port, emspBaseURL)

	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		return fmt.Errorf("http.Serve: %w", err)
	case <-interrupted:
	}

	if err := simulator.Stop(); err != nil {
		return fmt.Errorf("simulator.Stop: %w", err)
	}

	return nil
}
