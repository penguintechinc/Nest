package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

// QueryRequest is the request message for QueryService.Execute.
type QueryRequest struct {
	// Resource is the DataResource name (e.g. "orders-db")
	Resource string
	// SQL is the query to execute (for SQL engines)
	SQL      string
	// Command is the engine-native command (for non-SQL engines)
	Command  string
	// Parameters are the named bind parameters
	Parameters map[string]string
}

// QueryResponse is the response message for QueryService.Execute.
type QueryResponse struct {
	// Rows contains the result rows as JSON arrays
	Rows    []string
	// Affected is the number of rows affected (for writes)
	Affected int64
	// Error is a non-empty string if the query failed
	Error   string
}

// QueryServiceServer defines the gRPC service interface.
type QueryServiceServer interface {
	Execute(ctx context.Context, req *QueryRequest) (*QueryResponse, error)
}

type queryServiceImpl struct {
	cfg    config.Config
	logger *zap.Logger
}

func NewQueryService(cfg config.Config, logger *zap.Logger) QueryServiceServer {
	return &queryServiceImpl{cfg: cfg, logger: logger}
}

func (q *queryServiceImpl) Execute(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	cl, ok := claims.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims")
	}
	if req.Resource == "" {
		return nil, status.Error(codes.InvalidArgument, "resource is required")
	}

	// Resolve backend endpoint from nest-api
	endpoint, err := resolveEndpoint(ctx, q.cfg, cl.Tenant, req.Resource)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "resolve endpoint: %v", err)
	}

	q.logger.Info("query.execute",
		zap.String("tenant", cl.Tenant),
		zap.String("resource", req.Resource),
		zap.String("endpoint", endpoint),
	)

	// For v1.0: return endpoint for client to use directly (full proxy in P7.5)
	return &QueryResponse{
		Rows: []string{fmt.Sprintf(`{"endpoint":%q,"message":"use native driver at this endpoint"}`, endpoint)},
	}, nil
}

// resolveEndpoint calls the Nest API to look up a DataResource's native endpoint.
func resolveEndpoint(ctx context.Context, cfg config.Config, tenant, resource string) (string, error) {
	cl, _ := claims.FromContext(ctx)

	url := fmt.Sprintf("%s/api/v1/tenants/%s/data-resources/%s", cfg.APIEndpoint, tenant, resource)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// Forward the original token (stripped from JWT claims sub as service identity in v1.0)
	httpReq.Header.Set("X-Tenant", cl.Tenant)
	httpReq.Header.Set("X-Subject", cl.Subject)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("resource %q not found for tenant %q", resource, tenant)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned %d", resp.StatusCode)
	}

	// Parse endpoint from response — simplified for v1.0
	return strings.Join([]string{cfg.APIEndpoint, "proxy", tenant, resource}, "/"), nil
}

// queryServiceDesc is the grpc.ServiceDesc for QueryService.
var queryServiceDesc = grpc.ServiceDesc{
	ServiceName: "nest.v1.QueryService",
	HandlerType: (*QueryServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Execute",
			Handler:    queryServiceExecuteHandler,
		},
	},
	Streams: []grpc.StreamDesc{},
}

func queryServiceExecuteHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(QueryServiceServer).Execute(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/nest.v1.QueryService/Execute"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(QueryServiceServer).Execute(ctx, req.(*QueryRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// RegisterQueryService registers the QueryService with a gRPC server.
func RegisterQueryService(s *grpc.Server, srv QueryServiceServer) {
	s.RegisterService(&queryServiceDesc, srv)
}
