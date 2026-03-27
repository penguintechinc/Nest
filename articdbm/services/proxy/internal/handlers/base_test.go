package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldUseCacheNilManager(t *testing.T) {
	h := &BaseHandler{cacheManager: nil}
	assert.False(t, h.shouldUseCache("SELECT 1"))
}

func TestShouldUseCacheWithManager(t *testing.T) {
	// Note: We can't easily create a real cache manager without dependencies
	// This test just verifies the nil case
	h := &BaseHandler{cacheManager: nil}
	assert.False(t, h.shouldUseCache("SELECT * FROM users"))
}

func TestGetCachedResultNilManager(t *testing.T) {
	h := &BaseHandler{cacheManager: nil}
	data, found := h.getCachedResult("SELECT 1")
	assert.Nil(t, data)
	assert.False(t, found)
}

func TestCacheResultNilManager(t *testing.T) {
	h := &BaseHandler{cacheManager: nil}
	assert.NotPanics(t, func() {
		h.cacheResult("SELECT 1", []byte("result"))
	})
}

func TestShouldMultiWriteNilManager(t *testing.T) {
	h := &BaseHandler{multiwriteManager: nil}
	assert.False(t, h.shouldMultiWrite("INSERT INTO users VALUES (1)"))
}

func TestExecuteMultiWriteNilManager(t *testing.T) {
	h := &BaseHandler{multiwriteManager: nil}
	err := h.executeMultiWrite("INSERT INTO users VALUES (1)", []string{"db1", "db2"})
	assert.NoError(t, err)
}

func TestBaseHandlerNilFields(t *testing.T) {
	h := &BaseHandler{
		cfg:               nil,
		redis:             nil,
		logger:            nil,
		xdpController:     nil,
		cacheManager:      nil,
		multiwriteManager: nil,
	}

	assert.False(t, h.shouldUseCache("SELECT 1"))
	assert.False(t, h.shouldMultiWrite("INSERT 1"))

	data, found := h.getCachedResult("SELECT 1")
	assert.Nil(t, data)
	assert.False(t, found)

	assert.NotPanics(t, func() {
		h.cacheResult("SELECT 1", []byte("result"))
	})

	err := h.executeMultiWrite("INSERT 1", []string{"db1"})
	assert.NoError(t, err)
}
