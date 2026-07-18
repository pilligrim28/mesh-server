package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	pb "buf.build/gen/go/meshtastic/protobufs/protocolbuffers/go/meshtastic"
	"go.bug.st/serial"
	"google.golang.org/protobuf/proto"
)

const (
	serialStart1        = 0x94
	serialStart2        = 0xC3
	serialHeaderLen     = 4
	serialMaxPacketSize = 512
	serialBroadcastNum  = 0xFFFFFFFF
)

const seenPacketsCleanThreshold = 1000

// MeshtasticSerialClient клиент для подключения к Meshtastic через COM-порт (USB).
type MeshtasticSerialClient struct {
	port        string
	baudRate    uint
	conn        io.ReadWriteCloser
	mu          sync.RWMutex
	connected   bool
	ctx         context.Context
	cancel      context.CancelFunc
	messageCh   chan SerialMessage
	positionCh  chan SerialPosition
	nodeNum     uint32
	seenPackets map[uint32]struct{}
	seenCount   int // счётчик для периодической очистки seenPackets
}

// SerialMessage сообщение от/к ESP32 через COM-порт.
type SerialMessage struct {
	FromNode string
	ToNode   string
	Text     string
	Inbound  bool
}

// SerialPosition позиция устройства из mesh-сети
type SerialPosition struct {
	NodeID    string
	Latitude  float64
	Longitude float64
	Altitude  int32
}

// SerialConfig конфигурация COM-порта
type SerialConfig struct {
	Port     string
	BaudRate uint
}

// DefaultSerialConfig конфигурация по умолчанию для Meshtastic
func DefaultSerialConfig() SerialConfig {
	return SerialConfig{
		Port:     "",
		BaudRate: 115200,
	}
}

// NewMeshtasticSerialClient создает клиент для подключения через COM-порт
func NewMeshtasticSerialClient(port string) *MeshtasticSerialClient {
	return &MeshtasticSerialClient{
		port:        port,
		baudRate:    115200,
		messageCh:   make(chan SerialMessage, 100),
		positionCh:  make(chan SerialPosition, 100),
		seenPackets: make(map[uint32]struct{}),
	}
}

// Connect подключается к COM-порту и инициализирует Meshtastic API.
func (c *MeshtasticSerialClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("уже подключен")
	}
	if c.port == "" {
		return fmt.Errorf("не указан COM-порт")
	}

	mode := &serial.Mode{
		BaudRate: int(c.baudRate),
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	conn, err := serial.Open(c.port, mode)
	if err != nil {
		return fmt.Errorf("ошибка открытия COM-порта %s: %w", c.port, err)
	}

	if port, ok := conn.(interface{ SetReadTimeout(time.Duration) error }); ok {
		_ = port.SetReadTimeout(200 * time.Millisecond)
	}

	c.conn = conn
	c.connected = true
	c.ctx, c.cancel = context.WithCancel(context.Background())

	if err := c.initializeRadio(ctx); err != nil {
		_ = conn.Close()
		c.conn = nil
		c.connected = false
		return fmt.Errorf("ошибка инициализации Meshtastic: %w", err)
	}

	go c.readLoop()
	log.Printf("Meshtastic Serial: подключен к %s (node !%08x)", c.port, c.nodeNum)
	return nil
}

// Disconnect отключается от COM-порта.
func (c *MeshtasticSerialClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
	}

	c.connected = false
	c.conn = nil
	log.Printf("Meshtastic Serial: отключен от %s", c.port)
	return nil
}

func (c *MeshtasticSerialClient) initializeRadio(ctx context.Context) error {
	wantConfig := &pb.ToRadio{
		PayloadVariant: &pb.ToRadio_WantConfigId{
			WantConfigId: uint32(time.Now().Unix()),
		},
	}

	if err := c.writeToRadio(wantConfig); err != nil {
		return err
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		packets, err := c.readFromRadio(500 * time.Millisecond)
		if err != nil {
			return err
		}

		for _, packet := range packets {
			if info := packet.GetMyInfo(); info != nil {
				c.nodeNum = info.MyNodeNum
			}
		}

		if c.nodeNum != 0 {
			return nil
		}
	}

	// Некоторые прошивки не отдают my_info сразу — продолжаем работу.
	log.Printf("Meshtastic Serial: node number не получен, продолжаем без него")
	return nil
}

func (c *MeshtasticSerialClient) readLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		packets, err := c.readFromRadio(1 * time.Second)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			log.Printf("Meshtastic Serial: ошибка чтения: %v", err)
			continue
		}

		for _, fromRadio := range packets {
			c.handleFromRadio(fromRadio)
		}
	}
}

func (c *MeshtasticSerialClient) handleFromRadio(fromRadio *pb.FromRadio) {
	if packet := fromRadio.GetPacket(); packet != nil {
		c.handleMeshPacket(packet)
	}
}

func (c *MeshtasticSerialClient) handleMeshPacket(packet *pb.MeshPacket) {
	data := packet.GetDecoded()
	if data == nil {
		return
	}

	portnum := data.GetPortnum()

	switch portnum {
	case pb.PortNum_TEXT_MESSAGE_APP:
		c.handleTextMessage(packet, data)

	case pb.PortNum_POSITION_APP:
		c.handlePositionMessage(packet, data)

	default:
		// Игнорируем другие типы пакетов (NODEINFO, TELEMETRY и т.д.)
	}
}

func (c *MeshtasticSerialClient) handleTextMessage(packet *pb.MeshPacket, data *pb.Data) {
	text := string(data.GetPayload())
	if text == "" {
		return
	}

	c.mu.Lock()
	if _, seen := c.seenPackets[packet.GetId()]; seen {
		c.mu.Unlock()
		return
	}
	c.seenPackets[packet.GetId()] = struct{}{}
	c.seenCount++
	if c.seenCount > seenPacketsCleanThreshold {
		c.seenPackets = make(map[uint32]struct{})
		c.seenCount = 0
	}
	c.mu.Unlock()

	toNode := formatSerialNodeID(packet.GetTo())
	if packet.GetTo() == serialBroadcastNum {
		toNode = "broadcast"
	}

	msg := SerialMessage{
		FromNode: formatSerialNodeID(packet.GetFrom()),
		ToNode:   toNode,
		Text:     text,
		Inbound:  true,
	}

	select {
	case c.messageCh <- msg:
	default:
		log.Printf("Meshtastic Serial: буфер сообщений переполнен")
	}
}

func (c *MeshtasticSerialClient) handlePositionMessage(packet *pb.MeshPacket, data *pb.Data) {
	// Декодируем Position из payload
	position := &pb.Position{}
	if err := proto.Unmarshal(data.GetPayload(), position); err != nil {
		log.Printf("Meshtastic Serial: ошибка декодирования позиции: %v", err)
		return
	}

	nodeID := formatSerialNodeID(packet.GetFrom())
	var lat, lon float64
	var alt int32
	if position.LatitudeI != nil {
		lat = float64(*position.LatitudeI) / 1e7
	}
	if position.LongitudeI != nil {
		lon = float64(*position.LongitudeI) / 1e7
	}
	if position.Altitude != nil {
		alt = *position.Altitude
	}

	if lat == 0 && lon == 0 {
		return // Игнорируем нулевые координаты
	}

	pos := SerialPosition{
		NodeID:    nodeID,
		Latitude:  lat,
		Longitude: lon,
		Altitude:  alt,
	}

	select {
	case c.positionCh <- pos:
		log.Printf("Meshtastic Serial: позиция от %s: lat=%.6f, lon=%.6f", nodeID, lat, lon)
	default:
		log.Printf("Meshtastic Serial: буфер позиций переполнен")
	}
}

// SendMessage отправляет текстовое сообщение в mesh через USB.
func (c *MeshtasticSerialClient) SendMessage(ctx context.Context, toNode, text string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("не подключен")
	}

	toNum := parseSerialNodeID(toNode)
	packetID := uint32(rand.Intn(2386827) + 1)

	toRadio := &pb.ToRadio{
		PayloadVariant: &pb.ToRadio_Packet{
			Packet: &pb.MeshPacket{
				To:      toNum,
				From:    c.nodeNum,
				Id:      packetID,
				WantAck: true,
				PayloadVariant: &pb.MeshPacket_Decoded{
					Decoded: &pb.Data{
						Portnum: pb.PortNum_TEXT_MESSAGE_APP,
						Payload: []byte(text),
					},
				},
			},
		},
	}

	if err := c.writeToRadio(toRadio); err != nil {
		return fmt.Errorf("ошибка отправки сообщения: %w", err)
	}

	log.Printf("Meshtastic Serial: отправлено на %s: %s", toNode, text)
	return nil
}

func (c *MeshtasticSerialClient) writeToRadio(toRadio *pb.ToRadio) error {
	payload, err := proto.Marshal(toRadio)
	if err != nil {
		return err
	}

	packet := make([]byte, serialHeaderLen+len(payload))
	packet[0] = serialStart1
	packet[1] = serialStart2
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(payload)))
	copy(packet[serialHeaderLen:], payload)

	_, err = c.conn.Write(packet)
	return err
}

func (c *MeshtasticSerialClient) readFromRadio(timeout time.Duration) ([]*pb.FromRadio, error) {
	deadline := time.Now().Add(timeout)
	processed := make([]byte, 0, serialMaxPacketSize)
	previous := byte(0)
	repeatCount := 0
	results := make([]*pb.FromRadio, 0)
	buf := make([]byte, 1)

	for time.Now().Before(deadline) {
		n, err := c.conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
				break
			}
			return results, err
		}
		if n == 0 {
			continue
		}

		b := buf[0]
		if b == previous {
			repeatCount++
		} else {
			repeatCount = 0
		}
		previous = b

		if repeatCount > 20 && len(processed) < serialHeaderLen {
			break
		}

		pointer := len(processed)
		processed = append(processed, b)

		if pointer == 0 && b != serialStart1 {
			processed = processed[:0]
			continue
		}
		if pointer == 1 && b != serialStart2 {
			processed = processed[:0]
			continue
		}
		if pointer < serialHeaderLen {
			continue
		}

		packetLength := int(processed[2])<<8 | int(processed[3])
		if packetLength > serialMaxPacketSize {
			processed = processed[:0]
			continue
		}

		if len(processed) < serialHeaderLen+packetLength {
			continue
		}

		fromRadio := &pb.FromRadio{}
		if err := proto.Unmarshal(processed[serialHeaderLen:serialHeaderLen+packetLength], fromRadio); err != nil {
			processed = processed[:0]
			continue
		}

		results = append(results, fromRadio)
		processed = processed[:0]
	}

	return results, nil
}

func (c *MeshtasticSerialClient) GetMessages() <-chan SerialMessage {
	return c.messageCh
}

func (c *MeshtasticSerialClient) GetPositions() <-chan SerialPosition {
	return c.positionCh
}

func (c *MeshtasticSerialClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *MeshtasticSerialClient) GetPort() string {
	return c.port
}

func (c *MeshtasticSerialClient) SetPort(port string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.port = port
}

func (c *MeshtasticSerialClient) GetNodeNum() uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeNum
}

// ScanForDevices сканирует доступные COM-порты.
func ScanForDevices() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	return ports, nil
}

func formatSerialNodeID(num uint32) string {
	return fmt.Sprintf("!%08x", num)
}

func parseSerialNodeID(nodeID string) uint32 {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || nodeID == "broadcast" || nodeID == "^all" {
		return serialBroadcastNum
	}
	if strings.HasPrefix(nodeID, "!") {
		nodeID = nodeID[1:]
	}
	var result uint32
	fmt.Sscanf(nodeID, "%x", &result)
	return result
}

// parseBinaryMessage оставлен для совместимости.
func (c *MeshtasticSerialClient) parseBinaryMessage(data []byte) (*SerialMessage, error) {
	if len(data) < serialHeaderLen {
		return nil, fmt.Errorf("слишком короткие данные")
	}
	if data[0] != serialStart1 || data[1] != serialStart2 {
		return nil, fmt.Errorf("неверные magic bytes")
	}

	length := binary.BigEndian.Uint16(data[2:4])
	if len(data) < serialHeaderLen+int(length) {
		return nil, fmt.Errorf("неполные данные")
	}

	fromRadio := &pb.FromRadio{}
	if err := proto.Unmarshal(data[serialHeaderLen:], fromRadio); err != nil {
		return nil, err
	}

	packet := fromRadio.GetPacket()
	if packet == nil {
		return nil, fmt.Errorf("нет mesh packet")
	}

	dataMsg := packet.GetDecoded()
	if dataMsg == nil || dataMsg.GetPortnum() != pb.PortNum_TEXT_MESSAGE_APP {
		return nil, fmt.Errorf("не текстовое сообщение")
	}

	return &SerialMessage{
		FromNode: formatSerialNodeID(packet.GetFrom()),
		ToNode:   formatSerialNodeID(packet.GetTo()),
		Text:     string(dataMsg.GetPayload()),
		Inbound:  true,
	}, nil
}
