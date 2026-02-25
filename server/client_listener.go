package server

import (
	"context"
	"sync"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type ClientListener interface {
	ID() uint64
	Start(ctx context.Context)
	Close(ctx context.Context)
}

type ClientListenerManager struct {
	mutex     sync.Mutex
	listeners map[uint64]ClientListener
}

func newClientListenerManager() *ClientListenerManager {
	return &ClientListenerManager{
		mutex:     sync.Mutex{},
		listeners: make(map[uint64]ClientListener),
	}
}

func (m *ClientListenerManager) AddListener(ctx context.Context, listener ClientListener) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.listeners[listener.ID()] = listener

	return nil
}

func (m *ClientListenerManager) RemoveListener(id uint64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.listeners, id)

	return nil
}

func (m *ClientListenerManager) GetListener(id uint64) (ClientListener, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	listener, ok := m.listeners[id]
	return listener, ok
}

func (m *ClientListenerManager) CloseAll(ctx context.Context) {

	for _, listener := range m.listeners {
		listener.Close(ctx)
	}

	log.CtxInfof(ctx, "ClientListenerManager CloseAll done")
}
