package agenthub

func (h *Hub) ConnectionCount() int { h.mu.RLock(); defer h.mu.RUnlock(); return len(h.conns) }
