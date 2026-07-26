package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

type waitableShutdownHook struct {
	done chan struct{}
	once sync.Once
}

var waitableServerShutdownHooks sync.Map

func registerWaitableShutdownHook(server *http.Server, stop func()) {
	if server == nil || stop == nil {
		return
	}
	hook := &waitableShutdownHook{done: make(chan struct{})}
	waitableServerShutdownHooks.Store(server, hook)
	server.RegisterOnShutdown(func() {
		hook.once.Do(func() {
			defer close(hook.done)
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.Error("HTTP shutdown hook panic recovered", "component", "operation_center_scheduler", "error", fmt.Sprint(recovered))
				}
			}()
			stop()
		})
	})
}

func WaitForShutdownHooks(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}
	value, ok := waitableServerShutdownHooks.Load(server)
	if !ok {
		return nil
	}
	hook := value.(*waitableShutdownHook)
	select {
	case <-hook.done:
		waitableServerShutdownHooks.Delete(server)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
