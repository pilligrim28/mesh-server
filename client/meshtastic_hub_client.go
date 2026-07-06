package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "buf.build/gen/go/meshtastic/protobufs/protocolbuffers/go/meshtastic"
	"google.golang.org/protobuf/proto"
)

// MeshtasticHubClient двусторонний HTTP-мост к ESP32 с прошивкой Meshtastic.
type MeshtasticHubClient struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex

	configRequested bool
	seenPacketIDs   map[uint32]struct{}
}

// HubMessage входящее сообщение из mesh-сети через ESP32.
type HubMessage struct {
	FromNode string
	ToNode   string
	Text     string
	PacketID uint32
}

// NewMeshtasticHubClient создаёт клиент для ESP32-хаба.
func NewMeshtasticHubClient(baseURL string) *MeshtasticHubClient {
	url := normalizeMeshtasticURL(baseURL)
	return &MeshtasticHubClient{
		baseURL: url,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		seenPacketIDs: make(map[uint32]struct{}),
	}
}

func normalizeMeshtasticURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	return strings.TrimRight(baseURL, "/")
}

// SetBaseURL обновляет URL ESP32/Meshtastic устройства.
func (c *MeshtasticHubClient) SetBaseURL(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = normalizeMeshtasticURL(baseURL)
	c.configRequested = false
	c.seenPacketIDs = make(map[uint32]struct{})
}

// GetBaseURL возвращает текущий URL.
func (c *MeshtasticHubClient) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// IsAvailable проверяет доступность Meshtastic HTTP API.
func (c *MeshtasticHubClient) IsAvailable(ctx context.Context) bool {
	baseURL := c.GetBaseURL()
	if baseURL == "" {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/mesh", nil)
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

// EnsureConfig запрашивает конфигурацию и NodeDB у радио (первое подключение).
func (c *MeshtasticHubClient) EnsureConfig(ctx context.Context) error {
	c.mu.Lock()
	alreadyRequested := c.configRequested
	c.mu.Unlock()

	if alreadyRequested {
		return nil
	}

	toRadio := &pb.ToRadio{
		PayloadVariant: &pb.ToRadio_WantConfigId{
			WantConfigId: uint32(time.Now().Unix()),
		},
	}

	if err := c.sendToRadio(ctx, toRadio); err != nil {
		return err
	}

	c.mu.Lock()
	c.configRequested = true
	c.mu.Unlock()

	// Читаем ответ конфигурации, чтобы освободить очередь fromradio.
	_, _ = c.pollFromRadio(ctx, false)
	return nil
}

// PollMessages читает новые сообщения из mesh-сети через ESP32.
func (c *MeshtasticHubClient) PollMessages(ctx context.Context) ([]HubMessage, error) {
	if err := c.EnsureConfig(ctx); err != nil {
		return nil, err
	}

	fromRadios, err := c.pollFromRadio(ctx, true)
	if err != nil {
		return nil, err
	}

	messages := make([]HubMessage, 0)
	for _, fromRadio := range fromRadios {
		packet := fromRadio.GetPacket()
		if packet == nil {
			continue
		}

		data := packet.GetDecoded()
		if data == nil {
			continue
		}
		if data.GetPortnum() != pb.PortNum_TEXT_MESSAGE_APP {
			continue
		}

		text := string(data.GetPayload())
		if text == "" {
			continue
		}

		c.mu.Lock()
		if _, seen := c.seenPacketIDs[packet.GetId()]; seen {
			c.mu.Unlock()
			continue
		}
		c.seenPacketIDs[packet.GetId()] = struct{}{}
		c.mu.Unlock()

		toNode := formatNodeID(packet.GetTo())
		if packet.GetTo() == 0xFFFFFFFF {
			toNode = "broadcast"
		}

		messages = append(messages, HubMessage{
			FromNode: formatNodeID(packet.GetFrom()),
			ToNode:   toNode,
			Text:     text,
			PacketID: packet.GetId(),
		})
	}

	return messages, nil
}

// SendTextMessage отправляет текстовое сообщение в mesh через ESP32.
func (c *MeshtasticHubClient) SendTextMessage(ctx context.Context, toNode, text string) error {
	if err := c.EnsureConfig(ctx); err != nil {
		return err
	}

	toNum := parseNodeID(toNode)
	packet := &pb.MeshPacket{
		To:   toNum,
		From: 0,
		Id:   uint32(time.Now().UnixNano() & 0xFFFFFFFF),
		Decoded: &pb.Data{
			Portnum: pb.PortNum_TEXT_MESSAGE_APP,
			Payload: []byte(text),
		},
	}

	toRadio := &pb.ToRadio{
		PayloadVariant: &pb.ToRadio_Packet{
			Packet: packet,
		},
	}

	return c.sendToRadio(ctx, toRadio)
}

// SyncNodes возвращает узлы mesh-сети, видимые ESP32-хабом.
func (c *MeshtasticHubClient) SyncNodes(ctx context.Context) ([]MeshtasticNode, error) {
	baseURL := c.GetBaseURL()
	if baseURL == "" {
		return nil, fmt.Errorf("hub URL is not configured")
	}

	// Сначала пробуем JSON API прошивки Meshtastic.
	nodes, err := c.getJSONNodes(ctx)
	if err == nil && len(nodes) > 0 {
		return nodes, nil
	}

	// Fallback на /api/v1/nodes.
	return c.getV1Nodes(ctx)
}

func (c *MeshtasticHubClient) getJSONNodes(ctx context.Context) ([]MeshtasticNode, error) {
	baseURL := c.GetBaseURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/json/nodes", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("json/nodes returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Nodes []MeshtasticNode `json:"nodes"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Некоторые версии возвращают массив напрямую.
		var nodes []MeshtasticNode
		if err2 := json.Unmarshal(body, &nodes); err2 != nil {
			return nil, err
		}
		return nodes, nil
	}

	if len(payload.Nodes) > 0 {
		return payload.Nodes, nil
	}

	var nodes []MeshtasticNode
	if err := json.Unmarshal(body, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *MeshtasticHubClient) getV1Nodes(ctx context.Context) ([]MeshtasticNode, error) {
	baseURL := c.GetBaseURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/nodes", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api/v1/nodes returned status %d", resp.StatusCode)
	}

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

func (c *MeshtasticHubClient) pollFromRadio(ctx context.Context, all bool) ([]*pb.FromRadio, error) {
	baseURL := c.GetBaseURL()
	url := baseURL + "/api/v1/fromradio"
	if all {
		url += "?all=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fromradio returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}

	return decodeFromRadioStream(body), nil
}

func (c *MeshtasticHubClient) sendToRadio(ctx context.Context, toRadio *pb.ToRadio) error {
	baseURL := c.GetBaseURL()
	payload, err := proto.Marshal(toRadio)
	if err != nil {
		return fmt.Errorf("failed to marshal ToRadio: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/api/v1/toradio", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("toradio returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func decodeFromRadioStream(data []byte) []*pb.FromRadio {
	messages := make([]*pb.FromRadio, 0)
	offset := 0

	for offset < len(data) {
		fromRadio := &pb.FromRadio{}
		if err := proto.Unmarshal(data[offset:], fromRadio); err != nil {
			break
		}

		encoded, err := proto.Marshal(fromRadio)
		if err != nil || len(encoded) == 0 {
			break
		}

		messages = append(messages, fromRadio)
		offset += len(encoded)
	}

	return messages
}

func formatNodeID(num uint32) string {
	return fmt.Sprintf("!%08x", num)
}

func parseNodeID(nodeID string) uint32 {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || nodeID == "broadcast" || nodeID == "^all" {
		return 0xFFFFFFFF
	}
	if strings.HasPrefix(nodeID, "!") {
		nodeID = nodeID[1:]
	}
	var result uint32
	fmt.Sscanf(nodeID, "%x", &result)
	return result
}
