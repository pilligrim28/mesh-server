package pulsecom

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SimPort эмулирует Arduino со скетчем MaxPulse (флаг -sim): удобно проверить
// приложение без железа. Палец «прикладывается» на 40 с из каждых 50.
type SimPort struct {
	mu      sync.Mutex
	out     chan []byte
	closed  chan struct{}
	once    sync.Once
	pending []byte
	inbuf   string

	running, raw bool
	rate, led    int
	t0           time.Time
}

func NewSimPort() *SimPort {
	s := &SimPort{out: make(chan []byte, 512), closed: make(chan struct{}), running: true, rate: 1000, led: 31, t0: time.Now()}
	go s.gen()
	return s
}

func OpenSim(string) (Port, error) { return NewSimPort(), nil }

func (s *SimPort) ms() uint32 { return uint32(time.Since(s.t0).Milliseconds()) }

func (s *SimPort) send(format string, a ...any) {
	select {
	case s.out <- []byte(fmt.Sprintf(format, a...) + "\n"):
	case <-s.closed:
	default: // переполнение — как у настоящего USB, строка теряется
	}
}

func (s *SimPort) gen() {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	var lastSum, nextBeat time.Time
	phase := 0.0
	for {
		select {
		case <-s.closed:
			return
		case now := <-tick.C:
			s.mu.Lock()
			running, raw, rate, led := s.running, s.raw, s.rate, s.led
			s.mu.Unlock()
			el := time.Since(s.t0).Seconds()
			finger := math.Mod(el, 50) < 40
			bpm := 68 + 8*math.Sin(el/20) + rand.Float64()*2
			phase += 0.01 * bpm / 60 * 2 * math.Pi
			k := float64(led) / 31
			var ir, red float64
			if finger {
				w := math.Sin(phase) + 0.35*math.Sin(2*phase+0.8)
				ir = k*(120000-900*w) + rand.Float64()*150
				red = k*(95000-700*w) + rand.Float64()*150
			} else {
				ir, red = 900+rand.Float64()*200, 700+rand.Float64()*200
			}
			if raw {
				s.send("R,%d,%d,%d", s.ms(), int(ir), int(red))
			}
			if !running {
				continue
			}
			if finger && now.After(nextBeat) {
				if !nextBeat.IsZero() {
					s.send("B,%d,%d", s.ms(), int(bpm))
				}
				nextBeat = now.Add(time.Duration(60 / bpm * float64(time.Second)))
			}
			if !finger {
				nextBeat = time.Time{}
			}
			if now.Sub(lastSum) >= time.Duration(rate)*time.Millisecond {
				lastSum = now
				fb, sp, f := 0, 0, 0
				if finger {
					f, fb, sp = 1, int(bpm), 96+rand.Intn(3)
				}
				s.send("D,%d,%d,%d,%d,%d,%d", s.ms(), f, fb, sp, int(ir), int(red))
			}
		}
	}
}

func (s *SimPort) Read(b []byte) (int, error) {
	if len(s.pending) == 0 {
		select {
		case p := <-s.out:
			s.pending = p
		case <-s.closed:
			return 0, io.EOF
		}
	}
	n := copy(b, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *SimPort) Write(b []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, io.ErrClosedPipe
	default:
	}
	s.mu.Lock()
	s.inbuf += string(b)
	var cmds []string
	for {
		i := strings.IndexByte(s.inbuf, '\n')
		if i < 0 {
			break
		}
		cmds = append(cmds, strings.TrimSpace(s.inbuf[:i]))
		s.inbuf = s.inbuf[i+1:]
	}
	s.mu.Unlock()
	for _, c := range cmds {
		s.command(c)
	}
	return len(b), nil
}

func (s *SimPort) command(c string) {
	if c == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case c == "PING":
		s.send("HELLO,MAXPULSE,1.0-sim")
		return
	case c == "STATUS":
		b := func(v bool) int {
			if v {
				return 1
			}
			return 0
		}
		s.send("S,%d,%d,%d,%d,%d", b(s.running), b(s.raw), s.rate, s.led, 312+rand.Intn(10))
		return
	case c == "START":
		s.running = true
	case c == "STOP":
		s.running = false
	case c == "RAW ON":
		s.raw = true
	case c == "RAW OFF":
		s.raw = false
	case strings.HasPrefix(c, "RATE "):
		v, _ := strconv.Atoi(c[5:])
		if v < 200 || v > 10000 {
			s.send("ERR,RATE 200..10000")
			return
		}
		s.rate = v
	case strings.HasPrefix(c, "LED "):
		v, err := strconv.Atoi(c[4:])
		if err != nil || v < 0 || v > 255 {
			s.send("ERR,LED 0..255")
			return
		}
		s.led = v
	default:
		s.send("ERR,UNKNOWN %s", c)
		return
	}
	s.send("OK,%s", c)
}

func (s *SimPort) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}
