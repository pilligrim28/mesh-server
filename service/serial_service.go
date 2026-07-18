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

// SerialService сервис для работы с Meshtastic через COM-порт (USB hub).
type SerialService struct {
	serialClient *client.MeshtasticSerialClient
	messageRepo  *repository.MessageRepository
	deviceRepo   *repository.DeviceRepository
	broadcaster  Broadcaster
	mu           sync.RWMutex
	running      bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewSerialService создает сервис для работы с COM-портом.
func NewSerialService(
	messageRepo *repository.MessageRepository,
	deviceRepo *repository.DeviceRepository,
	broadcaster Broadcaster,
) *SerialService {
	return &SerialService{
		messageRepo: messageRepo,
		deviceRepo:  deviceRepo,
		broadcaster: broadcaster,
	}
}

// Start запускает сервис с указанным COM-портом.
func (s *SerialService) Start(comPort string) error {
	return s.Connect(context.Background(), comPort)
}

// Connect подключается к COM-порту.
func (s *SerialService) Connect(ctx context.Context, comPort string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.Println("SerialService уже запущен")
		return nil
	}

	if comPort == "" {
		log.Println("SerialService: COM-порт не указан")
		return nil
	}

	s.serialClient = client.NewMeshtasticSerialClient(comPort)

	if err := s.serialClient.Connect(ctx); err != nil {
		log.Printf("SerialService: ошибка подключения к %s: %v", comPort, err)
		return err
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.running = true

	go s.processMessages()

	log.Printf("SerialService запущен на %s (USB Meshtastic hub)", comPort)
	return nil
}

// Stop останавливает сервис.
func (s *SerialService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	if s.cancel != nil {
		s.cancel()
	}

	if s.serialClient != nil {
		if err := s.serialClient.Disconnect(); err != nil {
			log.Printf("SerialService: ошибка отключения: %v", err)
		}
	}

	s.running = false
	s.serialClient = nil
	log.Println("SerialService остановлен")
}

func (s *SerialService) processMessages() {
	if s.serialClient == nil {
		return
	}

	messageCh := s.serialClient.GetMessages()
	positionCh := s.serialClient.GetPositions()

	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-messageCh:
			if msg.Inbound {
				s.handleInboundMessage(msg)
			}
		case pos := <-positionCh:
			s.handlePositionUpdate(pos)
		}
	}
}

func (s *SerialService) handleInboundMessage(msg client.SerialMessage) {
	log.Printf("SerialService: получено сообщение от %s: %s", msg.FromNode, msg.Text)

	device, err := s.deviceRepo.GetByNodeID(msg.FromNode)
	if err != nil {
		device = &models.Device{
			NodeID:   msg.FromNode,
			Name:     "Meshtastic-" + msg.FromNode[1:],
			LastSeen: time.Now(),
		}
		if err := s.deviceRepo.Create(device); err != nil {
			log.Printf("SerialService: ошибка создания устройства: %v", err)
			return
		}
	} else {
		_ = s.deviceRepo.UpdateLastSeen(device.ID)
	}

	message := &models.Message{
		DeviceID:  device.ID,
		FromNode:  msg.FromNode,
		ToNode:    msg.ToNode,
		Text:      msg.Text,
		Direction: "inbound",
		SentAt:    time.Now(),
	}

	if err := s.messageRepo.Create(message); err != nil {
		log.Printf("SerialService: ошибка сохранения сообщения: %v", err)
		return
	}

	log.Printf("SerialService: сообщение сохранено в БД (ID: %d)", message.ID)

	if s.broadcaster != nil {
		s.broadcaster.Broadcast(map[string]interface{}{
			"type":    "new_message",
			"message": message,
			"source":  "serial_hub",
		})
	}
}

func (s *SerialService) handlePositionUpdate(pos client.SerialPosition) {
	log.Printf("SerialService: позиция от %s: lat=%.6f, lon=%.6f, alt=%d",
		pos.NodeID, pos.Latitude, pos.Longitude, pos.Altitude)

	device, err := s.deviceRepo.GetByNodeID(pos.NodeID)
	if err != nil {
		device = &models.Device{
			NodeID:    pos.NodeID,
			Name:      "Meshtastic-" + pos.NodeID[1:],
			Latitude:  pos.Latitude,
			Longitude: pos.Longitude,
			Altitude:  float64(pos.Altitude),
			LastSeen:  time.Now(),
		}
		if createErr := s.deviceRepo.Create(device); createErr != nil {
			log.Printf("SerialService: ошибка создания устройства: %v", createErr)
			return
		}
	} else {
		_ = s.deviceRepo.UpdatePosition(device.ID, pos.Latitude, pos.Longitude, float64(pos.Altitude))
		_ = s.deviceRepo.UpdateLastSeen(device.ID)
	}

	if s.broadcaster != nil {
		s.broadcaster.Broadcast(map[string]interface{}{
			"type": "device_position",
			"position": map[string]interface{}{
				"node_id":   pos.NodeID,
				"latitude":  pos.Latitude,
				"longitude": pos.Longitude,
				"altitude":  pos.Altitude,
			},
			"source": "serial_hub",
		})
	}
}

// SendMessage отправляет сообщение в mesh через USB (реализует MeshSender).
func (s *SerialService) SendMessage(ctx context.Context, toNode, text string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running || s.serialClient == nil {
		return nil
	}

	return s.serialClient.SendMessage(ctx, toNode, text)
}

// IsConnected проверяет подключение.
func (s *SerialService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running && s.serialClient != nil && s.serialClient.IsConnected()
}

// GetPort возвращает текущий COM-порт.
func (s *SerialService) GetPort() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.serialClient == nil {
		return ""
	}
	return s.serialClient.GetPort()
}

// GetStatus возвращает статус USB-хаба.
func (s *SerialService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"connected": s.running && s.serialClient != nil && s.serialClient.IsConnected(),
		"port":      "",
		"transport": "usb_serial",
	}

	if s.serialClient != nil {
		status["port"] = s.serialClient.GetPort()
		if nodeNum := s.serialClient.GetNodeNum(); nodeNum != 0 {
			status["node_id"] = formatNodeID(nodeNum)
		}
	}

	return status
}

func formatNodeID(num uint32) string {
	return "!" + padHex(num)
}

func padHex(num uint32) string {
	return fmt.Sprintf("%08x", num)
}

// ScanPorts сканирует доступные COM-порты.
func (s *SerialService) ScanPorts() ([]string, error) {
	return client.ScanForDevices()
}

// Disconnect отключается от COM-порта.
func (s *SerialService) Disconnect() error {
	s.Stop()
	return nil
}
