package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"cposim/app"
	"cposim/mockemsp"
)

const defaultPort = "8080"

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

	// Until EMSP_BASE_URL points somewhere else, the eMSP is the mock bundled in this process.
	selfBaseURL := "http://127.0.0.1:" + port
	config := app.Config{
		EMSPBaseURL: os.Getenv("EMSP_BASE_URL"),
		Out:         os.Stdout,
	}
	if config.EMSPBaseURL == "" {
		config.EMSPBaseURL = selfBaseURL + mockemsp.ReceiverPath
		config.MockEMSPSelfBaseURL = selfBaseURL
	}

	simulator, err := app.NewSimulator(config)
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

	if err := simulator.ConnectMockEMSP(); err != nil {
		return fmt.Errorf("simulator.ConnectMockEMSP: %w", err)
	}

	fmt.Fprintf(os.Stdout, "CPO simulator listening on :%s, pushing to %s\n", port, config.EMSPBaseURL)

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
