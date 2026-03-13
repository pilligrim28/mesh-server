package service

import (
	"context"
	"log"
	"sync"
	"time"

	"mesh-server/client"
	"mesh-server/models"
	"mesh-server/repository"
)

// SerialService сервис для работы с Meshtastic через COM-порт
type SerialService struct {
	serialClient *client.MeshtasticSerialClient
	messageRepo  *repository.MessageRepository
	deviceRepo   *repository.DeviceRepository
	mu           sync.RWMutex
	running      bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewSerialService создает сервис для работы с COM-портом
func NewSerialService(messageRepo *repository.MessageRepository, deviceRepo *repository.DeviceRepository) *SerialService {
	return &SerialService{
		messageRepo: messageRepo,
		deviceRepo:  deviceRepo,
	}
}

// Start запускает сервис с указанным COM-портом
func (s *SerialService) Start(comPort string) error {
	return s.Connect(context.Background(), comPort)
}

// Connect подключается к COM-порту
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

	// Запускаем обработку входящих сообщений
	go s.processMessages()

	log.Printf("SerialService запущен на %s", comPort)
	return nil
}

// Stop останавливает сервис
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

// processMessages обрабатывает входящие сообщения
func (s *SerialService) processMessages() {
	if s.serialClient == nil {
		return
	}

	messageCh := s.serialClient.GetMessages()

	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-messageCh:
			if msg.Inbound {
				s.handleInboundMessage(msg)
			}
		}
	}
}

// handleInboundMessage обрабатывает входящее сообщение
func (s *SerialService) handleInboundMessage(msg client.SerialMessage) {
	log.Printf("SerialService: получено сообщение от %s: %s", msg.FromNode, msg.Text)

	// Получаем или создаем устройство для этого узла
	device, err := s.deviceRepo.GetByNodeID(msg.FromNode)
	if err != nil {
		// Создаем новое устройство
		device = &models.Device{
			NodeID:   msg.FromNode,
			Name:     "Meshtastic-" + msg.FromNode[1:], // Убираем "!"
			LastSeen: time.Now(),
		}
		if err := s.deviceRepo.Create(device); err != nil {
			log.Printf("SerialService: ошибка создания устройства: %v", err)
			return
		}
	}

	// Сохраняем сообщение
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
}

// SendMessage отправляет сообщение через COM-порт
func (s *SerialService) SendMessage(toNode, text string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running || s.serialClient == nil {
		return nil // Сервис не запущен, игнорируем
	}

	ctx := context.Background()
	return s.serialClient.SendMessage(ctx, toNode, text)
}

// IsConnected проверяет подключение
func (s *SerialService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running && s.serialClient != nil && s.serialClient.IsConnected()
}

// GetPort возвращает текущий COM-порт
func (s *SerialService) GetPort() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.serialClient == nil {
		return ""
	}
	return s.serialClient.GetPort()
}

// ScanPorts сканирует доступные COM-порты
func (s *SerialService) ScanPorts() ([]string, error) {
	return client.ScanForDevices()
}

// Disconnect отключается от COM-порта
func (s *SerialService) Disconnect() error {
	s.Stop()
	return nil
}
