//go:build !(linux || windows || darwin)

package transport

import (
	"context"
	"time"
)

type BLEOptions struct {
	NameFilter  string
	Addresses   []string
	MaxGateways int
	ScanTimeout time.Duration
}

type BLE struct{ sink Sink }

func NewBLE(_ BLEOptions, sink Sink) *BLE { return &BLE{sink: sink} }

func (b *BLE) Run(ctx context.Context) {
	b.sink.SetGateway("bluetooth", "ble", "error", "", "BLE не поддерживается на этой ОС")
}
