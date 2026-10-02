package ws

func (h *Handler) ConnectionCount() int { h.mu.Lock(); defer h.mu.Unlock(); return h.connections }
