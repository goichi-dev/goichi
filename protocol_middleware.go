package goichi

import (
	"github.com/goichi-dev/goichi/core"
	"github.com/goichi-dev/goichi/protocol"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"google.golang.org/grpc"
	"log"
)

// UnifiedMiddleware defines a middleware that can be applied to both HTTP and gRPC protocols.
type UnifiedMiddleware interface {
	ToHTTP() ProtocolFastHTTPMiddleware
	ToGRPCUnary() grpc.UnaryServerInterceptor
	ToGRPCStream() grpc.StreamServerInterceptor
}

type ProtocolFastHTTPMiddleware func(next fasthttp.RequestHandler) fasthttp.RequestHandler

func (a *App) UseProtocol(m ...ProtocolFastHTTPMiddleware) {
	a.protocolMiddlewares = append(a.protocolMiddlewares, m...)
}

func (a *App) UseUnified(m ...UnifiedMiddleware) {
	for _, um := range m {
		a.UseProtocol(um.ToHTTP())
		a.UseGRPCUnary(um.ToGRPCUnary())
		a.UseGRPCStream(um.ToGRPCStream())
	}
}

func (a *App) UseGRPCUnary(i ...grpc.UnaryServerInterceptor) {
	a.grpcUnary = append(a.grpcUnary, i...)
}

func (a *App) UseGRPCStream(i ...grpc.StreamServerInterceptor) {
	a.grpcStream = append(a.grpcStream, i...)
}

func chainProtocolFastHTTP(mws []ProtocolFastHTTPMiddleware, next fasthttp.RequestHandler) fasthttp.RequestHandler {
	for i := len(mws) - 1; i >= 0; i-- {
		next = mws[i](next)
	}
	return next
}

func (a *App) wrapProtocolFastHTTP(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return chainProtocolFastHTTP(a.protocolMiddlewares, next)
}

func MiddlewareAsProtocol(m Middleware) ProtocolFastHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			h := func(c *Context) error {
				next(ctx)
				return nil
			}
			h = m(h)
			c := &Context{RequestCtx: ctx}
			if err := h(c); err != nil {
				log.Printf("[ERROR] %s %s: %v", ctx.Method(), ctx.Path(), err)
				ctx.SetStatusCode(StatusInternalServerError)
				_ = core.WriteJSONError(ctx, "internal server error")
			}
		}
	}
}

func (a *App) registerProtocolHooks(handler protocol.ProtocolHandler) {
	if a.Config.Server.EnablePortMultiplexing {
		// If it's an HTTP protocol, mount it on the main router
		paths := handler.GetPaths()
		if fa, ok := handler.(protocol.FastHTTPHandlerAware); ok {
			h := fa.Handler()
			if len(paths) > 0 {
				for _, p := range paths {
					a.Router.ANY(p, func(c *Context) error {
						h(c.RequestCtx)
						return nil
					}).HideDoc()
				}
			} else {
				a.Router.ANY("/*path", func(c *Context) error {
					h(c.RequestCtx)
					return nil
				}).HideDoc()
			}
		} else if ha, ok := handler.(protocol.HTTPHandlerAware); ok {
			h := fasthttpadaptor.NewFastHTTPHandler(ha.HTTPHandler())
			if len(paths) > 0 {
				for _, p := range paths {
					a.Router.ANY(p, func(c *Context) error {
						h(c.RequestCtx)
						return nil
					}).HideDoc()
				}
			} else {
				a.Router.ANY("/*path", func(c *Context) error {
					h(c.RequestCtx)
					return nil
				}).HideDoc()
			}
		}
	}

	if fa, ok := handler.(protocol.FastHTTPAware); ok {
		fa.SetFastHTTPOuterMiddleware(func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
			return a.wrapProtocolFastHTTP(next)
		})
	}
	if gp, ok := handler.(protocol.GRPCUnaryInterceptable); ok {
		gp.SetUnaryInterceptorSource(func() []grpc.UnaryServerInterceptor {
			out := make([]grpc.UnaryServerInterceptor, len(a.grpcUnary))
			copy(out, a.grpcUnary)
			return out
		})
	}
	if gp, ok := handler.(protocol.GRPCStreamInterceptable); ok {
		gp.SetStreamInterceptorSource(func() []grpc.StreamServerInterceptor {
			out := make([]grpc.StreamServerInterceptor, len(a.grpcStream))
			copy(out, a.grpcStream)
			return out
		})
	}
}
