//go:build linux || windows || darwin

package transport

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"

	"mesh-server/internal/mesh"
)

// BLEOptions — параметры автоподключения к шлюзам Meshtastic по Bluetooth.
type BLEOptions struct {
	NameFilter  string
	Addresses   []string
	MaxGateways int
	ScanTimeout time.Duration
}

// BLE находит узлы Meshtastic по UUID сервиса, сам подключается и переподключается.
type BLE struct {
	opt     BLEOptions
	sink    Sink
	adapter *bluetooth.Adapter

	mu        sync.Mutex
	connected map[string]chan struct{} // адрес -> канал «соединение потеряно»
	failures  map[string]time.Time
}

func NewBLE(opt BLEOptions, sink Sink) *BLE {
	if opt.MaxGateways < 1 {
		opt.MaxGateways = 1
	}
	if opt.ScanTimeout <= 0 {
		opt.ScanTimeout = 10 * time.Second
	}
	return &BLE{opt: opt, sink: sink, adapter: bluetooth.DefaultAdapter,
		connected: map[string]chan struct{}{}, failures: map[string]time.Time{}}
}

var (
	uuidService   = mustUUID(mesh.ServiceUUID)
	uuidToRadio   = mustUUID(mesh.ToRadioUUID)
	uuidFromRadio = mustUUID(mesh.FromRadioUUID)
	uuidFromNum   = mustUUID(mesh.FromNumUUID)
)

func mustUUID(s string) bluetooth.UUID {
	u, err := bluetooth.ParseUUID(s)
	if err != nil {
		panic(err)
	}
	return u
}

func (b *BLE) Run(ctx context.Context) {
	for attempt := 0; ctx.Err() == nil; attempt++ {
		if err := b.adapter.Enable(); err != nil {
			b.sink.SetGateway("bluetooth", "ble", "error", "", "Bluetooth-адаптер недоступен: "+err.Error())
			sleep(ctx, backoff(attempt))
			continue
		}
		break
	}
	b.adapter.SetConnectHandler(func(d bluetooth.Device, connected bool) {
		if connected {
			return
		}
		b.mu.Lock()
		if ch, ok := b.connected[d.Address.String()]; ok {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
		b.mu.Unlock()
	})
	log.Printf("BLE: автопоиск шлюзов Meshtastic (до %d одновременно)", b.opt.MaxGateways)

	for ctx.Err() == nil {
		if b.count() >= b.opt.MaxGateways {
			sleep(ctx, 3*time.Second)
			continue
		}
		cand, err := b.scan(ctx)
		if err != nil {
			b.sink.SetGateway("bluetooth", "ble", "error", "", "сканирование: "+err.Error())
			sleep(ctx, 5*time.Second)
			continue
		}
		if cand == nil {
			sleep(ctx, 2*time.Second)
			continue
		}
		b.connect(ctx, *cand)
	}
}

type candidate struct {
	addr bluetooth.Address
	name string
	rssi int16
}

func (b *BLE) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.connected)
}

func (b *BLE) eligible(addr, name string, hasService bool) bool {
	b.mu.Lock()
	_, busy := b.connected[addr]
	failedAt, failed := b.failures[addr]
	b.mu.Unlock()
	if busy || (failed && time.Since(failedAt) < 20*time.Second) {
		return false
	}
	if len(b.opt.Addresses) > 0 {
		for _, a := range b.opt.Addresses {
			if strings.EqualFold(a, addr) {
				return true
			}
		}
		return false
	}
	if b.opt.NameFilter != "" {
		return strings.Contains(strings.ToLower(name), strings.ToLower(b.opt.NameFilter))
	}
	return hasService
}

// scan ищет ближайший (лучший RSSI) подходящий узел.
func (b *BLE) scan(ctx context.Context) (*candidate, error) {
	b.sink.SetGateway("bluetooth", "ble", "scanning", "", "")
	var (
		mu   sync.Mutex
		best *candidate
	)
	stop := time.AfterFunc(b.opt.ScanTimeout, func() { b.adapter.StopScan() })
	defer stop.Stop()
	scanDone := make(chan struct{})
	defer close(scanDone)
	go func() {
		select {
		case <-ctx.Done():
			b.adapter.StopScan()
		case <-scanDone:
		}
	}()
	err := b.adapter.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
		addr := r.Address.String()
		if !b.eligible(addr, r.LocalName(), r.HasServiceUUID(uuidService)) {
			return
		}
		mu.Lock()
		if best == nil || r.RSSI > best.rssi {
			best = &candidate{addr: r.Address, name: r.LocalName(), rssi: r.RSSI}
		}
		mu.Unlock()
		// ждём ещё пару секунд, чтобы выбрать ближайший, но не всё время сканирования
		stop.Reset(2 * time.Second)
	})
	if err != nil && ctx.Err() == nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	if best != nil {
		b.sink.SetGateway("bluetooth", "ble", "found", "", fmt.Sprintf("%s %s RSSI %d", best.name, best.addr.String(), best.rssi))
	}
	return best, nil
}

func (b *BLE) connect(ctx context.Context, c candidate) {
	id := c.addr.String()
	lost := make(chan struct{})
	b.mu.Lock()
	b.connected[id] = lost
	b.mu.Unlock()

	b.sink.SetGateway(id, "ble", "connecting", c.name, "")
	dev, err := b.adapter.Connect(c.addr, bluetooth.ConnectionParams{})
	if err != nil {
		b.fail(id, c.name, fmt.Errorf("подключение: %w", err))
		return
	}
	go func() {
		err := b.session(ctx, dev, id, c.name, lost)
		dev.Disconnect()
		b.fail(id, c.name, err)
	}()
}

func (b *BLE) fail(id, name string, err error) {
	b.mu.Lock()
	delete(b.connected, id)
	b.failures[id] = time.Now()
	b.mu.Unlock()
	msg := ""
	if err != nil && !errors.Is(err, context.Canceled) {
		msg = err.Error()
	}
	b.sink.SetGateway(id, "ble", "disconnected", name, msg)
}

func (b *BLE) session(ctx context.Context, dev bluetooth.Device, id, name string, lost <-chan struct{}) error {
	svcs, err := dev.DiscoverServices([]bluetooth.UUID{uuidService})
	if err != nil || len(svcs) == 0 {
		return fmt.Errorf("сервис Meshtastic не найден: %v", err)
	}
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{uuidToRadio, uuidFromRadio, uuidFromNum})
	if err != nil {
		return fmt.Errorf("характеристики: %w", err)
	}
	var toRadio, fromRadio, fromNum *bluetooth.DeviceCharacteristic
	for i := range chars {
		switch chars[i].UUID() {
		case uuidToRadio:
			toRadio = &chars[i]
		case uuidFromRadio:
			fromRadio = &chars[i]
		case uuidFromNum:
			fromNum = &chars[i]
		}
	}
	if toRadio == nil || fromRadio == nil {
		return errors.New("нет характеристик ToRadio/FromRadio (старая прошивка?)")
	}

	wake := make(chan struct{}, 1)
	if fromNum != nil {
		if err := fromNum.EnableNotifications(func([]byte) {
			select {
			case wake <- struct{}{}:
			default:
			}
		}); err != nil {
			log.Printf("BLE %s: уведомления FromNum недоступны, работаю опросом: %v", id, err)
		}
	}

	if _, err := toRadio.Write(mesh.WantConfig(rand.Uint32N(1<<31) + 1)); err != nil {
		return fmt.Errorf("запрос конфигурации: %w (если включён PIN — см. README, раздел «Сопряжение»)", err)
	}
	b.sink.SetGateway(id, "ble", "connected", name, "")
	b.sink.DropGateway("bluetooth") // служебный статус «поиск» больше не нужен

	buf := make([]byte, 1024)
	drain := func() error {
		for i := 0; i < 1000; i++ { // конфигурация шлюза — до нескольких сотен сообщений
			n, err := fromRadio.Read(buf)
			if err != nil {
				return fmt.Errorf("чтение FromRadio: %w", err)
			}
			if n == 0 {
				return nil
			}
			b.sink.HandleFrame(id, append([]byte(nil), buf[:n]...))
		}
		return nil
	}
	if err := drain(); err != nil {
		return err
	}

	poll := time.NewTicker(5 * time.Second) // страховка, если уведомление потерялось
	heartbeat := time.NewTicker(60 * time.Second)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-lost:
			return errors.New("соединение потеряно")
		case <-wake:
		case <-poll.C:
		case <-heartbeat.C:
			if _, err := toRadio.Write(mesh.Heartbeat()); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}
			continue
		}
		if err := drain(); err != nil {
			return err
		}
	}
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<min(attempt, 5)) * time.Second
	return min(d, 30*time.Second)
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
