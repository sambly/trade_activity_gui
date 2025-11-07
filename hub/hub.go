package hub

import "sync"

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

func (h *Hub) UpdateConnection(service string, status ConnectionStatus, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.connections[service] == nil {
		h.connections[service] = &ConnectionInfo{}
	}

	h.connections[service].Service = service
	h.connections[service].Status = status
	h.connections[service].LastError = err
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
