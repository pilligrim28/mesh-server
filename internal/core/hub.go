package core

import "sync"

// Message — событие для веб-клиентов (WebSocket).
type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub рассылает события всем подписчикам; медленные подписчики не блокируют сервер.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Message]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan Message]struct{}{}} }

func (h *Hub) Subscribe() (<-chan Message, func()) {
	ch := make(chan Message, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(typ string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- Message{Type: typ, Data: data}:
		default: // клиент не успевает — пропускаем, он догрузит состояние через REST
		}
	}
}
