package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/penguintechinc/nest/services/db-proxy/proto/dbproxyv1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestConfigServiceGetConfig(t *testing.T) {
	logger := zap.NewNop()

	getConfigCalled := false
	getConfigFunc := func(ctx context.Context, key string) (interface{}, error) {
		getConfigCalled = true
		return map[string]interface{}{"test": "data"}, nil
	}

	service := NewConfigServiceServer(
		logger,
		getConfigFunc,
		func(ctx context.Context, key string, data []byte) error { return nil },
		func(ctx context.Context) error { return nil },
		func() map[string]interface{} { return make(map[string]interface{}) },
		func() int64 { return 0 },
		func() int64 { return 0 },
		func() int64 { return 0 },
	)

	req := &pb.GetConfigRequest{
		ApiVersion: "v1",
		ConfigKey:  "routes",
	}

	resp, err := service.GetConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}

	if !getConfigCalled {
		t.Error("getConfigFunc was not called")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.ConfigData, &data); err != nil {
		t.Errorf("failed to unmarshal config data: %v", err)
	}

	if data["test"] != "data" {
		t.Errorf("expected test=data, got %v", data)
	}
}

func TestConfigServiceGetConfigInvalidVersion(t *testing.T) {
	logger := zap.NewNop()

	service := NewConfigServiceServer(
		logger,
		func(ctx context.Context, key string) (interface{}, error) { return nil, nil },
		func(ctx context.Context, key string, data []byte) error { return nil },
		func(ctx context.Context) error { return nil },
		func() map[string]interface{} { return make(map[string]interface{}) },
		func() int64 { return 0 },
		func() int64 { return 0 },
		func() int64 { return 0 },
	)

	req := &pb.GetConfigRequest{
		ApiVersion: "v2",
		ConfigKey:  "routes",
	}

	_, err := service.GetConfig(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid api_version")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("error is not a gRPC status error")
	}

	if st.Code() != codes.Unimplemented {
		t.Errorf("expected code Unimplemented, got %v", st.Code())
	}
}

func TestConfigServiceSetConfig(t *testing.T) {
	logger := zap.NewNop()

	setConfigCalled := false
	setConfigFunc := func(ctx context.Context, key string, data []byte) error {
		setConfigCalled = true
		return nil
	}

	service := NewConfigServiceServer(
		logger,
		func(ctx context.Context, key string) (interface{}, error) { return nil, nil },
		setConfigFunc,
		func(ctx context.Context) error { return nil },
		func() map[string]interface{} { return make(map[string]interface{}) },
		func() int64 { return 0 },
		func() int64 { return 0 },
		func() int64 { return 0 },
	)

	configData, _ := json.Marshal(map[string]interface{}{"key": "value"})
	req := &pb.SetConfigRequest{
		ApiVersion: "v1",
		ConfigKey:  "routes",
		ConfigData: configData,
	}

	resp, err := service.SetConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}

	if !setConfigCalled {
		t.Error("setConfigFunc was not called")
	}
}

func TestConfigServiceReloadConfig(t *testing.T) {
	logger := zap.NewNop()

	reloadCalled := false
	reloadFunc := func(ctx context.Context) error {
		reloadCalled = true
		return nil
	}

	service := NewConfigServiceServer(
		logger,
		func(ctx context.Context, key string) (interface{}, error) { return nil, nil },
		func(ctx context.Context, key string, data []byte) error { return nil },
		reloadFunc,
		func() map[string]interface{} { return make(map[string]interface{}) },
		func() int64 { return 0 },
		func() int64 { return 0 },
		func() int64 { return 0 },
	)

	req := &pb.ReloadConfigRequest{
		ApiVersion: "v1",
	}

	resp, err := service.ReloadConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}

	if !reloadCalled {
		t.Error("reloadFunc was not called")
	}

	if resp.ReloadTimestamp == "" {
		t.Error("reload_timestamp not set")
	}
}

func TestConfigServiceGetStatus(t *testing.T) {
	logger := zap.NewNop()

	service := NewConfigServiceServer(
		logger,
		func(ctx context.Context, key string) (interface{}, error) { return nil, nil },
		func(ctx context.Context, key string, data []byte) error { return nil },
		func(ctx context.Context) error { return nil },
		func() map[string]interface{} { return make(map[string]interface{}) },
		func() int64 { return 5 },
		func() int64 { return 10 },
		func() int64 { return 2 },
	)

	// Delay slightly to allow time to pass for uptime
	time.Sleep(10 * time.Millisecond)

	req := &pb.GetStatusRequest{
		ApiVersion: "v1",
	}

	resp, err := service.GetStatus(context.Background(), req)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}

	if resp.ActiveConnections != 5 {
		t.Errorf("expected active_connections=5, got %d", resp.ActiveConnections)
	}

	if resp.TotalConnections != 10 {
		t.Errorf("expected total_connections=10, got %d", resp.TotalConnections)
	}

	if resp.QueriesBlocked != 2 {
		t.Errorf("expected queries_blocked=2, got %d", resp.QueriesBlocked)
	}

	if resp.UptimeSeconds == "" {
		t.Error("uptime_seconds not set")
	}
}

func TestConfigServiceGetStatusInvalidVersion(t *testing.T) {
	logger := zap.NewNop()

	service := NewConfigServiceServer(
		logger,
		func(ctx context.Context, key string) (interface{}, error) { return nil, nil },
		func(ctx context.Context, key string, data []byte) error { return nil },
		func(ctx context.Context) error { return nil },
		func() map[string]interface{} { return make(map[string]interface{}) },
		func() int64 { return 0 },
		func() int64 { return 0 },
		func() int64 { return 0 },
	)

	req := &pb.GetStatusRequest{
		ApiVersion: "v2",
	}

	_, err := service.GetStatus(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid api_version")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("error is not a gRPC status error")
	}

	if st.Code() != codes.Unimplemented {
		t.Errorf("expected code Unimplemented, got %v", st.Code())
	}
}
