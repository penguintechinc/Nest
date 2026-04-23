// Package grpc provides the gRPC server for the Nest API.
// Per Nest spec §17.1, gRPC is the source of truth; REST is a generated
// Connect/gRPC-Gateway surface on top. This stub registers health and
// reflection services so clients can discover the server before proto
// definitions are generated.
package grpc

import (
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Start launches the gRPC server on GRPC_PORT (default :50051) in a
// goroutine. It registers the standard health and reflection services.
// Any fatal startup error is logged and the goroutine exits — the HTTP
// server continues independently.
func Start() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	addr := ":" + port

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("gRPC: failed to listen on %s: %v", addr, err)
	}

	srv := grpc.NewServer()

	// Health service — reports SERVING for the empty service name ("") which
	// covers the whole server, and for the named NestService stub.
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("nest.v1.NestService", grpc_health_v1.HealthCheckResponse_SERVING)

	// Reflection service — lets grpcurl / Evans enumerate methods before
	// proto files are compiled into clients.
	reflection.Register(srv)

	log.Printf("gRPC server starting on %s", addr)
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()
}
