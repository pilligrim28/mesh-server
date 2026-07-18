package meshtastic

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// MDNSServer объявляет сервис через mDNS/DNS-SD
// чтобы приложение Meshtastic нашло сервер в локальной сети
type MDNSServer struct {
	serviceName string
	domain      string
	hostname    string
	port        int
	ip          net.IP
	isRunning   bool
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewMDNSServer создаёт новый mDNS сервер
func NewMDNSServer(hostname string, port int) *MDNSServer {
	return &MDNSServer{
		serviceName: "Meshtastic",
		domain:      "_meshtastic._tcp",
		hostname:    hostname,
		port:        port,
	}
}

// Start запускает mDNS объявление
func (m *MDNSServer) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return nil
	}

	// Получаем IP адрес
	m.ip = m.getLocalIP()
	if m.ip == nil {
		log.Println("MDNS: no suitable network interface found")
		return fmt.Errorf("no suitable network interface")
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.isRunning = true

	// Запускаем обработчик mDNS запросов
	go m.listenLoop()

	// Периодически отправляем announcement
	go m.advertiseLoop()

	log.Printf("MDNS: started, service='%s.%s', host='%s', ip=%s, port=%d",
		m.serviceName, m.domain, m.hostname, m.ip, m.port)

	return nil
}

// Stop останавливает mDNS
func (m *MDNSServer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.isRunning = false
	if m.cancel != nil {
		m.cancel()
	}
	log.Println("MDNS: stopped")
}

// IsRunning возвращает статус
func (m *MDNSServer) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetInfo возвращает информацию
func (m *MDNSServer) GetInfo() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ipStr := ""
	if m.ip != nil {
		ipStr = m.ip.String()
	}
	return map[string]interface{}{
		"running":  m.isRunning,
		"service":  m.serviceName + "." + m.domain,
		"hostname": m.hostname,
		"ip":       ipStr,
		"port":     m.port,
	}
}

// getLocalIP получает локальный IP адрес
func (m *MDNSServer) getLocalIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.To4()
			}
		}
	}
	return nil
}

// listenLoop слушает mDNS запросы и отвечает на них
func (m *MDNSServer) listenLoop() {
	// mDNS multicast адрес и порт
	addr, err := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
	if err != nil {
		log.Printf("MDNS: resolve error: %v", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Printf("MDNS: listen error: %v", err)
		return
	}
	defer conn.Close()

	conn.SetReadBuffer(65536)

	log.Println("MDNS: listening for queries on 224.0.0.251:5353")

	// Set deadline so we can check context cancellation periodically
	buf := make([]byte, 4096)
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		m.handleQuery(buf[:n], remoteAddr, conn)
	}
}

// handleQuery обрабатывает входящий mDNS запрос
func (m *MDNSServer) handleQuery(data []byte, remoteAddr *net.UDPAddr, conn *net.UDPConn) {
	if len(data) < 12 {
		return
	}

	// Проверяем что это запрос (QR=0, OPCODE=0)
	flags := binary.BigEndian.Uint16(data[2:4])
	if flags&0x8000 != 0 {
		return // это ответ, не запрос
	}

	// Извлекаем вопросы
	questionCount := binary.BigEndian.Uint16(data[4:6])
	if questionCount == 0 {
		return
	}

	offset := 12
	for i := 0; i < int(questionCount) && offset < len(data); i++ {
		name, newOffset, ok := m.parseDNSName(data, offset)
		if !ok {
			break
		}
		offset = newOffset

		if offset+4 > len(data) {
			break
		}

		qtype := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 4 // type + class

		// Проверяем что это PTR или ANY запрос для нашего сервиса
		if qtype == 12 || qtype == 255 { // PTR or ANY
			if m.matchesService(name) {
				log.Printf("MDNS: query for '%s' from %s", name, remoteAddr)
				m.sendResponse(conn, remoteAddr)
			}
		}
	}
}

// matchesService проверяет соответствует ли имя нашему сервису
func (m *MDNSServer) matchesService(name string) bool {
	name = strings.ToLower(name)
	suffix := strings.ToLower(m.domain)

	// Проверяем _meshtastic._tcp.local или _meshtastic._tcp
	if strings.Contains(name, "_meshtastic") && strings.Contains(name, "_tcp") {
		return true
	}
	if strings.Contains(name, suffix) {
		return true
	}
	// Проверяем общий запрос
	if name == "local" || name == "" {
		return true
	}
	return false
}

// sendResponse отправляет mDNS ответ
func (m *MDNSServer) sendResponse(conn *net.UDPConn, remoteAddr *net.UDPAddr) {
	resp := m.buildDNSResponse()

	// Отправляем unicast ответ
	_, err := conn.WriteToUDP(resp, remoteAddr)
	if err != nil {
		log.Printf("MDNS: response send error: %v", err)
	} else {
		log.Printf("MDNS: response sent to %s", remoteAddr)
	}
}

// buildDNSResponse создаёт DNS ответ
func (m *MDNSServer) buildDNSResponse() []byte {
	buf := make([]byte, 4096)
	offset := 0

	// DNS Header
	// ID
	binary.BigEndian.PutUint16(buf[offset:], 0)
	offset += 2
	// Flags: response, authoritative
	binary.BigEndian.PutUint16(buf[offset:], 0x8400)
	offset += 2
	// Questions: 0, Answers: 3, Authority: 0, Additional: 1
	binary.BigEndian.PutUint16(buf[offset:], 0) // questions
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], 3) // answers
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], 0) // authority
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], 1) // additional
	offset += 2

	// Answer 1: PTR记录 — сервис → хост
	offset = m.writePTRRecord(buf, offset, "_meshtastic._tcp.local", m.hostname+".local")

	// Answer 2: SRV记录 — хост → порт
	offset = m.writeSRVRecord(buf, offset, m.hostname+".local", m.port)

	// Answer 3: TXT记录 — свойства
	offset = m.writeTXTRecord(buf, offset, m.hostname+".local", map[string]string{
		"path":       "/",
		"v":          "2.5.0",
		"long_name":  "СтражСети",
		"short_name": "СС",
		"hw":         "SERVER",
		"firmware":   "2.5.0",
	})

	// Additional: A запись — hostname → IP
	offset = m.writeARecord(buf, offset, m.hostname+".local", m.ip)

	return buf[:offset]
}

// writePTRRecord записывает PTR запись
func (m *MDNSServer) writePTRRecord(buf []byte, offset int, service string, target string) int {
	// Name
	offset = m.writeDNSName(buf, offset, service)
	// Type: PTR (12)
	binary.BigEndian.PutUint16(buf[offset:], 12)
	offset += 2
	// Class: IN (1), cache-flush bit
	binary.BigEndian.PutUint16(buf[offset:], 0x8001)
	offset += 2
	// TTL: 120 seconds
	binary.BigEndian.PutUint32(buf[offset:], 120)
	offset += 4
	// Data length (placeholder)
	dataLenOffset := offset
	offset += 2
	// RDATA: domain name
	dataStart := offset
	offset = m.writeDNSName(buf, offset, target)
	// Patch data length
	binary.BigEndian.PutUint16(buf[dataLenOffset:], uint16(offset-dataStart))
	return offset
}

// writeSRVRecord записывает SRV запись
func (m *MDNSServer) writeSRVRecord(buf []byte, offset int, target string, port int) int {
	offset = m.writeDNSName(buf, offset, target)
	// Type: SRV (33)
	binary.BigEndian.PutUint16(buf[offset:], 33)
	offset += 2
	// Class: IN (1), cache-flush
	binary.BigEndian.PutUint16(buf[offset:], 0x8001)
	offset += 2
	// TTL: 120
	binary.BigEndian.PutUint32(buf[offset:], 120)
	offset += 4
	// Data length
	binary.BigEndian.PutUint16(buf[offset:], 8)
	offset += 2
	// Priority: 0
	binary.BigEndian.PutUint16(buf[offset:], 0)
	offset += 2
	// Weight: 0
	binary.BigEndian.PutUint16(buf[offset:], 0)
	offset += 2
	// Port
	binary.BigEndian.PutUint16(buf[offset:], uint16(port))
	offset += 2
	// Target
	offset = m.writeDNSName(buf, offset, target)
	return offset
}

// writeTXTRecord записывает TXT запись
func (m *MDNSServer) writeTXTRecord(buf []byte, offset int, name string, entries map[string]string) int {
	offset = m.writeDNSName(buf, offset, name)
	// Type: TXT (16)
	binary.BigEndian.PutUint16(buf[offset:], 16)
	offset += 2
	// Class: IN (1), cache-flush
	binary.BigEndian.PutUint16(buf[offset:], 0x8001)
	offset += 2
	// TTL: 120
	binary.BigEndian.PutUint32(buf[offset:], 120)
	offset += 4

	// Data (placeholder)
	dataLenOffset := offset
	offset += 2
	dataStart := offset

	for key, value := range entries {
		entry := key + "=" + value
		buf[offset] = byte(len(entry))
		offset++
		copy(buf[offset:], entry)
		offset += len(entry)
	}

	binary.BigEndian.PutUint16(buf[dataLenOffset:], uint16(offset-dataStart))
	return offset
}

// writeARecord записывает A запись
func (m *MDNSServer) writeARecord(buf []byte, offset int, name string, ip net.IP) int {
	offset = m.writeDNSName(buf, offset, name)
	// Type: A (1)
	binary.BigEndian.PutUint16(buf[offset:], 1)
	offset += 2
	// Class: IN (1), cache-flush
	binary.BigEndian.PutUint16(buf[offset:], 0x8001)
	offset += 2
	// TTL: 120
	binary.BigEndian.PutUint32(buf[offset:], 120)
	offset += 4
	// Data length: 4
	binary.BigEndian.PutUint16(buf[offset:], 4)
	offset += 2
	// IP address
	copy(buf[offset:], ip.To4())
	offset += 4
	return offset
}

// writeDNSName записывает DNS имя в формате labels
func (m *MDNSServer) writeDNSName(buf []byte, offset int, name string) int {
	// Добавляем точку в конец если нет
	if len(name) > 0 && name[len(name)-1] != '.' {
		name += "."
	}

	for _, label := range strings.Split(name, ".") {
		if label == "" {
			continue
		}
		buf[offset] = byte(len(label))
		offset++
		copy(buf[offset:], label)
		offset += len(label)
	}
	buf[offset] = 0 // terminator
	offset++
	return offset
}

// parseDNSName парсит DNS имя из буфера
func (m *MDNSServer) parseDNSName(data []byte, offset int) (string, int, bool) {
	if offset >= len(data) {
		return "", 0, false
	}

	var parts []string
	maxJumps := 10

	for i := 0; i < maxJumps; i++ {
		length := int(data[offset])
		offset++

		if length == 0 {
			break
		}

		// Compression pointer
		if length&0xC0 == 0xC0 {
			if offset >= len(data) {
				return "", 0, false
			}
			pointer := int(binary.BigEndian.Uint16(data[offset-1:offset+1])) & 0x3FFF
			offset++
			if pointer >= len(data) {
				return "", 0, false
			}
			suffix, _, ok := m.parseDNSName(data, pointer)
			if ok {
				parts = append(parts, suffix)
			}
			break
		}

		if offset+length > len(data) {
			return "", 0, false
		}

		parts = append(parts, string(data[offset:offset+length]))
		offset += length
	}

	return strings.Join(parts, "."), offset, true
}

// advertiseLoop периодически отправляет mDNS announcements
func (m *MDNSServer) advertiseLoop() {
	// Первый announcement сразу
	m.sendAnnouncement()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.sendAnnouncement()
		}
	}
}

// sendAnnouncement отправляет mDNS announcement на multicast
func (m *MDNSServer) sendAnnouncement() {
	addr, err := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	resp := m.buildDNSResponse()

	_, err = conn.Write(resp)
	if err != nil {
		log.Printf("MDNS: announcement error: %v", err)
	} else {
		log.Printf("MDNS: announcement sent")
	}
}
