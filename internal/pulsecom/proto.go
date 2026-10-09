// Package pulsecom — приложение для чтения MAX30102 через Arduino Pro Micro (скетч MaxPulse) по COM-порту.
package pulsecom

import (
	"fmt"
	"strconv"
	"strings"
)

// Типы сообщений протокола MaxPulse v1.
const (
	KindHello  = "hello"
	KindData   = "d"
	KindBeat   = "b"
	KindRaw    = "r"
	KindStatus = "s"
	KindOK     = "ok"
	KindErr    = "err"
)

// Msg — одна разобранная строка от Arduino.
type Msg struct {
	Kind string `json:"t"`
	Ms   uint32 `json:"ms,omitempty"` // millis() устройства

	Version string `json:"ver,omitempty"` // HELLO

	Finger bool   `json:"finger,omitempty"` // D
	BPM    int    `json:"bpm,omitempty"`    // D, B (0 = нет значения)
	SpO2   int    `json:"spo2,omitempty"`   // D
	IR     uint32 `json:"ir,omitempty"`     // D, R
	Red    uint32 `json:"red,omitempty"`    // D, R

	Running bool    `json:"running,omitempty"` // S
	RawOn   bool    `json:"raw,omitempty"`     // S
	RateMs  int     `json:"rate,omitempty"`    // S
	LED     int     `json:"led,omitempty"`     // S
	TempC   float64 `json:"temp,omitempty"`    // S

	Text string `json:"text,omitempty"` // OK / ERR
}

// ParseLine разбирает одну строку протокола (без \r\n).
func ParseLine(line string) (Msg, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Msg{}, fmt.Errorf("пустая строка")
	}
	f := strings.Split(line, ",")
	need := func(n int) error {
		if len(f) != n {
			return fmt.Errorf("%s: ожидалось %d полей, получено %d", f[0], n, len(f))
		}
		return nil
	}
	var perr error
	u := func(s string) uint32 {
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil && perr == nil {
			perr = fmt.Errorf("%s: плохое число %q", f[0], s)
		}
		return uint32(v)
	}
	i := func(s string) int {
		v, err := strconv.Atoi(s)
		if err != nil && perr == nil {
			perr = fmt.Errorf("%s: плохое число %q", f[0], s)
		}
		return v
	}
	var m Msg
	switch f[0] {
	case "HELLO":
		if len(f) < 3 || f[1] != "MAXPULSE" {
			return m, fmt.Errorf("HELLO: чужое устройство")
		}
		m = Msg{Kind: KindHello, Version: f[2]}
	case "D":
		if err := need(7); err != nil {
			return m, err
		}
		m = Msg{Kind: KindData, Ms: u(f[1]), Finger: i(f[2]) != 0, BPM: i(f[3]), SpO2: i(f[4]), IR: u(f[5]), Red: u(f[6])}
	case "B":
		if err := need(3); err != nil {
			return m, err
		}
		m = Msg{Kind: KindBeat, Ms: u(f[1]), BPM: i(f[2])}
	case "R":
		if err := need(4); err != nil {
			return m, err
		}
		m = Msg{Kind: KindRaw, Ms: u(f[1]), IR: u(f[2]), Red: u(f[3])}
	case "S":
		if err := need(6); err != nil {
			return m, err
		}
		m = Msg{Kind: KindStatus, Running: i(f[1]) != 0, RawOn: i(f[2]) != 0, RateMs: i(f[3]), LED: i(f[4]), TempC: float64(i(f[5])) / 10}
	case "OK":
		m = Msg{Kind: KindOK, Text: strings.Join(f[1:], ",")}
	case "ERR":
		m = Msg{Kind: KindErr, Text: strings.Join(f[1:], ",")}
	default:
		return m, fmt.Errorf("неизвестная строка %q", line)
	}
	return m, perr
}

// ValidCommand проверяет команду перед отправкой на Arduino.
func ValidCommand(c string) (string, error) {
	c = strings.ToUpper(strings.TrimSpace(c))
	switch c {
	case "PING", "STATUS", "START", "STOP", "RAW ON", "RAW OFF":
		return c, nil
	}
	if p := strings.Fields(c); len(p) == 2 {
		v, err := strconv.Atoi(p[1])
		if err == nil {
			switch {
			case p[0] == "RATE" && v >= 200 && v <= 10000,
				p[0] == "LED" && v >= 0 && v <= 255:
				return p[0] + " " + strconv.Itoa(v), nil
			}
		}
	}
	return "", fmt.Errorf("недопустимая команда %q", c)
}
