package server

import (
	"context"
	"sync"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type ClientManager struct {
	mutex     sync.Mutex
	clientMap map[uint64]*Client
}

func newClientManager() *ClientManager {
	return &ClientManager{
		clientMap: make(map[uint64]*Client),
	}
}

func (cm *ClientManager) AddClient(ctx context.Context, client *Client) bool {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if _, ok := cm.clientMap[client.ClientID()]; ok {
		log.CtxErrorf(ctx, "clientID %d already exists", client.ClientID())
		return false
	}

	cm.clientMap[client.ClientID()] = client
	return true
}

func (cm *ClientManager) GetClient(ctx context.Context, clientID uint64) *Client {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.clientMap[clientID]
}

func (cm *ClientManager) RemoveClient(ctx context.Context, clientID uint64) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	delete(cm.clientMap, clientID)
}
