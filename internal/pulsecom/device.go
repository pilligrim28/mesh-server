package pulsecom

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// PortInfo — COM-порт в системе.
type PortInfo struct {
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	VID     string `json:"vid,omitempty"`
	PID     string `json:"pid,omitempty"`
	Arduino bool   `json:"arduino"`
}

// VID плат на ATmega32U4: SparkFun, Arduino, Arduino.org, Adafruit.
var arduinoVIDs = map[string]bool{"1B4F": true, "2341": true, "2A03": true, "239A": true}

// ListPorts возвращает порты, Arduino-подобные — первыми.
func ListPorts() []PortInfo {
	var out []PortInfo
	if det, err := enumerator.GetDetailedPortsList(); err == nil {
		for _, p := range det {
			pi := PortInfo{Name: p.Name, Desc: p.Product, VID: strings.ToUpper(p.VID), PID: strings.ToUpper(p.PID)}
			if p.IsUSB {
				pi.Arduino = arduinoVIDs[pi.VID]
				if pi.Desc == "" {
					pi.Desc = "USB " + pi.VID + ":" + pi.PID
				}
			}
			out = append(out, pi)
		}
	} else if names, err := serial.GetPortsList(); err == nil {
		for _, n := range names {
			out = append(out, PortInfo{Name: n})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Arduino && !out[j].Arduino })
	return out
}

// Port — открытое соединение (настоящий COM-порт или симулятор).
type Port interface {
	io.ReadWriteCloser
}

// Opener открывает порт по имени.
type Opener func(name string) (Port, error)

// OpenSerial открывает COM-порт. Pro Micro (ATmega32U4) работает по USB CDC:
// скорость не важна, но DTR должен быть поднят, иначе Serial на плате «не подключён».
func OpenSerial(name string) (Port, error) {
	p, err := serial.Open(name, &serial.Mode{BaudRate: 115200, InitialStatusBits: &serial.ModemOutputBits{DTR: true, RTS: true}})
	if err != nil {
		// некоторые драйверы/виртуальные порты не умеют модемные линии
		var err2 error
		if p, err2 = serial.Open(name, &serial.Mode{BaudRate: 115200}); err2 != nil {
			return nil, err
		}
	}
	p.SetDTR(true)
	p.SetRTS(true)
	return p, nil
}

// DevState — состояние подключения.
type DevState struct {
	Connected bool      `json:"connected"`
	Port      string    `json:"port"`
	Want      string    `json:"want"` // "" = автопоиск
	Firmware  string    `json:"firmware"`
	SensorOK  bool      `json:"sensorOk"`
	Status    *Msg      `json:"status,omitempty"`
	Last      *Msg      `json:"last,omitempty"`
	Error     string    `json:"error,omitempty"`
	Since     time.Time `json:"since"`
}

// Device ищет Arduino с MaxPulse, держит соединение и переподключается.
type Device struct {
	Open    Opener
	List    func() []PortInfo
	OnMsg   func(time.Time, Msg)
	OnState func(DevState)
	OnLog   func(string)

	mu   sync.Mutex
	st   DevState
	port Port
	kick chan struct{}
	wmu  sync.Mutex
}

func NewDevice(want string) *Device {
	return &Device{Open: OpenSerial, List: ListPorts, st: DevState{Want: want}, kick: make(chan struct{}, 1)}
}

func (d *Device) State() DevState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.st
}

func (d *Device) update(f func(*DevState)) {
	d.mu.Lock()
	f(&d.st)
	s := d.st
	d.mu.Unlock()
	if d.OnState != nil {
		d.OnState(s)
	}
}

func (d *Device) logf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	log.Print(msg)
	if d.OnLog != nil {
		d.OnLog(msg)
	}
}

// SetPort выбирает порт ("" — автопоиск) и переподключается.
func (d *Device) SetPort(name string) {
	d.update(func(s *DevState) { s.Want = name })
	d.mu.Lock()
	p := d.port
	d.mu.Unlock()
	if p != nil {
		p.Close()
	}
	select {
	case d.kick <- struct{}{}:
	default:
	}
}

// Send отправляет команду на Arduino.
func (d *Device) Send(cmd string) error {
	c, err := ValidCommand(cmd)
	if err != nil {
		return err
	}
	d.mu.Lock()
	p := d.port
	ok := d.st.Connected
	d.mu.Unlock()
	if p == nil || !ok {
		return errors.New("Arduino не подключена")
	}
	d.wmu.Lock()
	defer d.wmu.Unlock()
	_, err = p.Write([]byte(c + "\n"))
	return err
}

type lineOrErr struct {
	at   time.Time
	line string
	err  error
}

// noZero пропускает пустые чтения (таймауты драйвера), иначе bufio.Scanner падает.
type noZero struct{ r io.Reader }

func (z noZero) Read(b []byte) (int, error) {
	for {
		n, err := z.r.Read(b)
		if n > 0 || err != nil {
			return n, err
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readLines(p Port, ch chan<- lineOrErr) {
	sc := bufio.NewScanner(noZero{p})
	sc.Buffer(make([]byte, 256), 4096)
	for sc.Scan() {
		ch <- lineOrErr{at: time.Now(), line: sc.Text()}
	}
	err := sc.Err()
	if err == nil {
		err = io.EOF
	}
	ch <- lineOrErr{err: err}
}

// handshake шлёт PING и ждёт HELLO,MAXPULSE.
func handshake(p Port, ch <-chan lineOrErr, timeout time.Duration) (string, error) {
	deadline := time.After(timeout)
	resend := time.NewTicker(800 * time.Millisecond)
	defer resend.Stop()
	if _, err := p.Write([]byte("\nPING\n")); err != nil {
		return "", err
	}
	for {
		select {
		case l := <-ch:
			if l.err != nil {
				return "", l.err
			}
			if m, err := ParseLine(l.line); err == nil && m.Kind == KindHello {
				return m.Version, nil
			}
		case <-resend.C:
			p.Write([]byte("PING\n"))
		case <-deadline:
			return "", errors.New("нет ответа HELLO,MAXPULSE — это не скетч MaxPulse?")
		}
	}
}

// try открывает порт и проверяет, что там MaxPulse.
func (d *Device) try(name string) (Port, chan lineOrErr, string, error) {
	p, err := d.Open(name)
	if err != nil {
		return nil, nil, "", err
	}
	ch := make(chan lineOrErr, 256)
	go readLines(p, ch)
	ver, err := handshake(p, ch, 2500*time.Millisecond)
	if err != nil {
		p.Close()
		go func() { // дочитываем канал, чтобы горутина чтения завершилась
			for l := range ch {
				if l.err != nil {
					return
				}
			}
		}()
		return nil, nil, "", err
	}
	return p, ch, ver, nil
}

func (d *Device) wait(t time.Duration) {
	select {
	case <-d.kick:
	case <-time.After(t):
	}
}

// Run — бесконечный цикл поиска, чтения и переподключения.
func (d *Device) Run() {
	failed := map[string]time.Time{} // порты без MaxPulse не мучаем 15 с
	for {
		want := d.State().Want
		var cands []string
		if want != "" {
			cands = []string{want}
		} else {
			for _, p := range d.List() {
				if t, ok := failed[p.Name]; ok && time.Since(t) < 15*time.Second {
					continue
				}
				cands = append(cands, p.Name)
			}
		}
		var (
			port Port
			ch   chan lineOrErr
			ver  string
			name string
			last error
		)
		for _, c := range cands {
			p, lc, v, err := d.try(c)
			if err == nil {
				port, ch, ver, name = p, lc, v, c
				delete(failed, c)
				break
			}
			failed[c] = time.Now()
			last = fmt.Errorf("%s: %v", c, err)
		}
		if port == nil {
			msg := "Arduino не найдена — подключите Pro Micro по USB"
			if want != "" && last != nil {
				msg = last.Error()
			}
			d.update(func(s *DevState) { s.Connected = false; s.Port = ""; s.Error = msg })
			d.wait(2 * time.Second)
			continue
		}
		d.mu.Lock()
		d.port = port
		d.mu.Unlock()
		d.update(func(s *DevState) {
			*s = DevState{Want: s.Want, Connected: true, Port: name, Firmware: ver, SensorOK: true, Since: time.Now()}
		})
		d.logf("Подключено: %s, прошивка MaxPulse %s", name, ver)
		d.Send("STATUS")
		err := d.loop(ch)
		port.Close()
		d.mu.Lock()
		d.port = nil
		d.mu.Unlock()
		d.update(func(s *DevState) {
			s.Connected = false
			s.Error = "соединение потеряно: " + err.Error()
		})
		d.logf("Отключено от %s: %v", name, err)
		d.wait(time.Second)
	}
}

func (d *Device) loop(ch <-chan lineOrErr) error {
	const quiet = 15 * time.Second
	silent := time.NewTimer(quiet)
	defer silent.Stop()
	pinged := false
	for {
		select {
		case l := <-ch:
			if l.err != nil {
				return l.err
			}
			silent.Reset(quiet)
			pinged = false
			m, err := ParseLine(l.line)
			if err != nil {
				continue // мусор при подключении и т.п.
			}
			d.handle(l.at, m)
		case <-silent.C:
			// при STOP плата молчит — проверяем, жива ли она
			if pinged {
				return errors.New("плата не отвечает")
			}
			if err := d.Send("PING"); err != nil {
				return err
			}
			pinged = true
			silent.Reset(3 * time.Second)
		}
	}
}

func (d *Device) handle(at time.Time, m Msg) {
	switch m.Kind {
	case KindStatus:
		mm := m
		d.update(func(s *DevState) { s.Status = &mm })
	case KindData:
		mm := m
		d.mu.Lock()
		d.st.Last = &mm
		d.mu.Unlock()
	case KindErr:
		if m.Text == "SENSOR_NOT_FOUND" {
			d.update(func(s *DevState) { s.SensorOK = false })
		}
		d.logf("Arduino: ERR %s", m.Text)
	case KindOK:
		if m.Text == "SENSOR_FOUND" {
			d.update(func(s *DevState) { s.SensorOK = true })
		}
		d.logf("Arduino: OK %s", m.Text)
		if m.Text != "SENSOR_FOUND" {
			go d.Send("STATUS")
		}
	}
	if d.OnMsg != nil {
		d.OnMsg(at, m)
	}
}
