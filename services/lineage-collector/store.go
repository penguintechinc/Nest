package main

import (
	"fmt"
	"sync"
	"time"
)

type LineageEvent struct {
	ID           string                 `json:"id"`
	EventTime    time.Time              `json:"eventTime"`
	EventType    string                 `json:"eventType"`
	RunID        string                 `json:"runId"`
	JobName      string                 `json:"jobName"`
	JobNamespace string                 `json:"jobNamespace,omitempty"`
	Tenant       string                 `json:"tenant"`
	Inputs       []LineageDataset       `json:"inputs,omitempty"`
	Outputs      []LineageDataset       `json:"outputs,omitempty"`
	Producer     string                 `json:"producer,omitempty"`
	SchemaURL    string                 `json:"schemaURL,omitempty"`
	Raw          map[string]interface{} `json:"raw,omitempty"`
	ReceivedAt   time.Time              `json:"receivedAt"`
}

type LineageDataset struct {
	Namespace string                 `json:"namespace"`
	Name      string                 `json:"name"`
	Facets    map[string]interface{} `json:"facets,omitempty"`
}

type LineageStore struct {
	mu     sync.RWMutex
	events []*LineageEvent
}

func NewLineageStore() *LineageStore {
	return &LineageStore{
		events: make([]*LineageEvent, 0),
	}
}

func (s *LineageStore) Append(e *LineageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.ID == "" {
		e.ID = fmt.Sprintf("lineage-%d", time.Now().UnixNano())
	}
	e.ReceivedAt = time.Now()

	s.events = append(s.events, e)
}

func (s *LineageStore) Query(tenant, jobName, runID string, limit int) []*LineageEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	result := make([]*LineageEvent, 0)

	for i := len(s.events) - 1; i >= 0 && len(result) < limit; i-- {
		e := s.events[i]

		if tenant != "" && e.Tenant != tenant {
			continue
		}
		if jobName != "" && e.JobName != jobName {
			continue
		}
		if runID != "" && e.RunID != runID {
			continue
		}

		result = append(result, e)
	}

	return result
}

func (s *LineageStore) LineageForDataset(namespace, name string) []*LineageEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*LineageEvent, 0)

	for _, e := range s.events {
		for _, input := range e.Inputs {
			if input.Namespace == namespace && input.Name == name {
				result = append(result, e)
				break
			}
		}

		for _, output := range e.Outputs {
			if output.Namespace == namespace && output.Name == name {
				result = append(result, e)
				break
			}
		}
	}

	return result
}
