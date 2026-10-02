package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"time"

	"go.bug.st/serial"

	"mesh-server/internal/mesh"
)

// Serial — шлюз Meshtastic по USB. Протокол: 0x94 0xC3 <len hi> <len lo> <protobuf>.
type Serial struct {
	port string
	sink Sink
}

func NewSerial(port string, sink Sink) *Serial { return &Serial{port: port, sink: sink} }

func (s *Serial) Run(ctx context.Context) {
	for attempt := 0; ctx.Err() == nil; attempt++ {
		s.sink.SetGateway(s.port, "serial", "connecting", "", "")
		err := s.session(ctx)
		if ctx.Err() != nil {
			return
		}
		s.sink.SetGateway(s.port, "serial", "disconnected", "", errText(err))
		sleep(ctx, backoff(attempt))
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func frame(payload []byte) []byte {
	return append([]byte{0x94, 0xC3, byte(len(payload) >> 8), byte(len(payload))}, payload...)
}

func (s *Serial) session(ctx context.Context) error {
	p, err := serial.Open(s.port, &serial.Mode{BaudRate: 115200})
	if err != nil {
		return fmt.Errorf("открыть %s: %w", s.port, err)
	}
	defer p.Close()
	go func() { <-ctx.Done(); p.Close() }()

	wake := make([]byte, 32)
	for i := range wake {
		wake[i] = 0xC3 // пробуждение API, как в официальном meshtastic-python
	}
	if _, err := p.Write(append(wake, frame(mesh.WantConfig(rand.Uint32N(1<<31)+1))...)); err != nil {
		return err
	}
	s.sink.SetGateway(s.port, "serial", "connected", "", "")

	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := p.Write(frame(mesh.Heartbeat())); err != nil {
					return
				}
			}
		}
	}()

	r := bufio.NewReader(p)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b != 0x94 {
			continue // текстовый отладочный лог прошивки
		}
		if b, err = r.ReadByte(); err != nil {
			return err
		} else if b != 0xC3 {
			continue
		}
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(r, hdr); err != nil {
			return err
		}
		n := int(hdr[0])<<8 | int(hdr[1])
		if n == 0 || n > 512 {
			continue
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("порт закрыт")
			}
			return err
		}
		s.sink.HandleFrame(s.port, payload)
	}
}
