package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ESP32HealbeClient HTTP-клиент для Healbe-моста на ESP32.
type ESP32HealbeClient struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
}

// ESP32HealbeData данные с часов через ESP32.
type ESP32HealbeData struct {
	DeviceID    string `json:"device_id"`
	MAC         string `json:"mac"`
	HeartRate   int    `json:"heart_rate"`
	StressLevel int    `json:"stress_level"`
	Battery     int    `json:"battery"`
	Connected   bool   `json:"connected"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// NewESP32HealbeClient создаёт клиент ESP32 Healbe bridge.
func NewESP32HealbeClient(baseURL string) *ESP32HealbeClient {
	return &ESP32HealbeClient{
		baseURL: normalizeMeshtasticURL(baseURL),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetBaseURL обновляет URL ESP32.
func (c *ESP32HealbeClient) SetBaseURL(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = normalizeMeshtasticURL(baseURL)
}

// GetBaseURL возвращает URL ESP32.
func (c *ESP32HealbeClient) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// IsConfigured проверяет, задан ли URL.
func (c *ESP32HealbeClient) IsConfigured() bool {
	return c.GetBaseURL() != ""
}

// Connect просит ESP32 подключиться к часам Healbe по MAC.
func (c *ESP32HealbeClient) Connect(ctx context.Context, mac string) error {
	baseURL := c.GetBaseURL()
	if baseURL == "" {
		return fmt.Errorf("ESP32 URL is not configured")
	}

	body, _ := json.Marshal(map[string]string{"mac": mac})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/healbe/connect", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ESP32 healbe connect failed: %d %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Disconnect отключает Healbe на ESP32.
func (c *ESP32HealbeClient) Disconnect(ctx context.Context) error {
	baseURL := c.GetBaseURL()
	if baseURL == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/healbe/disconnect", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetData получает последние данные с ESP32.
func (c *ESP32HealbeClient) GetData(ctx context.Context) (*ESP32HealbeData, error) {
	baseURL := c.GetBaseURL()
	if baseURL == "" {
		return nil, fmt.Errorf("ESP32 URL is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/healbe/data", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESP32 healbe data status: %d", resp.StatusCode)
	}

	var data ESP32HealbeData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

// GetStatus возвращает статус Healbe-моста на ESP32.
func (c *ESP32HealbeClient) GetStatus(ctx context.Context) (map[string]interface{}, error) {
	baseURL := c.GetBaseURL()
	if baseURL == "" {
		return map[string]interface{}{"configured": false}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/healbe/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return status, nil
}
