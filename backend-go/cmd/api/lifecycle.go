package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/httpserver"
)

type apiShutdownCoordinator struct {
	server      *http.Server
	timeout     time.Duration
	stopWorkers func()
	logger      *log.Logger
	once        sync.Once
	done        chan struct{}
	err         error
}

func newAPIShutdownCoordinator(server *http.Server, timeout time.Duration, stopWorkers func(), logger *log.Logger) *apiShutdownCoordinator {
	if stopWorkers == nil {
		stopWorkers = func() {}
	}
	if logger == nil {
		logger = log.Default()
	}
	return &apiShutdownCoordinator{
		server: server, timeout: timeout, stopWorkers: stopWorkers, logger: logger, done: make(chan struct{}),
	}
}

func (coordinator *apiShutdownCoordinator) Shutdown(reason string) error {
	coordinator.once.Do(func() {
		defer close(coordinator.done)
		coordinator.err = coordinator.shutdown(reason)
	})
	<-coordinator.done
	return coordinator.err
}

func (coordinator *apiShutdownCoordinator) shutdown(reason string) error {
	if coordinator.server == nil || coordinator.timeout <= 0 {
		return fmt.Errorf("invalid API shutdown configuration")
	}
	coordinator.logger.Printf("API graceful shutdown started reason=%s timeout=%s", reason, coordinator.timeout)
	var failures []error
	if err := runShutdownStep("stop background workers", func() error {
		coordinator.stopWorkers()
		return nil
	}); err != nil {
		failures = append(failures, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), coordinator.timeout)
	defer cancel()
	if err := runShutdownStep("shutdown HTTP server", func() error {
		return coordinator.server.Shutdown(ctx)
	}); err != nil {
		failures = append(failures, err)
	}
	if err := runShutdownStep("wait HTTP shutdown hooks", func() error {
		return httpserver.WaitForShutdownHooks(ctx, coordinator.server)
	}); err != nil {
		failures = append(failures, err)
	}
	if ctx.Err() != nil {
		coordinator.logger.Printf("API graceful shutdown timeout reached; forcing listener close")
		if err := runShutdownStep("force close HTTP server", coordinator.server.Close); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) == 0 {
		coordinator.logger.Printf("API graceful shutdown completed")
		return nil
	}
	return errors.Join(failures...)
}

func runShutdownStep(name string, step func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%s panic recovered: %v", name, recovered)
		}
	}()
	if step == nil {
		return nil
	}
	if err := step(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func serveAPIUntilShutdown(server *http.Server, signals <-chan os.Signal, timeout time.Duration, stopWorkers func(), logger *log.Logger) (err error) {
	if server == nil || signals == nil {
		return fmt.Errorf("server and signal channel are required")
	}
	coordinator := newAPIShutdownCoordinator(server, timeout, stopWorkers, logger)
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = coordinator.Shutdown("panic")
			panic(recovered)
		}
	}()
	listenResult := make(chan error, 1)
	go func() {
		listenResult <- server.ListenAndServe()
	}()
	select {
	case signalValue := <-signals:
		reason := "signal"
		if signalValue != nil {
			reason = signalValue.String()
		}
		shutdownErr := coordinator.Shutdown(reason)
		listenErr := <-listenResult
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			return errors.Join(shutdownErr, listenErr)
		}
		return shutdownErr
	case listenErr := <-listenResult:
		shutdownErr := coordinator.Shutdown("listener_exit")
		if listenErr == nil || errors.Is(listenErr, http.ErrServerClosed) {
			return shutdownErr
		}
		return errors.Join(listenErr, shutdownErr)
	}
}
