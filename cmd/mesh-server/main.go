// mesh-server — приём данных пульса с ESP32-C3 через сеть Meshtastic.
//
//	ESP32-C3 + MAX30102 --BLE--> Heltec V3 (носимый) ~~LoRa mesh~~> Heltec V3 (шлюз) --BLE/USB--> mesh-server
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mesh-server/internal/config"
	"mesh-server/internal/core"
	"mesh-server/internal/store"
	"mesh-server/internal/transport"
	"mesh-server/internal/web"
	webui "mesh-server/web"
)

var version = "3.0.0"

func main() {
	resetPw := flag.String("reset-password", "", "задать новый пароль пользователю ADMIN_USER и выйти")
	showVer := flag.Bool("version", false, "показать версию")
	flag.Parse()
	if *showVer {
		fmt.Println("mesh-server", version)
		return
	}

	cfg := config.Load()
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("БД: %v", err)
	}
	defer st.Close()

	if *resetPw != "" {
		if _, err := st.EnsureUser(cfg.AdminUser, *resetPw); err != nil {
			log.Fatal(err)
		}
		if err := st.SetPassword(cfg.AdminUser, *resetPw); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Пароль обновлён для", cfg.AdminUser)
		return
	}
	if created, err := st.EnsureUser(cfg.AdminUser, cfg.AdminPassword); err != nil {
		log.Fatalf("пользователь: %v", err)
	} else if created {
		log.Printf("Создан пользователь %q. Смените пароль в интерфейсе!", cfg.AdminUser)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	svc := core.NewService(cfg, st, core.NewHub())
	go svc.Watchdog(ctx)

	for _, t := range cfg.Transports {
		switch t {
		case "ble":
			go transport.NewBLE(transport.BLEOptions{NameFilter: cfg.BLENameFilter, Addresses: cfg.BLEAddresses,
				MaxGateways: cfg.BLEMaxGateways, ScanTimeout: cfg.BLEScanTimeout}, svc).Run(ctx)
		case "serial":
			if cfg.SerialPort == "" {
				log.Fatal("TRANSPORT=serial требует SERIAL_PORT (например COM4 или /dev/ttyUSB0)")
			}
			go transport.NewSerial(cfg.SerialPort, svc).Run(ctx)
		case "sim":
			go transport.NewSim(cfg.SimNodes, 5*time.Second, svc).Run(ctx)
		default:
			log.Fatalf("неизвестный транспорт %q (ble, serial, sim)", t)
		}
	}

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: web.New(svc, webui.Files).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sh, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		srv.Shutdown(sh)
	}()
	log.Printf("mesh-server %s: http://localhost%s  (транспорты: %v)", version, cfg.HTTPAddr, cfg.Transports)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
