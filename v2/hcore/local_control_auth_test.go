package hcore

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"net"
	"testing"
	"time"
)

type privacyRPC interface{}

func TestLocalControlRejectsUnauthorizedUnaryAndStreaming(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(localControlServerOptions("private-test-key")...)
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "privacy.Test", HandlerType: (*privacyRPC)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Unary", Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
			request := new(emptypb.Empty)
			if err := dec(request); err != nil {
				return nil, err
			}
			handler := func(context.Context, interface{}) (interface{}, error) { return &emptypb.Empty{}, nil }
			return interceptor(ctx, request, &grpc.UnaryServerInfo{FullMethod: "/privacy.Test/Unary"}, handler)
		}}},
		Streams: []grpc.StreamDesc{{StreamName: "Stream", ServerStreams: true, Handler: func(_ interface{}, stream grpc.ServerStream) error {
			request := new(emptypb.Empty)
			if err := stream.RecvMsg(request); err != nil {
				return err
			}
			return stream.SendMsg(&emptypb.Empty{})
		}}},
	}, struct{}{})
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///privacy", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, test := range []struct {
		name   string
		values []string
		want   codes.Code
	}{
		{"missing", nil, codes.Unauthenticated}, {"wrong", []string{"Bearer wrong"}, codes.Unauthenticated},
		{"duplicate", []string{"Bearer private-test-key", "Bearer wrong"}, codes.Unauthenticated},
		{"authorized", []string{"Bearer private-test-key"}, codes.OK},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if test.values != nil {
				ctx = metadata.NewOutgoingContext(ctx, metadata.MD{"authorization": test.values})
			}
			err = conn.Invoke(ctx, "/privacy.Test/Unary", &emptypb.Empty{}, &emptypb.Empty{})
			if status.Code(err) != test.want {
				t.Fatalf("unary: %v", err)
			}
			stream, e := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/privacy.Test/Stream")
			if e != nil {
				t.Fatal(e)
			}
			if e = stream.SendMsg(&emptypb.Empty{}); e != nil {
				t.Fatal(e)
			}
			stream.CloseSend()
			e = stream.RecvMsg(&emptypb.Empty{})
			if status.Code(e) != test.want {
				t.Fatalf("stream: %v", e)
			}
		})
	}
}

func TestLocalControlRecoversHandlerPanic(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(localControlServerOptions("")...)
	server.RegisterService(&grpc.ServiceDesc{ServiceName: "panic.Test", HandlerType: (*privacyRPC)(nil), Methods: []grpc.MethodDesc{{MethodName: "Unary", Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
		request := new(emptypb.Empty)
		if err := dec(request); err != nil {
			return nil, err
		}
		handler := func(context.Context, interface{}) (interface{}, error) { panic("test handler") }
		return interceptor(ctx, request, &grpc.UnaryServerInfo{FullMethod: "/panic.Test/Unary"}, handler)
	}}}, Streams: []grpc.StreamDesc{{StreamName: "Stream", ServerStreams: true, Handler: func(interface{}, grpc.ServerStream) error { panic("test stream") }}}}, struct{}{})
	go server.Serve(listener)
	defer server.Stop()
	connection, err := grpc.NewClient("passthrough:///panic", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for range 2 {
		if err := connection.Invoke(ctx, "/panic.Test/Unary", &emptypb.Empty{}, &emptypb.Empty{}); status.Code(err) != codes.Internal {
			t.Fatalf("panic not returned: %v", err)
		}
	}
	stream, err := connection.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/panic.Test/Stream")
	if err != nil {
		t.Fatal(err)
	}
	stream.SendMsg(&emptypb.Empty{})
	stream.CloseSend()
	if err := stream.RecvMsg(&emptypb.Empty{}); status.Code(err) != codes.Internal {
		t.Fatalf("stream panic not returned: %v", err)
	}
}
