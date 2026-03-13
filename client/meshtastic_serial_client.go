package client

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

// MeshtasticSerialClient клиент для подключения к Meshtastic через COM-порт (USB)
type MeshtasticSerialClient struct {
	port       string
	baudRate   uint
	conn       io.ReadWriteCloser
	mu         sync.RWMutex
	connected  bool
	ctx        context.Context
	cancel     context.CancelFunc
	messageCh  chan SerialMessage
}

// SerialMessage сообщение от/к ESP32 через COM-порт
type SerialMessage struct {
	FromNode string
	ToNode   string
	Text     string
	Inbound  bool
}

// SerialConfig конфигурация COM-порта
type SerialConfig struct {
	Port     string
	BaudRate uint
}

// DefaultSerialConfig конфигурация по умолчанию для Meshtastic
func DefaultSerialConfig() SerialConfig {
	return SerialConfig{
		Port:     "", // Нужно указать при создании
		BaudRate: 115200,
	}
}

// NewMeshtasticSerialClient создает клиент для подключения через COM-порт
func NewMeshtasticSerialClient(port string) *MeshtasticSerialClient {
	return &MeshtasticSerialClient{
		port:      port,
		baudRate:  115200,
		connected: false,
		messageCh: make(chan SerialMessage, 100),
	}
}

// Connect подключается к COM-порту
func (c *MeshtasticSerialClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("уже подключен")
	}

	if c.port == "" {
		return fmt.Errorf("не указан COM-порт")
	}

	// Конфигурация последовательного порта
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

	c.conn = conn
	c.connected = true

	// Создаем контекст для фонового чтения
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Запускаем фоновое чтение сообщений
	go c.readLoop()

	log.Printf("Meshtastic Serial: подключен к %s", c.port)
	return nil
}

// Disconnect отключается от COM-порта
func (c *MeshtasticSerialClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	// Останавливаем фоновое чтение
	if c.cancel != nil {
		c.cancel()
	}

	// Закрываем соединение
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

// readLoop фоновое чтение сообщений из COM-порта
func (c *MeshtasticSerialClient) readLoop() {
	if c.conn == nil {
		return
	}

	reader := bufio.NewReader(c.conn)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF && c.ctx.Err() == nil {
					log.Printf("Meshtastic Serial: ошибка чтения: %v", err)
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Парсим сообщение Meshtastic
			msg := c.parseMessage(line)
			if msg != nil {
				select {
				case c.messageCh <- *msg:
				default:
					log.Printf("Meshtastic Serial: буфер сообщений переполнен")
				}
			}
		}
	}
}

// parseMessage парсит строку в сообщение
// Meshtastic отправляет данные в формате:
// от !12345678: Текст сообщения
func (c *MeshtasticSerialClient) parseMessage(line string) *SerialMessage {
	// Простой парсинг формата Meshtastic
	// Пример: "from !12345678: Hello" или "from !12345678 to !87654321: Hello"
	
	if !strings.Contains(line, "from !") {
		return nil
	}

	msg := &SerialMessage{
		Inbound: true,
	}

	// Удаляем префикс "from "
	parts := strings.SplitN(line, "from !", 2)
	if len(parts) < 2 {
		return nil
	}

	rest := parts[1]
	
	// Извлекаем FromNode (до пробела или ":")
	nodeParts := strings.SplitN(rest, ":", 2)
	if len(nodeParts) < 2 {
		return nil
	}

	fromParts := strings.Fields(nodeParts[0])
	if len(fromParts) == 0 {
		return nil
	}

	msg.FromNode = "!" + strings.TrimSpace(fromParts[0])

	// Извлекаем текст сообщения
	textParts := strings.SplitN(nodeParts[1], "to !", 2)
	if len(textParts) > 1 {
		// Есть "to" адресат
		toAndText := strings.SplitN(textParts[1], ":", 2)
		if len(toAndText) >= 2 {
			msg.ToNode = "!" + strings.TrimSpace(toAndText[0])
			msg.Text = strings.TrimSpace(toAndText[1])
		}
	} else {
		msg.Text = strings.TrimSpace(textParts[0])
		msg.ToNode = "!broadcast"
	}

	return msg
}

// SendMessage отправляет сообщение через COM-порт
func (c *MeshtasticSerialClient) SendMessage(ctx context.Context, toNode, text string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return fmt.Errorf("не подключен")
	}

	if c.conn == nil {
		return fmt.Errorf("соединение не установлено")
	}

	// Формируем команду для Meshtastic
	// Meshtastic принимает команды в формате:
	// sendtext <текст> --to <node_id>
	command := fmt.Sprintf("sendtext %s --to %s\n", text, toNode)

	_, err := c.conn.Write([]byte(command))
	if err != nil {
		return fmt.Errorf("ошибка отправки сообщения: %w", err)
	}

	log.Printf("Meshtastic Serial: отправлено сообщение на %s: %s", toNode, text)
	return nil
}

// GetMessages возвращает канал для получения сообщений
func (c *MeshtasticSerialClient) GetMessages() <-chan SerialMessage {
	return c.messageCh
}

// IsConnected проверяет статус подключения
func (c *MeshtasticSerialClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetPort возвращает имя COM-порта
func (c *MeshtasticSerialClient) GetPort() string {
	return c.port
}

// SetPort устанавливает COM-порт
func (c *MeshtasticSerialClient) SetPort(port string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.port = port
}

// ScanForDevices сканирует доступные COM-порты для поиска Meshtastic устройств
func ScanForDevices() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}

	found := make([]string, 0)

	for _, port := range ports {
		if isMeshtasticPort(port) {
			found = append(found, port)
		}
	}

	// Если не нашли Meshtastic, вернем все доступные порты
	if len(found) == 0 {
		return ports, nil
	}

	return found, nil
}

// isMeshtasticPort проверяет является ли COM-порт устройством Meshtastic
func isMeshtasticPort(port string) bool {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	conn, err := serial.Open(port, mode)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Устанавливаем таймаут
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Отправляем тестовую команду
	_, err = conn.Write([]byte("version\n"))
	if err != nil {
		return false
	}

	// Читаем ответ
	buffer := make([]byte, 1024)
	
	// Устанавливаем дедлайн для чтения
	readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
	defer readCancel()

	done := make(chan bool, 1)
	var n int
	var readErr error

	go func() {
		n, readErr = conn.Read(buffer)
		done <- true
	}()

	select {
	case <-readCtx.Done():
		return false
	case <-done:
		if readErr != nil {
			return false
		}
		response := string(buffer[:n])
		// Meshtastic обычно отвечает версией прошивки
		return strings.Contains(response, "Meshtastic") || 
		       strings.Contains(response, "firmware") ||
		       len(response) > 10
	}
}

// Binary протокол Meshtastic (для продвинутого использования)
const (
	MESHTASTIC_MAGIC1 = 0x94
	MESHTASTIC_MAGIC2 = 0xC3
)

// parseBinaryMessage парсит бинарное сообщение Meshtastic
func (c *MeshtasticSerialClient) parseBinaryMessage(data []byte) (*SerialMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("слишком короткие данные")
	}

	// Проверка magic bytes
	if data[0] != MESHTASTIC_MAGIC1 || data[1] != MESHTASTIC_MAGIC2 {
		return nil, fmt.Errorf("неверные magic bytes")
	}

	// Читаем длину payload
	length := binary.LittleEndian.Uint16(data[2:4])

	if len(data) < 4+int(length) {
		return nil, fmt.Errorf("неполные данные")
	}

	// Здесь должен быть парсинг protobuf сообщения Meshtastic
	// Для простоты возвращаем ошибку
	return nil, fmt.Errorf("бинарный парсинг не реализован")
}
