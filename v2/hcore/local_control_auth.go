package hcore

import (
	"context"
	"crypto/subtle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func localControlServerOptions(secret string) []grpc.ServerOption {
	serverOptions := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response interface{}, err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					Log(LogLevel_ERROR, LogType_CORE, "RPC panic: ", recovered)
					err = status.Error(codes.Internal, "core request failed")
				}
			}()
			return handler(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					Log(LogLevel_ERROR, LogType_CORE, "stream panic: ", recovered)
					err = status.Error(codes.Internal, "core stream failed")
				}
			}()
			return handler(srv, stream)
		}),
	}
	if secret != "" {
		check := func(ctx context.Context) error {
			md, _ := metadata.FromIncomingContext(ctx)
			values := md.Get("authorization")
			if len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte("Bearer "+secret)) != 1 {
				return status.Error(codes.Unauthenticated, "local control authorization required")
			}
			return nil
		}
		serverOptions = append(serverOptions,
			grpc.ChainUnaryInterceptor(func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
				if err := check(ctx); err != nil {
					return nil, err
				}
				return handler(ctx, req)
			}),
			grpc.ChainStreamInterceptor(func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				if err := check(stream.Context()); err != nil {
					return err
				}
				return handler(srv, stream)
			}),
		)
	}
	return serverOptions
}
