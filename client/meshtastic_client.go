package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"mesh-server/models"
)

// MeshtasticClient клиент для подключения к Meshtastic устройству (ESP32)
type MeshtasticClient struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
}

// MeshtasticDevice информация об устройстве Meshtastic
type MeshtasticDevice struct {
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name"`
}

// MeshtasticNode узел в сети Meshtastic
type MeshtasticNode struct {
	Num      uint32         `json:"num"`
	User     MeshtasticUser `json:"user"`
	Position *Position      `json:"position,omitempty"`
}

// MeshtasticUser пользователь Meshtastic
type MeshtasticUser struct {
	ID        string `json:"id"`
	LongName  string `json:"long_name"`
	ShortName string `json:"short_name"`
}

// Position позиция устройства
type Position struct {
	LatitudeI  int32 `json:"latitude_i"`
	LongitudeI int32 `json:"longitude_i"`
	Altitude   int32 `json:"altitude"`
}

// ConvertPosition конвертирует позицию в float координаты
func (p *Position) ConvertPosition() (float64, float64, float64) {
	return float64(p.LatitudeI) / 1e7, float64(p.LongitudeI) / 1e7, float64(p.Altitude)
}

// ConvertToDiscoveredDevice конвертирует в обнаруженное устройство
func (d *MeshtasticDevice) ConvertToDiscoveredDevice(address string) models.DiscoveredDevice {
	return models.DiscoveredDevice{
		Address:      address,
		Name:         d.LongName,
		Type:         "wifi",
		Meshtastic:   true,
		LastSeen:     time.Now(),
		DiscoveredAt: time.Now(),
	}
}

// NewMeshtasticClient создает клиент для Meshtastic
func NewMeshtasticClient(ip string) *MeshtasticClient {
	baseURL := ip
	if len(ip) > 0 && ip[:7] != "http://" && ip[:8] != "https://" {
		baseURL = "http://" + ip
	}
	return &MeshtasticClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsMeshtasticDevice проверяет является ли устройство Meshtastic
func (c *MeshtasticClient) IsMeshtasticDevice(ctx context.Context) bool {
	url := fmt.Sprintf("%s/api/v1/mesh", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetDevice получает информацию об устройстве
func (c *MeshtasticClient) GetDevice(ctx context.Context) (*MeshtasticDevice, error) {
	url := fmt.Sprintf("%s/api/v1/mesh", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var device MeshtasticDevice
	if err := json.Unmarshal(body, &device); err != nil {
		return nil, err
	}
	return &device, nil
}

// GetMetrics получает метрики с устройства
func (c *MeshtasticClient) GetMetrics(ctx context.Context) (*MeshtasticDevice, error) {
	return c.GetDevice(ctx)
}

// GetNodes получает список узлов сети
func (c *MeshtasticClient) GetNodes(ctx context.Context) ([]MeshtasticNode, error) {
	url := fmt.Sprintf("%s/api/v1/nodes", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var nodes []MeshtasticNode
	if err := json.Unmarshal(body, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// SetBaseURL обновляет базовый URL
func (c *MeshtasticClient) SetBaseURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = url
}

// GetBaseURL возвращает текущий URL
func (c *MeshtasticClient) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}
