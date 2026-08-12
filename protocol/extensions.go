package protocol

import (
	"github.com/valyala/fasthttp"
	"google.golang.org/grpc"
	"net/http"
)

type FastHTTPAware interface {
	ProtocolHandler
	SetFastHTTPOuterMiddleware(func(next fasthttp.RequestHandler) fasthttp.RequestHandler)
}

type FastHTTPHandlerAware interface {
	ProtocolHandler
	Handler() func(ctx *fasthttp.RequestCtx)
}

type HTTPHandlerAware interface {
	ProtocolHandler
	HTTPHandler() http.Handler
}

type GRPCUnaryInterceptable interface {
	ProtocolHandler
	SetUnaryInterceptorSource(func() []grpc.UnaryServerInterceptor)
}

type GRPCStreamInterceptable interface {
	ProtocolHandler
	SetStreamInterceptorSource(func() []grpc.StreamServerInterceptor)
}
