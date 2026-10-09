package pulsecom

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const tsLayout = "2006-01-02 15:04:05.000"

// SessionInfo — описание текущей или сохранённой сессии записи.
type SessionInfo struct {
	Name    string    `json:"name"`  // базовое имя без .csv
	Label   string    `json:"label"` // метка (имя человека и т.п.)
	Started time.Time `json:"started"`
	Rows    int       `json:"rows"`
	Beats   int       `json:"beats"`
	Raw     int       `json:"raw"`
	Size    int64     `json:"size"`
	Files   []string  `json:"files"`
	Active  bool      `json:"active"`
}

type csvFile struct {
	f *os.File
	w *bufio.Writer
}

func newCSV(path, header string) (*csvFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(f, 64<<10)
	w.WriteString("\uFEFF" + header + "\r\n") // BOM — чтобы Excel понял UTF-8
	return &csvFile{f, w}, nil
}

func (c *csvFile) line(fields ...string) { c.w.WriteString(strings.Join(fields, ";") + "\r\n") }
func (c *csvFile) flush()                { c.w.Flush() }
func (c *csvFile) close() error          { c.w.Flush(); return c.f.Close() }

// Recorder пишет каждую сессию в отдельные CSV-файлы:
//
//	<name>.csv       — сводка раз в RATE мс (пульс, SpO2, IR, Red)
//	<name>_beats.csv — каждый обнаруженный удар
//	<name>_raw.csv   — сырые отсчёты 100 Гц (только если включён RAW)
type Recorder struct {
	dir string
	mu  sync.Mutex
	cur *SessionInfo
	d   *csvFile
	b   *csvFile
	r   *csvFile

	lastBeatMs uint32
}

func NewRecorder(dir string) (*Recorder, error) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rec := &Recorder{dir: dir}
	go func() { // сбрасываем буферы на диск раз в секунду
		for range time.Tick(time.Second) {
			rec.mu.Lock()
			for _, c := range []*csvFile{rec.d, rec.b, rec.r} {
				if c != nil {
					c.flush()
				}
			}
			rec.mu.Unlock()
		}
	}()
	return rec, nil
}

func (r *Recorder) Dir() string { return r.dir }

var badLabel = regexp.MustCompile(`[^\p{L}\p{N}_\-]+`)

func sanitizeLabel(s string) string {
	s = strings.Map(func(c rune) rune {
		if unicode.IsSpace(c) {
			return '_'
		}
		return c
	}, strings.TrimSpace(s))
	s = strings.Trim(badLabel.ReplaceAllString(s, ""), "_-")
	if rs := []rune(s); len(rs) > 40 {
		s = string(rs[:40])
	}
	return s
}

// Start закрывает текущую сессию (если есть) и начинает новую.
func (r *Recorder) Start(label string) (SessionInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
	now := time.Now()
	label = sanitizeLabel(label)
	name := "session_" + now.Format("20060102_150405")
	if label != "" {
		name += "_" + label
	}
	base := name
	for n := 2; ; n++ { // две сессии в одну секунду
		if _, err := os.Stat(filepath.Join(r.dir, name+".csv")); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s_%d", base, n)
	}
	d, err := newCSV(filepath.Join(r.dir, name+".csv"), "время;мс_устройства;палец;пульс_уд_мин;spo2_%;ir;red")
	if err != nil {
		return SessionInfo{}, err
	}
	b, err := newCSV(filepath.Join(r.dir, name+"_beats.csv"), "время;мс_устройства;пульс_уд_мин;интервал_мс")
	if err != nil {
		d.close()
		return SessionInfo{}, err
	}
	r.d, r.b = d, b
	r.cur = &SessionInfo{Name: name, Label: label, Started: now, Active: true}
	return *r.cur, nil
}

func (r *Recorder) stopLocked() {
	for _, c := range []*csvFile{r.d, r.b, r.r} {
		if c != nil {
			c.close()
		}
	}
	r.d, r.b, r.r, r.cur = nil, nil, nil, nil
	r.lastBeatMs = 0
}

// Stop завершает запись.
func (r *Recorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
}

// Current возвращает текущую сессию или nil.
func (r *Recorder) Current() *SessionInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == nil {
		return nil
	}
	c := *r.cur
	return &c
}

func opt(v int) string {
	if v <= 0 {
		return ""
	}
	return strconv.Itoa(v)
}

// Write сохраняет сообщение в файлы текущей сессии.
func (r *Recorder) Write(at time.Time, m Msg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == nil {
		return
	}
	ts := at.Format(tsLayout)
	ms := strconv.FormatUint(uint64(m.Ms), 10)
	switch m.Kind {
	case KindData:
		fg := "0"
		if m.Finger {
			fg = "1"
		}
		r.d.line(ts, ms, fg, opt(m.BPM), opt(m.SpO2), strconv.FormatUint(uint64(m.IR), 10), strconv.FormatUint(uint64(m.Red), 10))
		r.cur.Rows++
	case KindBeat:
		iv := ""
		if r.lastBeatMs != 0 && m.Ms > r.lastBeatMs {
			iv = strconv.FormatUint(uint64(m.Ms-r.lastBeatMs), 10)
		}
		r.lastBeatMs = m.Ms
		r.b.line(ts, ms, opt(m.BPM), iv)
		r.cur.Beats++
	case KindRaw:
		if r.r == nil {
			c, err := newCSV(filepath.Join(r.dir, r.cur.Name+"_raw.csv"), "время;мс_устройства;ir;red")
			if err != nil {
				return
			}
			r.r = c
		}
		r.r.line(ts, ms, strconv.FormatUint(uint64(m.IR), 10), strconv.FormatUint(uint64(m.Red), 10))
		r.cur.Raw++
	}
}

var sessRe = regexp.MustCompile(`^session_(\d{8}_\d{6})(?:_(.+?))?\.csv$`)

// List возвращает сохранённые сессии, новые сверху.
func (r *Recorder) List() ([]SessionInfo, error) {
	ents, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	cur := r.Current()
	byName := map[string]*SessionInfo{}
	var out []*SessionInfo
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, "session_") || !strings.HasSuffix(n, ".csv") {
			continue
		}
		base := strings.TrimSuffix(n, ".csv")
		base = strings.TrimSuffix(strings.TrimSuffix(base, "_beats"), "_raw")
		s := byName[base]
		if s == nil {
			s = &SessionInfo{Name: base}
			if m := sessRe.FindStringSubmatch(base + ".csv"); m != nil {
				s.Started, _ = time.ParseInLocation("20060102_150405", m[1], time.Local)
				s.Label = m[2]
			}
			byName[base] = s
			out = append(out, s)
		}
		if fi, err := e.Info(); err == nil {
			s.Size += fi.Size()
		}
		s.Files = append(s.Files, n)
		if cur != nil && cur.Name == base {
			s.Active, s.Rows, s.Beats, s.Raw = true, cur.Rows, cur.Beats, cur.Raw
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	res := make([]SessionInfo, len(out))
	for i, s := range out {
		sort.Strings(s.Files)
		res[i] = *s
	}
	return res, nil
}

// SafeFile проверяет, что имя — файл сессии в папке данных, и возвращает путь.
func (r *Recorder) SafeFile(name string) (string, bool) {
	if name != filepath.Base(name) || !strings.HasPrefix(name, "session_") || !strings.HasSuffix(name, ".csv") {
		return "", false
	}
	p := filepath.Join(r.dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// Delete удаляет все файлы сессии (кроме активной).
func (r *Recorder) Delete(base string) error {
	if c := r.Current(); c != nil && c.Name == base {
		return fmt.Errorf("сессия сейчас записывается")
	}
	list, err := r.List()
	if err != nil {
		return err
	}
	for _, s := range list {
		if s.Name == base {
			for _, f := range s.Files {
				if p, ok := r.SafeFile(f); ok {
					os.Remove(p)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("сессия не найдена")
}

// Flush сбрасывает буферы текущей сессии на диск (перед скачиванием).
func (r *Recorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range []*csvFile{r.d, r.b, r.r} {
		if c != nil {
			c.flush()
		}
	}
}
