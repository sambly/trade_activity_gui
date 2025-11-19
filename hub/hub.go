package hub

import (
	"fmt"
	"sync"
)

type ConnectionStatus string

const (
	Connected    ConnectionStatus = "connected"
	Disconnected ConnectionStatus = "disconnected"
	Error        ConnectionStatus = "error"
)

type ConnectionInfo struct {
	Service   string
	Status    ConnectionStatus
	LastError error
}

type Hub struct {
	mu          sync.RWMutex
	status      ConnectionStatus
	connections map[string]*ConnectionInfo
}

func NewHub() *Hub {
	return &Hub{
		status:      Connected,
		connections: make(map[string]*ConnectionInfo),
	}
}

func (h *Hub) UpdateStatus(status ConnectionStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = status
}

func (h *Hub) GetStatus() ConnectionStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

func (h *Hub) AddConnection(service string, status ConnectionStatus, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.connections[service] = &ConnectionInfo{
		Service:   service,
		Status:    status,
		LastError: err,
	}
}

func (h *Hub) UpdateConnection(service string, status ConnectionStatus, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conn, exists := h.connections[service]; exists {
		conn.Status = status
		conn.LastError = err
	}
}

func (h *Hub) GetConnectionStatus(service string) *ConnectionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if conn, exists := h.connections[service]; exists {
		return conn
	}
	return nil
}

func (h *Hub) GetOverallStatus() ConnectionStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.status == Error {
		return Error
	}

	if len(h.connections) == 0 {
		return h.status
	}

	hasError := false
	hasDisconnected := false

	for _, conn := range h.connections {
		if conn.Status == Error {
			hasError = true
		}
		if conn.Status == Disconnected {
			hasDisconnected = true
		}
	}

	if hasError || h.status == Error {
		return Error
	}

	if hasDisconnected || h.status == Disconnected {
		return Disconnected
	}

	return Connected
}

func (h *Hub) RemoveConnection(service string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, service)
}

func (h *Hub) DebugConnections() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fmt.Printf("=== Connections Debug ===\n")
	fmt.Printf("Total connections: %d\n", len(h.connections))
	fmt.Printf("Global status: %v\n", h.status)

	for service, conn := range h.connections {
		fmt.Printf("  %s: Status=%v, Error=%v\n",
			service, conn.Status, conn.LastError)
	}
	fmt.Printf("=========================\n")
}
