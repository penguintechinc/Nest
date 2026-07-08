package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

// ObjectRequest is the request message for ObjectService operations.
type ObjectRequest struct {
	Resource string
	Bucket   string
	Key      string
	Body     []byte
}

// ObjectResponse is the response message for ObjectService operations.
type ObjectResponse struct {
	Body        []byte
	ContentType string
	ETag        string
	Error       string
}

// ObjectServiceServer defines the gRPC service interface.
type ObjectServiceServer interface {
	Get(ctx context.Context, req *ObjectRequest) (*ObjectResponse, error)
	Put(ctx context.Context, req *ObjectRequest) (*ObjectResponse, error)
	Delete(ctx context.Context, req *ObjectRequest) (*ObjectResponse, error)
}

type objectServiceImpl struct {
	cfg    config.Config
	logger *zap.Logger
}

func NewObjectService(cfg config.Config, logger *zap.Logger) ObjectServiceServer {
	return &objectServiceImpl{cfg: cfg, logger: logger}
}

func (o *objectServiceImpl) Get(ctx context.Context, req *ObjectRequest) (*ObjectResponse, error) {
	cl, ok := claims.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims")
	}
	if req.Resource == "" || req.Bucket == "" || req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "resource, bucket, and key are required")
	}

	endpoint, err := resolveEndpoint(ctx, o.cfg, cl.Tenant, req.Resource)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "resolve endpoint: %v", err)
	}

	// Validate bucket and key to prevent path traversal
	if strings.Contains(req.Bucket, "..") || strings.Contains(req.Bucket, "/") {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket name: contains path traversal characters")
	}
	if strings.Contains(req.Key, "..") {
		return nil, status.Error(codes.InvalidArgument, "invalid key: contains path traversal characters")
	}

	// Escape bucket and key for URL safety
	escapedBucket := url.PathEscape(req.Bucket)
	escapedKey := url.PathEscape(req.Key)

	urlStr := fmt.Sprintf("%s/%s/%s", endpoint, escapedBucket, escapedKey)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "object get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read body: %v", err)
	}

	o.logger.Info("object.get",
		zap.String("tenant", cl.Tenant),
		zap.String("resource", req.Resource),
		zap.String("key", req.Key),
	)

	return &ObjectResponse{
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
	}, nil
}

func (o *objectServiceImpl) Put(ctx context.Context, req *ObjectRequest) (*ObjectResponse, error) {
	cl, ok := claims.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims")
	}
	if req.Resource == "" || req.Bucket == "" || req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "resource, bucket, and key are required")
	}

	endpoint, err := resolveEndpoint(ctx, o.cfg, cl.Tenant, req.Resource)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "resolve endpoint: %v", err)
	}

	// Validate bucket and key to prevent path traversal
	if strings.Contains(req.Bucket, "..") || strings.Contains(req.Bucket, "/") {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket name: contains path traversal characters")
	}
	if strings.Contains(req.Key, "..") {
		return nil, status.Error(codes.InvalidArgument, "invalid key: contains path traversal characters")
	}

	// Escape bucket and key for URL safety
	escapedBucket := url.PathEscape(req.Bucket)
	escapedKey := url.PathEscape(req.Key)

	urlStr := fmt.Sprintf("%s/%s/%s", endpoint, escapedBucket, escapedKey)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, urlStr, io.NopCloser(bytesReader(req.Body)))
	httpReq.ContentLength = int64(len(req.Body))
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "object put: %v", err)
	}
	defer resp.Body.Close()

	o.logger.Info("object.put",
		zap.String("tenant", cl.Tenant),
		zap.String("resource", req.Resource),
		zap.String("key", req.Key),
		zap.Int("bytes", len(req.Body)),
	)

	return &ObjectResponse{ETag: resp.Header.Get("ETag")}, nil
}

func (o *objectServiceImpl) Delete(ctx context.Context, req *ObjectRequest) (*ObjectResponse, error) {
	cl, ok := claims.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims")
	}
	if req.Resource == "" || req.Bucket == "" || req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "resource, bucket, and key are required")
	}

	endpoint, err := resolveEndpoint(ctx, o.cfg, cl.Tenant, req.Resource)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "resolve endpoint: %v", err)
	}

	// Validate bucket and key to prevent path traversal
	if strings.Contains(req.Bucket, "..") || strings.Contains(req.Bucket, "/") {
		return nil, status.Error(codes.InvalidArgument, "invalid bucket name: contains path traversal characters")
	}
	if strings.Contains(req.Key, "..") {
		return nil, status.Error(codes.InvalidArgument, "invalid key: contains path traversal characters")
	}

	// Escape bucket and key for URL safety
	escapedBucket := url.PathEscape(req.Bucket)
	escapedKey := url.PathEscape(req.Key)

	urlStr := fmt.Sprintf("%s/%s/%s", endpoint, escapedBucket, escapedKey)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete, urlStr, nil)
	if _, err := http.DefaultClient.Do(httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "object delete: %v", err)
	}

	return &ObjectResponse{}, nil
}

func bytesReader(b []byte) io.Reader {
	return &bytesReaderImpl{b: b, pos: 0}
}

type bytesReaderImpl struct {
	b   []byte
	pos int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}

var objectServiceDesc = grpc.ServiceDesc{
	ServiceName: "nest.v1.ObjectService",
	HandlerType: (*ObjectServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Get", Handler: objectGetHandler},
		{MethodName: "Put", Handler: objectPutHandler},
		{MethodName: "Delete", Handler: objectDeleteHandler},
	},
	Streams: []grpc.StreamDesc{},
}

func objectGetHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ObjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ObjectServiceServer).Get(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/nest.v1.ObjectService/Get"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ObjectServiceServer).Get(ctx, req.(*ObjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func objectPutHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ObjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ObjectServiceServer).Put(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/nest.v1.ObjectService/Put"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ObjectServiceServer).Put(ctx, req.(*ObjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func objectDeleteHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ObjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ObjectServiceServer).Delete(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/nest.v1.ObjectService/Delete"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ObjectServiceServer).Delete(ctx, req.(*ObjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func RegisterObjectService(s *grpc.Server, srv ObjectServiceServer) {
	s.RegisterService(&objectServiceDesc, srv)
}
