package goichi

import (
	"context"
	"fmt"
	"github.com/goichi-dev/goichi/protocol"
)

func (a *App) GetProtocols() *protocol.ProtocolRegistry {
	return a.Registry
}

func (a *App) StartAllProtocols(ctx context.Context) error {
	for name, handler := range a.Registry.GetAll() {
		if a.Config.Server.EnablePortMultiplexing {
			// Skip protocols that are mounted as HTTP handlers on the main router
			// because they are handled by the main fasthttp server.
			_, isFastHTTP := handler.(protocol.FastHTTPHandlerAware)
			_, isHTTP := handler.(protocol.HTTPHandlerAware)
			if isFastHTTP || isHTTP {
				continue
			}
		}
		if err := handler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start protocol %s: %w", name, err)
		}
	}
	return nil
}

func (a *App) StartProtocols(ctx context.Context, names ...string) error {
	for _, name := range names {
		handler := a.Registry.Get(name)
		if handler == nil {
			return fmt.Errorf("protocol not found: %s", name)
		}
		if err := handler.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) StopAllProtocols(ctx context.Context) error {
	return a.Registry.StopAll(ctx)
}
