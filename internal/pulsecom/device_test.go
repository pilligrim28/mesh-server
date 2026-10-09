package pulsecom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Полный путь: эмулятор Arduino -> рукопожатие -> команды -> запись CSV.
func TestDeviceWithSim(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	dev := NewDevice("")
	dev.Open = OpenSim
	dev.List = func() []PortInfo { return []PortInfo{{Name: "SIM"}} }
	got := make(chan Msg, 1000)
	dev.OnMsg = func(at time.Time, m Msg) { rec.Write(at, m); got <- m }
	s, err := rec.Start("тест")
	if err != nil {
		t.Fatal(err)
	}
	go dev.Run()

	deadline := time.After(5 * time.Second)
	for !dev.State().Connected {
		select {
		case <-deadline:
			t.Fatal("не подключилось")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := dev.Send("RAW ON"); err != nil {
		t.Fatal(err)
	}
	if err := dev.Send("RATE 200"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	timeout := time.After(3 * time.Second)
loop:
	for {
		select {
		case m := <-got:
			seen[m.Kind]++
			if seen[KindRaw] > 50 && seen[KindData] > 3 && seen[KindStatus] > 0 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}
	if seen[KindRaw] <= 50 || seen[KindData] <= 3 || seen[KindStatus] == 0 {
		t.Fatalf("мало данных: %v", seen)
	}
	if st := dev.State(); st.Status == nil || !st.Status.RawOn || st.Status.RateMs != 200 {
		t.Fatalf("статус не обновился: %+v", st.Status)
	}
	rec.Stop()
	b, _ := os.ReadFile(filepath.Join(dir, s.Name+".csv"))
	lines := strings.Split(strings.TrimSpace(string(b)), "\r\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "\uFEFFвремя;") || strings.Count(lines[1], ";") != 6 {
		t.Fatalf("плохой CSV: %q", lines)
	}
	list, _ := rec.List()
	if len(list) != 1 || len(list[0].Files) != 3 || list[0].Label != "тест" {
		t.Fatalf("список сессий: %+v", list)
	}
	if _, ok := rec.SafeFile("../x.csv"); ok {
		t.Fatal("path traversal")
	}
}
