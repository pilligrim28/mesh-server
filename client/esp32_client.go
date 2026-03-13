package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"mesh-server/models"
)

// ESP32Client - клиент для взаимодействия с устройством ESP32 через HTTP API
type ESP32Client struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
}

// ESP32MessageRequest - запрос на отправку сообщения
type ESP32MessageRequest struct {
	ToNode string `json:"to_node"`
	Text   string `json:"text"`
}

// ESP32MessageResponse - ответ от ESP32
type ESP32MessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
}

// NewESP32Client создает новый клиент для подключения к ESP32
func NewESP32Client(ip string) *ESP32Client {
	if ip == "" {
		ip = "http://localhost"
	}
	return &ESP32Client{
		baseURL: ip,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendMessage отправляет сообщение на ESP32 для передачи по LoRa/Bluetooth
func (c *ESP32Client) SendMessage(ctx context.Context, msg *models.Message) (*ESP32MessageResponse, error) {
	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()

	reqBody := ESP32MessageRequest{
		ToNode: msg.ToNode,
		Text:   msg.Text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/message", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to ESP32: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ESP32 returned status: %d", resp.StatusCode)
	}

	var result ESP32MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetStatus проверяет доступность ESP32
func (c *ESP32Client) GetStatus(ctx context.Context) (bool, error) {
	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()

	url := fmt.Sprintf("%s/health", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// SetBaseURL обновляет базовый URL для подключения
func (c *ESP32Client) SetBaseURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = url
	log.Printf("ESP32 client baseURL updated to: %s", url)
}

// GetBaseURL возвращает текущий базовый URL
func (c *ESP32Client) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}
