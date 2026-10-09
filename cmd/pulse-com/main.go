// pulse-com — чтение пульсоксиметра MAX30102 с Arduino Pro Micro (скетч MaxPulse)
// по COM-порту, запись каждой сессии в отдельные CSV и живой веб-интерфейс.
//
//	pulse-com                 автопоиск платы, интерфейс http://127.0.0.1:8081
//	pulse-com -port COM5      конкретный порт (Linux: /dev/ttyACM0)
//	pulse-com -list           показать COM-порты
//	pulse-com -sim            без железа: встроенный эмулятор Arduino
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"mesh-server/internal/pulsecom"
)

var version = "1.0.0"

func main() {
	port := flag.String("port", "", "COM-порт (пусто — автопоиск), напр. COM5 или /dev/ttyACM0")
	dir := flag.String("dir", "", "папка для CSV (по умолчанию data рядом с программой)")
	addr := flag.String("addr", "127.0.0.1:8081", "адрес веб-интерфейса (0.0.0.0:8081 — доступ из сети)")
	noBrowser := flag.Bool("no-browser", false, "не открывать браузер")
	noRec := flag.Bool("no-autorec", false, "не начинать запись автоматически при запуске")
	label := flag.String("label", "", "метка первой сессии (например, имя)")
	list := flag.Bool("list", false, "показать COM-порты и выйти")
	sim := flag.Bool("sim", false, "эмулятор Arduino вместо реального порта")
	flag.Parse()

	if *list {
		ps := pulsecom.ListPorts()
		if len(ps) == 0 {
			fmt.Println("COM-порты не найдены")
		}
		for _, p := range ps {
			mark := " "
			if p.Arduino {
				mark = "*"
			}
			fmt.Printf("%s %-16s %s\n", mark, p.Name, p.Desc)
		}
		return
	}

	if *dir == "" {
		*dir = "data"
		if exe, err := os.Executable(); err == nil && !isTempDir(exe) {
			*dir = filepath.Join(filepath.Dir(exe), "data")
		}
	}
	rec, err := pulsecom.NewRecorder(*dir)
	if err != nil {
		log.Fatalf("папка данных: %v", err)
	}
	abs, _ := filepath.Abs(*dir)
	log.Printf("pulse-com %s, данные: %s", version, abs)

	dev := pulsecom.NewDevice(*port)
	if *sim {
		dev.Open = pulsecom.OpenSim
		dev.List = func() []pulsecom.PortInfo {
			return []pulsecom.PortInfo{{Name: "SIM", Desc: "эмулятор Arduino", Arduino: true}}
		}
	}
	app := pulsecom.NewApp(dev, rec)
	if !*noRec {
		if s, err := rec.Start(*label); err == nil {
			log.Printf("Запись: %s.csv", s.Name)
		}
	}
	go dev.Run()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("порт %s занят (уже запущено?): %v", *addr, err)
	}
	url := "http://" + ln.Addr().String()
	if h, p, _ := net.SplitHostPort(ln.Addr().String()); h == "0.0.0.0" || h == "::" {
		url = "http://127.0.0.1:" + p
	}
	log.Printf("Интерфейс: %s  (Ctrl+C — выход)", url)
	if !*noBrowser {
		go func() { time.Sleep(400 * time.Millisecond); openBrowser(url) }()
	}
	srv := &http.Server{Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil {
			log.Fatal(err)
		}
	}()
	waitSignal()
	rec.Stop()
	log.Print("Файлы сохранены, выход")
}

func isTempDir(exe string) bool { // go run собирает во временную папку
	rel, err := filepath.Rel(os.TempDir(), exe)
	return err == nil && !filepath.IsAbs(rel) && rel[0] != '.'
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	if c.Start() == nil {
		go c.Wait()
	}
}
