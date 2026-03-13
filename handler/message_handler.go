package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"mesh-server/client"
	"mesh-server/models"
	"mesh-server/repository"
)

type MessageHandler struct {
	messageRepo *repository.MessageRepository
	esp32Client *client.ESP32Client
	wsHandler   *WebSocketHandler
}

func NewMessageHandler(messageRepo *repository.MessageRepository, esp32Client *client.ESP32Client, wsHandler *WebSocketHandler) *MessageHandler {
	return &MessageHandler{
		messageRepo: messageRepo,
		esp32Client: esp32Client,
		wsHandler:   wsHandler,
	}
}

func (h *MessageHandler) Create(w http.ResponseWriter, r *http.Request) {
	var msg models.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if msg.DeviceID == 0 {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	if msg.FromNode == "" {
		http.Error(w, "from_node is required", http.StatusBadRequest)
		return
	}

	if msg.Text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	if msg.Direction == "" {
		msg.Direction = "outbound"
	}

	if msg.SentAt.IsZero() {
		msg.SentAt = time.Now()
	}

	// Отправка сообщения на ESP32 если клиент настроен
	if h.esp32Client != nil && msg.Direction == "outbound" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			_, err := h.esp32Client.SendMessage(ctx, &msg)
			if err != nil {
				log.Printf("Failed to send message to ESP32: %v", err)
			} else {
				log.Printf("Message sent to ESP32: %s", msg.Text)
			}
		}()
	}

	if err := h.messageRepo.Create(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Отправка уведомления через WebSocket
	if h.wsHandler != nil {
		h.wsHandler.Broadcast(map[string]interface{}{
			"type":    "new_message",
			"message": msg,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}

func (h *MessageHandler) GetByDeviceID(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	if deviceIDStr == "" {
		http.Error(w, "device_id parameter required", http.StatusBadRequest)
		return
	}

	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid device_id", http.StatusBadRequest)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	messages, err := h.messageRepo.GetByDeviceID(deviceID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (h *MessageHandler) GetOutbound(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	messages, err := h.messageRepo.GetOutbound(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}
