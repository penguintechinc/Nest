package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncConnection(t *testing.T) {
	assert.NotPanics(t, func() {
		IncConnection("mysql")
	})
}

func TestDecConnection(t *testing.T) {
	assert.NotPanics(t, func() {
		DecConnection("postgresql")
	})
}

func TestIncQuery(t *testing.T) {
	assert.NotPanics(t, func() {
		IncQuery("mysql", false)
	})
	assert.NotPanics(t, func() {
		IncQuery("mysql", true)
	})
}

func TestRecordQueryDuration(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordQueryDuration("mysql", false, 0.5)
	})
	assert.NotPanics(t, func() {
		RecordQueryDuration("postgresql", true, 1.2)
	})
}

func TestIncAuthFailure(t *testing.T) {
	assert.NotPanics(t, func() {
		IncAuthFailure("mysql")
	})
}

func TestIncSQLInjection(t *testing.T) {
	assert.NotPanics(t, func() {
		IncSQLInjection("mysql")
	})
}

func TestIncBackendError(t *testing.T) {
	assert.NotPanics(t, func() {
		IncBackendError("mysql", "backend1")
	})
}

func TestIncConfigReload(t *testing.T) {
	assert.NotPanics(t, func() {
		IncConfigReload()
	})
}

func TestMultipleIncConnections(t *testing.T) {
	assert.NotPanics(t, func() {
		for i := 0; i < 10; i++ {
			IncConnection("mysql")
		}
		for i := 0; i < 5; i++ {
			DecConnection("mysql")
		}
	})
}

func TestMultipleQueryTypes(t *testing.T) {
	assert.NotPanics(t, func() {
		IncQuery("mysql", false)
		IncQuery("mysql", true)
		IncQuery("postgresql", false)
		IncQuery("postgresql", true)
		IncQuery("mongodb", false)
		IncQuery("redis", true)
	})
}

func TestRecordVariousDurations(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordQueryDuration("mysql", false, 0.001)
		RecordQueryDuration("mysql", false, 0.1)
		RecordQueryDuration("mysql", false, 1.0)
		RecordQueryDuration("mysql", true, 5.0)
	})
}
