package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"mesh-server/client"
	"mesh-server/config"
	"mesh-server/database"
	"mesh-server/discovery"
	"mesh-server/handler"
	"mesh-server/repository"
	"mesh-server/service"
	"mesh-server/simulator"
)

func main() {
	// Загрузка конфигурации
	cfg := config.Load()
	log.Printf("Starting mesh-server on port %s", cfg.ServerPort)

	// Инициализация базы данных
	db, err := database.NewDatabase(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Инициализация репозиториев
	deviceRepo := repository.NewDeviceRepository(db.DB)
	metricsRepo := repository.NewMetricsRepository(db.DB)
	healbeRepo := repository.NewHealbeRepository(db.DB)
	alertRepo := repository.NewAlertRepository(db.DB)
	messageRepo := repository.NewMessageRepository(db.DB, deviceRepo)
	discoveryRepo := repository.NewDiscoveryRepository(db.DB)

	// Инициализация BLE сканера
	bleScanner := discovery.NewBLEScanner(discoveryRepo)

	// Инициализация ESP32 Bluetooth клиента
	esp32BtClient := client.NewESP32BluetoothClient("")

	// Инициализация сервисов
	services := service.NewServices(deviceRepo, metricsRepo, alertRepo, messageRepo, discoveryRepo, cfg.ESP32URL)

	// Инициализация ESP32 handler
	esp32Handler := handler.NewESP32Handler(bleScanner, esp32BtClient, messageRepo, services.WSHandler)

	// Инициализация MQTT сервиса (Meshtastic integration)
	mqttConfig := service.MQTTConfig{
		Enabled:             cfg.MQTTEnabled,
		Server:              cfg.MQTTServer,
		Username:            cfg.MQTTUsername,
		Password:            cfg.MQTTPassword,
		RootTopic:           cfg.MQTTRootTopic,
		JSONEnabled:         true,
		EncryptionEnabled:   false,
		MapReportingEnabled: cfg.MQTTMapReporting,
		MapReportInterval:   3600,
	}

	mqttService := service.NewMeshtasticService(deviceRepo, messageRepo, services.WSHandler, mqttConfig)
	services.MQTTService = mqttService

	if err := mqttService.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start MQTT service: %v", err)
	}
	defer mqttService.Stop()

	// Инициализация MQTT handler
	mqttHandler := handler.NewMQTTHandler(mqttService)

	// Инициализация Serial сервиса (USB подключение к Meshtastic)
	serialService := service.NewSerialService(messageRepo, deviceRepo)
	if cfg.ESP32COMPort != "" {
		if err := serialService.Start(cfg.ESP32COMPort); err != nil {
			log.Printf("Warning: Failed to start serial service: %v", err)
		}
	}
	defer serialService.Stop()

	// Serial handler
	serialHandler := handler.NewSerialHandler(serialService)

	// Запуск сервиса обнаружения устройств
	discoveryConfig := discovery.DiscoveryConfig{
		EnableBluetooth: cfg.EnableBluetooth,
		EnableWiFi:      cfg.EnableWiFi,
		ScanInterval:    cfg.DiscoveryInterval,
		MeshtasticIP:    cfg.MeshtasticIP,
	}
	if err := services.DiscoveryService.Start(discoveryConfig); err != nil {
		log.Printf("Warning: Failed to start discovery service: %v", err)
	}
	defer services.DiscoveryService.Stop()
	defer services.Close()

	// Запуск BLE сканера
	bleConfig := discovery.DiscoveryConfig{
		EnableBluetooth: cfg.EnableBluetooth,
	}
	if bleConfig.EnableBluetooth {
		if err := bleScanner.Start(context.Background()); err != nil {
			log.Printf("Warning: Failed to start BLE scanner: %v", err)
		}
		defer bleScanner.Stop()
	}

	// Инициализация симулятора носимого устройства
	sim := simulator.NewSimulator(deviceRepo, metricsRepo, alertRepo)
	sim.Start(5 * time.Second) // Генерация данных каждые 5 секунд
	defer sim.Stop()

	// Обработчик симулятора
	simHandler := handler.NewSimulatorHandler(sim)

	// Инициализация Healbe handler (часы GoBe)
	healbeHandler := handler.NewHealbeHandler(healbeRepo, metricsRepo, deviceRepo, services.WSHandler, bleScanner)
	// Устанавливаем ESP32 клиент для пересылки в Meshtastic
	if cfg.ESP32URL != "" {
		esp32Client := client.NewESP32Client(cfg.ESP32URL)
		healbeHandler.SetMeshtasticClient(esp32Client)
	}

	// Настройка роутинга
	mux := http.NewServeMux()

	// Devices API
	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.DeviceHandler.GetAll(w, r)
		case http.MethodPost:
			services.DeviceHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Metrics API
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.MetricsHandler.GetLatest(w, r)
		case http.MethodPost:
			services.MetricsHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/metrics/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			services.MetricsHandler.GetByDeviceID(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Alerts API
	mux.HandleFunc("/api/alerts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.AlertHandler.GetUnread(w, r)
		case http.MethodPost:
			services.AlertHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/alerts/device", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.AlertHandler.GetByDeviceID(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/alerts/read", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			services.AlertHandler.MarkAsRead(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/alerts/read-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			services.AlertHandler.MarkAllAsRead(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Messages API
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.MessageHandler.GetOutbound(w, r)
		case http.MethodPost:
			services.MessageHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/messages/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			services.MessageHandler.GetByDeviceID(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Map API
	mux.HandleFunc("/api/map", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			services.MapHandler.GetDevicesForMap(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// WebSocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		services.WSHandler.ServeHTTP(w, r)
	})

	// Discovery API
	mux.HandleFunc("/api/discovery", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.DiscoveryHandler.GetAllDevices(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/bluetooth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.DiscoveryHandler.GetBluetoothDevices(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/wifi", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			services.DiscoveryHandler.GetWiFiDevices(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/meshtastic", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			services.DiscoveryHandler.GetMeshtasticDevices(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			services.DiscoveryHandler.GetStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			services.DiscoveryHandler.StartScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/bluetooth/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			services.DiscoveryHandler.StartBluetoothScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/bluetooth/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			services.DiscoveryHandler.StopBluetoothScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ESP32 API
	mux.HandleFunc("/api/esp32/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			esp32Handler.ScanForESP32(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/esp32/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			esp32Handler.ConnectToESP32(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/esp32/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			esp32Handler.Disconnect(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/esp32/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			esp32Handler.GetStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/esp32/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			esp32Handler.ReceiveMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Serial API (COM-порт / USB)
	mux.HandleFunc("/api/serial/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			serialHandler.ScanPorts(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/serial/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			serialHandler.Connect(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/serial/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			serialHandler.Disconnect(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/serial/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			serialHandler.GetStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/serial/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			serialHandler.SendMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// MQTT API
	mux.HandleFunc("/api/mqtt/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			mqttHandler.GetStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/mqtt/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mqttHandler.Connect(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/mqtt/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mqttHandler.Disconnect(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/mqtt/message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mqttHandler.SendMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/esp32/bluetooth", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			esp32Handler.GetBluetoothDevices(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/wifi/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			services.DiscoveryHandler.StartWiFiScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/wifi/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			services.DiscoveryHandler.StopWiFiScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			services.DiscoveryHandler.ClearDevices(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/network", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			services.DiscoveryHandler.GetNetworkInfo(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Simulator API
	mux.HandleFunc("/api/simulator/status", simHandler.Status)
	mux.HandleFunc("/api/simulator/start", simHandler.Start)
	mux.HandleFunc("/api/simulator/stop", simHandler.Stop)
	mux.HandleFunc("/api/simulator/event", simHandler.Event)

	// Healbe API (часы GoBe)
	mux.HandleFunc("/api/healbe/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			healbeHandler.ScanHealbeDevices(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/healbe/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			healbeHandler.ConnectToHealbe(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/healbe/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			healbeHandler.DisconnectHealbe(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/healbe/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			healbeHandler.GetHealbeStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/healbe/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			healbeHandler.GetHealbeData(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/healbe/forward", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			healbeHandler.ForwardToMeshtastic(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Static files (frontend)
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "static/index.html")
		} else {
			http.NotFound(w, r)
		}
	})

	// Middleware для логгирования и CORS
	loggedMux := withLogging(withCORS(mux))

	log.Printf("Server listening on :%s", cfg.ServerPort)
	if err := http.ListenAndServe(":"+cfg.ServerPort, loggedMux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
