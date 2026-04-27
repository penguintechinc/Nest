package provider

import (
	"fmt"
	"sync"
)

var (
	mu        sync.RWMutex
	providers = map[string]ExternalProvider{}
)

func Register(p ExternalProvider) {
	mu.Lock()
	defer mu.Unlock()
	providers[p.Name()] = p
}

func Get(name string) (ExternalProvider, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[name]
	if !ok {
		if generic, exists := providers["generic"]; exists {
			return generic, nil
		}
		return nil, fmt.Errorf("no provider registered for %q", name)
	}
	return p, nil
}

func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	return names
}
