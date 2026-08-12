package middleware

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func GRPCLogger() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		dur := time.Since(start)

		st, _ := status.FromError(err)
		code := st.Code()

		color := "\033[32m" // default green
		if err != nil {
			color = "\033[31m" // red for errors
		}

		log.Printf("[gRPC] %-30s %s%v%s  %s",
			info.FullMethod, color, code, "\033[0m", dur)

		return resp, err
	}
}

func GRPCStreamLogger() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		dur := time.Since(start)

		st, _ := status.FromError(err)
		code := st.Code()

		color := "\033[32m"
		if err != nil {
			color = "\033[31m"
		}

		log.Printf("[gRPC-Stream] %-30s %s%v%s  %s",
			info.FullMethod, color, code, "\033[0m", dur)

		return err
	}
}
