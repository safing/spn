package ships

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type sharedServer struct {
	listener net.Listener
	server   *http.Server

	handlers     map[string]http.HandlerFunc
	handlersLock sync.RWMutex
}

// ServeHTTP forwards requests to registered handler or uses defaults.
func (shared *sharedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	shared.handlersLock.Lock()
	defer shared.handlersLock.Unlock()

	// Get and forward to registered handler.
	handler, ok := shared.handlers[r.URL.Path]
	if ok {
		handler(w, r)
		return
	}

	// If there is registered handler and path is "/", respond with info page.
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		ServeInfoPage(w, r)
		return
	}

	// Otherwise, respond with error.
	http.Error(w, "", http.StatusNotFound)
}

var (
	sharedHTTPServers     = make(map[uint16]*sharedServer)
	sharedHTTPServersLock sync.Mutex
)

func addHTTPHandler(port uint16, path string, handler http.HandlerFunc) (ln net.Listener, err error) {
	// Check params.
	if port == 0 {
		return nil, errors.New("cannot listen on port 0")
	}

	// Default to root path.
	if path == "" {
		path = "/"
	}

	sharedHTTPServersLock.Lock()
	defer sharedHTTPServersLock.Unlock()

	// Get http server of the port.
	shared, ok := sharedHTTPServers[port]
	if ok {
		// Set path to handler.
		shared.handlersLock.Lock()
		defer shared.handlersLock.Unlock()

		// Check if path is already registered.
		_, ok := shared.handlers[path]
		if ok {
			return nil, errors.New("path already registered")
		}

		// Else, register handler at path.
		shared.handlers[path] = handler
		return shared.listener, nil
	}

	// Shared server does not exist - create one.
	shared = &sharedServer{
		handlers: make(map[string]http.HandlerFunc),
	}

	// Add first handler.
	shared.handlers[path] = handler

	// Define new server.
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           shared,
		ReadTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      1 * time.Minute,
		IdleTimeout:       1 * time.Minute,
		MaxHeaderBytes:    4096,
		// ErrorLog:          &log.Logger{}, // FIXME
		BaseContext: func(net.Listener) context.Context { return module.Ctx },
	}
	shared.server = server

	// Start listener.
	shared.listener, err = net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Add shared http server to list.
	sharedHTTPServers[port] = shared

	// Start server in service worker.
	module.StartServiceWorker(
		fmt.Sprintf("shared http server listener on port %d", port), 0,
		func(ctx context.Context) error {
			err := shared.server.Serve(shared.listener)
			if !errors.Is(http.ErrServerClosed, err) {
				return err
			}
			return nil
		},
	)

	return shared.listener, nil
}

func removeHTTPHandler(port uint16, path string) error {
	// Check params.
	if port == 0 {
		return nil
	}

	// Default to root path.
	if path == "" {
		path = "/"
	}

	sharedHTTPServersLock.Lock()
	defer sharedHTTPServersLock.Unlock()

	// Get http server of the port.
	shared, ok := sharedHTTPServers[port]
	if !ok {
		return nil
	}

	// Set path to handler.
	shared.handlersLock.Lock()
	defer shared.handlersLock.Unlock()

	// Check if path is registered.
	_, ok = shared.handlers[path]
	if !ok {
		return nil
	}

	// Remove path from handler.
	delete(shared.handlers, path)

	// Shutdown shared HTTP server if no more handlers are registered.
	if len(shared.handlers) == 0 {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()
		return shared.server.Shutdown(ctx)
	}

	// Remove shared HTTP server from map.
	delete(sharedHTTPServers, port)

	return nil
}
