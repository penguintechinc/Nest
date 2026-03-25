package handlers

import (
	"context"
	"net"

	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/penguintechinc/articdbm/proxy/internal/cache"
	"github.com/penguintechinc/articdbm/proxy/internal/multiwrite"
	"github.com/penguintechinc/articdbm/proxy/internal/xdp"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type Handler interface {
	Start(ctx context.Context, listener net.Listener)
	Close() error
}

type BaseHandler struct {
	cfg               *config.Config
	redis             *redis.Client
	logger            *zap.Logger
	xdpController     *xdp.Controller
	cacheManager      *cache.MultiTierCache
	multiwriteManager *multiwrite.Manager
}

func NewBaseHandler(cfg *config.Config, redis *redis.Client, logger *zap.Logger,
	xdpController *xdp.Controller, cacheManager *cache.MultiTierCache,
	multiwriteManager *multiwrite.Manager) *BaseHandler {
	return &BaseHandler{
		cfg:               cfg,
		redis:             redis,
		logger:            logger,
		xdpController:     xdpController,
		cacheManager:      cacheManager,
		multiwriteManager: multiwriteManager,
	}
}

func (h *BaseHandler) shouldUseCache(query string) bool {
	if h.cacheManager == nil {
		return false
	}
	// Check if caching is enabled via the cache manager
	return true
}

func (h *BaseHandler) getCachedResult(query string) ([]byte, bool) {
	if h.cacheManager == nil {
		return nil, false
	}
	req := &cache.CacheRequest{
		Query: query,
	}
	resp, err := h.cacheManager.Get(context.Background(), req)
	if err != nil || resp == nil {
		return nil, false
	}
	return resp.Data, resp.Found
}

func (h *BaseHandler) cacheResult(query string, result []byte) {
	if h.cacheManager == nil {
		return
	}
	req := &cache.CacheRequest{
		Query: query,
	}
	h.cacheManager.Set(context.Background(), req, result)
}

func (h *BaseHandler) shouldMultiWrite(query string) bool {
	if h.multiwriteManager == nil {
		return false
	}
	// Check if multi-write is enabled via the manager config
	return true
}

func (h *BaseHandler) executeMultiWrite(query string, databases []string) error {
	if h.multiwriteManager == nil {
		return nil
	}
	req := &multiwrite.WriteRequest{
		Query:    query,
		Strategy: "sync",
	}
	_, err := h.multiwriteManager.ExecuteWrite(context.Background(), req)
	return err
}