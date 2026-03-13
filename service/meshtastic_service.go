package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"mesh-server/models"
	"mesh-server/repository"
)

// Broadcaster интерфейс для отправки уведомлений
type Broadcaster interface {
	Broadcast(data interface{})
}

// MeshtasticService сервис для интеграции с Meshtastic MQTT
type MeshtasticService struct {
	client      mqtt.Client
	config      MQTTConfig
	connected   bool
	mu          sync.RWMutex
	deviceRepo  *repository.DeviceRepository
	messageRepo *repository.MessageRepository
	broadcaster Broadcaster
	onMessage   func(*MeshtasticMessage)
}

// MQTTConfig конфигурация MQTT подключения
type MQTTConfig struct {
	Enabled             bool   `json:"enabled"`
	Server              string `json:"server"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	RootTopic           string `json:"root_topic"`
	JSONEnabled         bool   `json:"json_enabled"`
	EncryptionEnabled   bool   `json:"encryption_enabled"`
	MapReportingEnabled bool   `json:"map_reporting_enabled"`
	MapReportInterval   int    `json:"map_report_interval"`
}

// MeshtasticMessage сообщение от Meshtastic
type MeshtasticMessage struct {
	From      uint32      `json:"from"`
	To        uint32      `json:"to"`
	Channel   int         `json:"channel"`
	PacketID  uint32      `json:"id"`
	Timestamp uint32      `json:"timestamp"`
	Payload   string      `json:"payload"`
	Decoded   DecodedInfo `json:"decoded,omitempty"`
	SNR       float32     `json:"snr"`
	RSSI      int         `json:"rssi"`
	RXTime    float64     `json:"rx_time"`
	NodeID    string      `json:"node_id"`
}

// DecodedInfo декодированная информация
type DecodedInfo struct {
	PortNum   string      `json:"portnum"`
	Payload   interface{} `json:"payload"`
	RequestID uint32      `json:"request_id"`
}

// MapReport отчёт о карте для Meshtastic Map
type MapReport struct {
	LongName        string  `json:"long_name"`
	ShortName       string  `json:"short_name"`
	ID              string  `json:"id"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	Altitude        int32   `json:"altitude"`
	HardwareModel   string  `json:"hardware_model"`
	FirmwareVersion string  `json:"firmware_version"`
	Region          string  `json:"region"`
	ModemPreset     string  `json:"modem_preset"`
	PrimaryChannel  string  `json:"primary_channel"`
	PublicAddress   bool    `json:"public_address"`
	OnlineNodes     int     `json:"online_nodes"`
	LastHeard       int64   `json:"last_heard"`
}

// NewMeshtasticService создает новый MQTT сервис
func NewMeshtasticService(
	deviceRepo *repository.DeviceRepository,
	messageRepo *repository.MessageRepository,
	broadcaster Broadcaster,
	config MQTTConfig,
) *MeshtasticService {
	return &MeshtasticService{
		config:      config,
		deviceRepo:  deviceRepo,
		messageRepo: messageRepo,
		broadcaster: broadcaster,
	}
}

// Start запускает MQTT сервис
func (s *MeshtasticService) Start(ctx context.Context) error {
	if !s.config.Enabled {
		log.Println("MQTT disabled, skipping start")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connected {
		return nil
	}

	log.Printf("Connecting to MQTT broker %s...", s.config.Server)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:1883", s.config.Server))
	opts.SetUsername(s.config.Username)
	opts.SetPassword(s.config.Password)
	opts.SetClientID(fmt.Sprintf("mesh-server-%d", time.Now().UnixNano()))
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetOrderMatters(false)

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Printf("MQTT connected to %s", s.config.Server)
		s.connected = true
		s.subscribeToTopics(client)
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
		s.connected = false
	})

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		s.handleMessage(msg.Topic(), msg.Payload())
	})

	s.client = mqtt.NewClient(opts)
	token := s.client.Connect()
	token.Wait()

	if token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT: %w", token.Error())
	}

	if s.config.MapReportingEnabled {
		go s.startMapReporting(ctx)
	}

	return nil
}

// Stop останавливает сервис
func (s *MeshtasticService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil && s.connected {
		s.client.Disconnect(1000)
		s.connected = false
		log.Println("MQTT disconnected")
	}
}

func (s *MeshtasticService) subscribeToTopics(client mqtt.Client) {
	topics := []string{
		s.getRootTopic() + "/2/e/#",
		s.getRootTopic() + "/2/json/#",
		s.getRootTopic() + "/stat/#",
	}

	for _, topic := range topics {
		log.Printf("Subscribing to: %s", topic)
		token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			s.handleMessage(msg.Topic(), msg.Payload())
		})
		token.Wait()
	}
}

func (s *MeshtasticService) getRootTopic() string {
	if s.config.RootTopic != "" {
		return s.config.RootTopic
	}
	return "msh/RU"
}

func (s *MeshtasticService) handleMessage(topic string, payload []byte) {
	log.Printf("MQTT message on %s", topic)

	var msg MeshtasticMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		log.Printf("Failed to parse MQTT message: %v", err)
		return
	}

	fromNode := fmt.Sprintf("!%08x", msg.From)
	toNode := fmt.Sprintf("!%08x", msg.To)

	if msg.To == 0xFFFFFFFF {
		toNode = "broadcast"
	}

	device, err := s.deviceRepo.GetByNodeID(fromNode)
	if err != nil {
		device = &models.Device{
			NodeID:   fromNode,
			Name:     fmt.Sprintf("Node %s", fromNode),
			LastSeen: time.Now(),
		}
		s.deviceRepo.Create(device)
	} else {
		s.deviceRepo.UpdateLastSeen(device.ID)
	}

	dbMsg := &models.Message{
		DeviceID:  device.ID,
		FromNode:  fromNode,
		ToNode:    toNode,
		Text:      msg.Payload,
		Direction: "inbound",
		SentAt:    time.Now(),
	}

	if err := s.messageRepo.Create(dbMsg); err != nil {
		log.Printf("Failed to save message: %v", err)
		return
	}

	log.Printf("Saved inbound message from %s: %s", fromNode, msg.Payload)

	if s.broadcaster != nil {
		s.broadcaster.Broadcast(map[string]interface{}{
			"type": "new_message",
			"message": map[string]interface{}{
				"id":        dbMsg.ID,
				"device_id": dbMsg.DeviceID,
				"from_node": dbMsg.FromNode,
				"to_node":   dbMsg.ToNode,
				"text":      dbMsg.Text,
				"direction": dbMsg.Direction,
				"sent_at":   dbMsg.SentAt,
			},
		})
	}

	if s.onMessage != nil {
		s.onMessage(&msg)
	}
}

func (s *MeshtasticService) SendMessage(fromNode, toNode, text string, channel int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.connected {
		return fmt.Errorf("MQTT not connected")
	}

	msg := MeshtasticMessage{
		From:      parseNodeID(fromNode),
		To:        parseNodeID(toNode),
		Channel:   channel,
		PacketID:  uint32(time.Now().Unix()),
		Timestamp: uint32(time.Now().Unix()),
		Payload:   text,
		Decoded: DecodedInfo{
			PortNum: "TEXT_MESSAGE_APP",
			Payload: text,
		},
	}

	payload, _ := json.Marshal(msg)
	topic := s.getRootTopic() + "/2/e/" + fromNode
	token := s.client.Publish(topic, 0, false, payload)
	token.Wait()

	log.Printf("Sent message to %s via MQTT", topic)
	return token.Error()
}

func (s *MeshtasticService) SendMapReport(report MapReport) error {
	if !s.connected {
		return fmt.Errorf("MQTT not connected")
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}

	topic := s.getRootTopic() + "/2/map/" + report.ID
	token := s.client.Publish(topic, 0, false, payload)
	token.Wait()

	log.Printf("Sent map report for %s", report.LongName)
	return token.Error()
}

func (s *MeshtasticService) startMapReporting(ctx context.Context) {
	interval := time.Duration(s.config.MapReportInterval) * time.Second
	if interval < 60*time.Second {
		interval = 3600 * time.Second
	}

	log.Printf("Map reporting started (interval: %v)", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendMapReport()
		}
	}
}

func (s *MeshtasticService) sendMapReport() {
	devices, err := s.deviceRepo.GetAll()
	if err != nil {
		log.Printf("Failed to get devices for map report: %v", err)
		return
	}

	var lat, lon float64 = 55.7558, 37.6173
	var altitude int32 = 150

	if len(devices) > 0 && devices[0].Latitude != 0 {
		lat = devices[0].Latitude
		lon = devices[0].Longitude
		altitude = int32(devices[0].Altitude)
	}

	report := MapReport{
		LongName:        "Mesh Server",
		ShortName:       "MS",
		ID:              "!server001",
		Latitude:        lat,
		Longitude:       lon,
		Altitude:        altitude,
		HardwareModel:   "SERVER",
		FirmwareVersion: "1.0.0",
		Region:          "RU",
		ModemPreset:     "LONG_FAST",
		PrimaryChannel:  "LongFast",
		PublicAddress:   true,
		OnlineNodes:     len(devices),
		LastHeard:       time.Now().Unix(),
	}

	if err := s.SendMapReport(report); err != nil {
		log.Printf("Failed to send map report: %v", err)
	}
}

func (s *MeshtasticService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected && s.client.IsConnected()
}

func (s *MeshtasticService) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"connected":      s.connected,
		"server":         s.config.Server,
		"root_topic":     s.getRootTopic(),
		"json_enabled":   s.config.JSONEnabled,
		"map_reporting":  s.config.MapReportingEnabled,
	}

	if devices, err := s.deviceRepo.GetAll(); err == nil {
		status["devices_count"] = len(devices)
	}

	return status
}

func (s *MeshtasticService) Reconnect() error {
	s.Stop()
	time.Sleep(1 * time.Second)
	return s.Start(context.Background())
}

func (s *MeshtasticService) SetOnMessage(handler func(*MeshtasticMessage)) {
	s.onMessage = handler
}

func parseNodeID(nodeID string) uint32 {
	if len(nodeID) == 0 {
		return 0
	}
	if nodeID[0] == '!' {
		nodeID = nodeID[1:]
	}
	var result uint32
	fmt.Sscanf(nodeID, "%x", &result)
	return result
}

func GetDefaultMQTTConfig() MQTTConfig {
	return MQTTConfig{
		Enabled:             false,
		Server:              "mqtt.meshtastic.org",
		Username:            "meshdev",
		Password:            "large4cats",
		RootTopic:           "msh/RU",
		JSONEnabled:         true,
		EncryptionEnabled:   false,
		MapReportingEnabled: false,
		MapReportInterval:   3600,
	}
}
