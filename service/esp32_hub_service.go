package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mesh-server/client"
	"mesh-server/models"
	"mesh-server/repository"
)

// ESP32HubConfig конфигурация ESP32-хаба.
type ESP32HubConfig struct {
	Enabled      bool
	DeviceURL    string
	PollInterval int // секунды
}

// ESP32HubService опрашивает ESP32 с Meshtastic и синхронизирует mesh-сеть с сервером.
type ESP32HubService struct {
	hubClient   *client.MeshtasticHubClient
	fallback    *client.ESP32Client
	deviceRepo  *repository.DeviceRepository
	messageRepo *repository.MessageRepository
	broadcaster Broadcaster
	config      ESP32HubConfig

	mu        sync.RWMutex
	running   bool
	connected bool
	lastPoll  time.Time
	lastError string
	nodeCount int
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewESP32HubService создаёт сервис ESP32-хаба.
func NewESP32HubService(
	deviceRepo *repository.DeviceRepository,
	messageRepo *repository.MessageRepository,
	broadcaster Broadcaster,
	config ESP32HubConfig,
) *ESP32HubService {
	return &ESP32HubService{
		hubClient:   client.NewMeshtasticHubClient(config.DeviceURL),
		fallback:    client.NewESP32Client(config.DeviceURL),
		deviceRepo:  deviceRepo,
		messageRepo: messageRepo,
		broadcaster: broadcaster,
		config:      config,
	}
}

// Start запускает фоновый опрос ESP32-хаба.
func (s *ESP32HubService) Start(ctx context.Context) error {
	if !s.config.Enabled || s.config.DeviceURL == "" {
		log.Println("ESP32 hub disabled or URL not configured")
		return nil
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	hubCtx, cancel := context.WithCancel(ctx)
	s.ctx = hubCtx
	s.cancel = cancel
	s.running = true
	s.mu.Unlock()

	log.Printf("Starting ESP32 hub service for %s", s.config.DeviceURL)
	go s.pollLoop()

	return nil
}

// Stop останавливает сервис.
func (s *ESP32HubService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
	s.connected = false
	log.Println("ESP32 hub service stopped")
}

func (s *ESP32HubService) pollLoop() {
	interval := time.Duration(s.config.PollInterval) * time.Second
	if interval < 2*time.Second {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.pollOnce()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.pollOnce()
		}
	}
}

func (s *ESP32HubService) pollOnce() {
	ctx, cancel := context.WithTimeout(s.ctx, 12*time.Second)
	defer cancel()

	available := s.hubClient.IsAvailable(ctx)
	s.mu.Lock()
	s.connected = available
	s.lastPoll = time.Now()
	if !available {
		s.lastError = "Meshtastic HTTP API недоступен"
		s.mu.Unlock()
		return
	}
	s.lastError = ""
	s.mu.Unlock()

	if err := s.syncNodes(ctx); err != nil {
		log.Printf("ESP32 hub: failed to sync nodes: %v", err)
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
	}

	messages, err := s.hubClient.PollMessages(ctx)
	if err != nil {
		log.Printf("ESP32 hub: failed to poll messages: %v", err)
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}

	for _, msg := range messages {
		s.handleInboundMessage(msg)
	}
}

func (s *ESP32HubService) syncNodes(ctx context.Context) error {
	nodes, err := s.hubClient.SyncNodes(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.nodeCount = len(nodes)
	s.mu.Unlock()

	for _, node := range nodes {
		nodeID := fmt.Sprintf("!%08x", node.Num)
		if node.User.ID != "" {
			nodeID = node.User.ID
		}

		dev, err := s.deviceRepo.GetByNodeID(nodeID)
		if err != nil {
			dev = &models.Device{
				NodeID:   nodeID,
				Name:     node.User.LongName,
				LastSeen: time.Now(),
			}
		} else {
			if node.User.LongName != "" {
				dev.Name = node.User.LongName
			}
			dev.LastSeen = time.Now()
		}

		if node.Position != nil {
			lat, lon, alt := node.Position.ConvertPosition()
			dev.Latitude = lat
			dev.Longitude = lon
			dev.Altitude = alt
		}

		if err != nil {
			if createErr := s.deviceRepo.Create(dev); createErr != nil {
				log.Printf("ESP32 hub: failed to create device %s: %v", nodeID, createErr)
			}
		} else {
			if node.Position != nil {
				_ = s.deviceRepo.UpdatePosition(dev.ID, dev.Latitude, dev.Longitude, dev.Altitude)
			}
			_ = s.deviceRepo.UpdateLastSeen(dev.ID)
		}
	}

	return nil
}

func (s *ESP32HubService) handleInboundMessage(msg client.HubMessage) {
	device, err := s.deviceRepo.GetByNodeID(msg.FromNode)
	if err != nil {
		device = &models.Device{
			NodeID:   msg.FromNode,
			Name:     "Mesh-" + msg.FromNode,
			LastSeen: time.Now(),
		}
		if createErr := s.deviceRepo.Create(device); createErr != nil {
			log.Printf("ESP32 hub: failed to create device %s: %v", msg.FromNode, createErr)
			return
		}
	} else {
		_ = s.deviceRepo.UpdateLastSeen(device.ID)
	}

	dbMsg := &models.Message{
		DeviceID:  device.ID,
		FromNode:  msg.FromNode,
		ToNode:    msg.ToNode,
		Text:      msg.Text,
		Direction: "inbound",
		SentAt:    time.Now(),
	}

	if err := s.messageRepo.Create(dbMsg); err != nil {
		log.Printf("ESP32 hub: failed to save message: %v", err)
		return
	}

	log.Printf("ESP32 hub: inbound message from %s: %s", msg.FromNode, msg.Text)

	if s.broadcaster != nil {
		s.broadcaster.Broadcast(map[string]interface{}{
			"type":    "new_message",
			"message": dbMsg,
			"source":  "esp32_hub",
		})
	}
}

// SendMessage отправляет сообщение в mesh через ESP32-хаб.
func (s *ESP32HubService) SendMessage(ctx context.Context, toNode, text string) error {
	if s.hubClient.IsAvailable(ctx) {
		return s.hubClient.SendTextMessage(ctx, toNode, text)
	}

	// Fallback для кастомной прошивки mesh-server.
	msg := &models.Message{
		ToNode: toNode,
		Text:   text,
	}
	_, err := s.fallback.SendMessage(ctx, msg)
	return err
}

// IsConnected возвращает true, если хаб доступен.
func (s *ESP32HubService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// GetStatus возвращает статус ESP32-хаба.
func (s *ESP32HubService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"enabled":     s.config.Enabled,
		"running":     s.running,
		"connected":   s.connected,
		"device_url":  s.config.DeviceURL,
		"node_count":  s.nodeCount,
		"last_poll":   s.lastPoll,
		"last_error":  s.lastError,
		"poll_interval": s.config.PollInterval,
	}
}

// SetDeviceURL обновляет URL устройства-хаба.
func (s *ESP32HubService) SetDeviceURL(url string) {
	s.mu.Lock()
	s.config.DeviceURL = url
	s.mu.Unlock()
	s.hubClient.SetBaseURL(url)
	s.fallback.SetBaseURL(url)
}
