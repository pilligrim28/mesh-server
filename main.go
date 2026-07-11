package main

import (
	"context"
	"log"
	"net/http"
	"strings"
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
	if cfg.DemoMode {
		log.Println("=== DEMO MODE: презентация для заказчика ===")
		log.Printf("ESP32 USB: %s | Healbe: симулятор | BLE-скан: выкл", cfg.ESP32COMPort)
	}
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

	// Инициализация сервисов и HTTP-обработчиков
	services := service.NewServices(discoveryRepo, deviceRepo)
	handlers := handler.NewHandlers(deviceRepo, metricsRepo, alertRepo, messageRepo, services.DiscoveryService, cfg.ESP32URL)
	defer handlers.Close()

	// Инициализация ESP32 handler
	esp32Handler := handler.NewESP32Handler(bleScanner, esp32BtClient, messageRepo, handlers.WS)

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

	mqttService := service.NewMeshtasticService(deviceRepo, messageRepo, handlers.WS, mqttConfig)
	services.MQTTService = mqttService

	if err := mqttService.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start MQTT service: %v", err)
	}
	defer mqttService.Stop()

	// Инициализация MQTT handler
	mqttHandler := handler.NewMQTTHandler(mqttService)

	// Инициализация Serial сервиса (USB hub для Meshtastic)
	serialService := service.NewSerialService(messageRepo, deviceRepo, handlers.WS)
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
	if !cfg.PeopleSimEnabled && !cfg.DemoMode {
		sim.Start(5 * time.Second)
	}
	defer sim.Stop()

	peopleSim := simulator.NewPeopleMovementSimulator(
		deviceRepo, metricsRepo, alertRepo,
		simulator.PeopleMovementConfig{
			CenterLat:   cfg.PeopleSimCenterLat,
			CenterLon:   cfg.PeopleSimCenterLon,
			RadiusKm:    cfg.PeopleSimRadiusKm,
			PersonCount: cfg.PeopleSimCount,
			Interval:    time.Duration(cfg.PeopleSimInterval) * time.Second,
		},
	)
	if cfg.PeopleSimEnabled || cfg.DemoMode {
		if err := peopleSim.Start(); err != nil {
			log.Printf("Warning: People movement sim failed: %v", err)
		}
		defer peopleSim.Stop()
	}

	// Обработчики симуляторов
	simHandler := handler.NewSimulatorHandler(sim)
	peopleSimHandler := handler.NewPeopleSimHandler(peopleSim)

	// Инициализация ESP32 Hub (мост между mesh-сетями и сервером)
	hubURL := cfg.ESP32URL
	if hubURL == "" {
		hubURL = cfg.MeshtasticIP
	}
	if hubURL != "" && !strings.HasPrefix(hubURL, "http") {
		hubURL = "http://" + hubURL
	}

	esp32HubService := service.NewESP32HubService(
		deviceRepo,
		messageRepo,
		handlers.WS,
		service.ESP32HubConfig{
			Enabled:      cfg.ESP32HubEnabled && hubURL != "",
			DeviceURL:    hubURL,
			PollInterval: cfg.ESP32HubPollInterval,
		},
	)
	if err := esp32HubService.Start(context.Background()); err != nil {
		log.Printf("Warning: Failed to start ESP32 hub service: %v", err)
	}
	defer esp32HubService.Stop()

	if cfg.ESP32HubEnabled && hubURL != "" {
		handlers.Message.SetMeshSender(esp32HubService)
	} else if cfg.ESP32COMPort != "" && serialService.IsConnected() {
		handlers.Message.SetMeshSender(serialService)
		log.Printf("USB Meshtastic hub active on %s", cfg.ESP32COMPort)
	}

	esp32HubHandler := handler.NewESP32HubHandler(esp32HubService)

	healbeBridgeURL := cfg.HealbeBridgeURL
	if healbeBridgeURL == "" {
		healbeBridgeURL = cfg.ESP32URL
	}
	if healbeBridgeURL != "" && !strings.HasPrefix(healbeBridgeURL, "http") {
		healbeBridgeURL = "http://" + healbeBridgeURL
	}

	meshSenderDesc := "не настроен"
	if cfg.ESP32COMPort != "" && serialService.IsConnected() {
		meshSenderDesc = "serial:" + cfg.ESP32COMPort
	} else if cfg.ESP32HubEnabled && hubURL != "" {
		meshSenderDesc = "wifi:" + hubURL
	} else if cfg.ESP32URL != "" {
		meshSenderDesc = "wifi:" + cfg.ESP32URL
	}

	// Инициализация Healbe handler (часы GoBe)
	healbeHandler := handler.NewHealbeHandler(healbeRepo, metricsRepo, deviceRepo, handlers.WS, bleScanner)
	healbeHandler.InitESP32Bridge(healbeBridgeURL)
	healbeHandler.SetRuntimeInfo(handler.HealbeRuntimeInfo{
		BridgeURL:      healbeBridgeURL,
		MeshSenderDesc: meshSenderDesc,
	})

	if cfg.ESP32COMPort != "" && serialService.IsConnected() {
		healbeHandler.SetMeshSender(serialService)
	} else if cfg.ESP32HubEnabled && hubURL != "" {
		healbeHandler.SetMeshtasticClient(client.NewESP32Client(hubURL))
	} else if cfg.ESP32URL != "" {
		healbeHandler.SetMeshtasticClient(client.NewESP32Client(cfg.ESP32URL))
	}

	if cfg.HealbeMAC != "" && !cfg.DemoMode {
		healbeHandler.SetForwardEnabled(cfg.HealbeForwardMesh)
		go func() {
			time.Sleep(2 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := healbeHandler.AutoConnect(ctx, cfg.HealbeMAC, cfg.HealbeMode, cfg.HealbeForwardMesh); err != nil {
				log.Printf("Healbe auto-connect failed: %v", err)
			}
		}()
	}

	var healbeDemo *simulator.HealbeDemo
	if cfg.DemoMode {
		healbeHandler.EnableDemo(cfg.HealbeMAC, cfg.HealbeForwardMesh)
		healbeDemo = simulator.NewHealbeDemo(healbeRepo, deviceRepo, cfg.HealbeMAC, healbeHandler.PublishDemoData)
		healbeDemo.Start(5 * time.Second)
		defer healbeDemo.Stop()
		log.Printf("Healbe demo active for MAC %s", cfg.HealbeMAC)
	}

	systemHandler := handler.NewSystemHandler(cfg.DemoMode, cfg.ServerPort, serialService)

	// Настройка роутинга
	mux := http.NewServeMux()

	// Devices API
	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Device.GetAll(w, r)
		case http.MethodPost:
			handlers.Device.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Metrics API
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Metrics.GetLatest(w, r)
		case http.MethodPost:
			handlers.Metrics.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/metrics/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.Metrics.GetByDeviceID(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Alerts API
	mux.HandleFunc("/api/alerts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Alert.GetUnread(w, r)
		case http.MethodPost:
			handlers.Alert.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/alerts/device", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Alert.GetByDeviceID(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/alerts/read", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			handlers.Alert.MarkAsRead(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/alerts/read-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			handlers.Alert.MarkAllAsRead(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Messages API
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Message.GetOutbound(w, r)
		case http.MethodPost:
			handlers.Message.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/messages/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.Message.GetByDeviceID(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Map API
	mux.HandleFunc("/api/map", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.Map.GetDevicesForMap(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// WebSocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handlers.WS.ServeHTTP(w, r)
	})

	// Discovery API
	mux.HandleFunc("/api/discovery", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Discovery.GetAllDevices(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/bluetooth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Discovery.GetBluetoothDevices(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/wifi", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.Discovery.GetWiFiDevices(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/meshtastic", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.Discovery.GetMeshtasticDevices(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.Discovery.GetStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handlers.Discovery.StartScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/bluetooth/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handlers.Discovery.StartBluetoothScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/bluetooth/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handlers.Discovery.StopBluetoothScan(w, r)
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

	mux.HandleFunc("/api/esp32/hub/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			esp32HubHandler.GetStatus(w, r)
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
			handlers.Discovery.StartWiFiScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/wifi/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handlers.Discovery.StopWiFiScan(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handlers.Discovery.ClearDevices(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/discovery/network", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.Discovery.GetNetworkInfo(w, r)
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

	mux.HandleFunc("/api/people-sim/status", peopleSimHandler.Status)
	mux.HandleFunc("/api/people-sim/zone", peopleSimHandler.Zone)
	mux.HandleFunc("/api/people-sim/start", peopleSimHandler.Start)
	mux.HandleFunc("/api/people-sim/stop", peopleSimHandler.Stop)

	// System API (демо-режим)
	mux.HandleFunc("/api/system/status", systemHandler.Status)

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
	mux.HandleFunc("/api/healbe/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			healbeHandler.IngestHealbeData(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/healbe/esp32/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			healbeHandler.GetESP32HealbeConfig(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Static files (frontend) — без кэша для JS/CSS при разработке
	mux.Handle("/static/", noCacheStatic(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))

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

func noCacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
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
