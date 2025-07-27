package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type EndpointConfig struct {
	Name        string `json:"name"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
	NeedsBody   bool   `json:"needs_body"`
}

type LoadProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

type AppState struct {
	Running     bool             `json:"running"`
	RPS         int              `json:"rps"`
	CurrentRPS  int              `json:"current_rps"`
	Endpoint    string           `json:"endpoint"`
	Profile     string           `json:"profile"`
	TotalReqs   int64            `json:"total_requests"`
	SuccessReqs int64            `json:"success_requests"`
	ErrorReqs   int64            `json:"error_requests"`
	StartTime   time.Time        `json:"start_time"`
	Endpoints   []EndpointConfig `json:"endpoints"`
	Profiles    []LoadProfile    `json:"profiles"`
}

var endpoints = []EndpointConfig{
	{Name: "Главная страница", Method: "GET", Path: "/", Description: "Главная страница приюта", NeedsBody: false},
	{Name: "Список животных", Method: "GET", Path: "/animals", Description: "Получить список всех животных", NeedsBody: false},
	{Name: "Добавить животное", Method: "POST", Path: "/api/animals", Description: "Добавить новое животное", NeedsBody: true},
	{Name: "Метрики", Method: "GET", Path: "/metrics", Description: "Метрики Prometheus", NeedsBody: false},
	{Name: "Документация", Method: "GET", Path: "/docs", Description: "Документация API", NeedsBody: false},
	{Name: "Замедлялка", Method: "GET", Path: "/slow", Description: "Замедлялка", NeedsBody: false},
}

var profiles = []LoadProfile{
	{Name: "Постоянная нагрузка", Description: "Стабильный RPS на протяжении всего теста", Type: "constant"},
	{Name: "Постепенное нарастание", Description: "Плавное увеличение RPS от 1 до заданного значения", Type: "ramp_up"},
	{Name: "Пиковая нагрузка", Description: "Резкий скачок до максимума, затем постоянная нагрузка", Type: "spike"},
	{Name: "Волнообразная нагрузка", Description: "Циклические колебания RPS (синусоида)", Type: "wave"},
	{Name: "Ступенчатая нагрузка", Description: "Пошаговое увеличение нагрузки каждые 30 секунд", Type: "step"},
	{Name: "Стресс-тест", Description: "Экстремальная нагрузка с случайными всплесками", Type: "stress"},
}

type LoadTester struct {
	state     AppState
	stopChan  chan struct{}
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning int32
	wg        sync.WaitGroup
}

var (
	loadTester  *LoadTester
	testerMutex sync.Mutex
)

const indexHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Нагрузочное тестирование Приюта для животных</title>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 0;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
        }
        .container {
            max-width: 1000px;
            margin: 0 auto;
            background: white;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
            color: white;
            padding: 30px;
            text-align: center;
        }
        .header h1 {
            margin: 0;
            font-size: 2.5em;
            font-weight: 300;
        }
        .content {
            padding: 30px;
        }
        .control-panel {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            margin-bottom: 30px;
        }
        .control-section {
            background: #f8f9fa;
            padding: 25px;
            border-radius: 10px;
            border: 1px solid #e9ecef;
        }
        .control-section h3 {
            margin-top: 0;
            color: #495057;
            border-bottom: 2px solid #dee2e6;
            padding-bottom: 10px;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            font-weight: 600;
            color: #495057;
        }
        select, input[type="number"] {
            width: 100%;
            padding: 12px;
            border: 2px solid #ced4da;
            border-radius: 6px;
            font-size: 16px;
            transition: border-color 0.3s;
        }
        select:focus, input[type="number"]:focus {
            outline: none;
            border-color: #667eea;
        }
        .buttons {
            display: flex;
            gap: 15px;
            margin-top: 20px;
        }
        button {
            flex: 1;
            padding: 15px;
            border: none;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s;
        }
        .toggle-btn {
            background: linear-gradient(135deg, #00d2ff, #3a7bd5);
            color: white;
            position: relative;
            overflow: hidden;
        }
        .toggle-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(0, 210, 255, 0.4);
        }
        .toggle-btn.running {
            background: linear-gradient(135deg, #ff6b6b, #ee5a24);
        }
        .toggle-btn.running:hover {
            box-shadow: 0 5px 15px rgba(255, 107, 107, 0.4);
        }
        .toggle-btn:disabled {
            opacity: 0.6;
            cursor: not-allowed;
            transform: none;
        }
        .status-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
        }
        .status-item {
            text-align: center;
            padding: 15px;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
        }
        .status-value {
            font-size: 2em;
            font-weight: bold;
            margin-bottom: 5px;
        }
        .status-label {
            color: #6c757d;
            font-size: 0.9em;
        }
        .running .status-value {
            color: #28a745;
        }
        .stopped .status-value {
            color: #dc3545;
        }
        .endpoint-info {
            background: #e3f2fd;
            padding: 15px;
            border-radius: 8px;
            margin-top: 10px;
            border-left: 4px solid #2196f3;
        }
        .method-badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: bold;
            color: white;
            margin-right: 10px;
        }
        .method-GET { background: #28a745; }
        .method-POST { background: #007bff; }
        .method-PUT { background: #ffc107; color: #212529; }
        .method-DELETE { background: #dc3545; }
    </style>
    <script>
        let updateInterval;
        let endpoints = ENDPOINTS_JSON_PLACEHOLDER;
        let profiles = PROFILES_JSON_PLACEHOLDER;

        function updateEndpointInfo() {
            const select = document.getElementById('endpoint');
            const selectedPath = select.value;
            const endpoint = endpoints.find(ep => ep.path === selectedPath);
            
            if (endpoint) {
                document.getElementById('endpoint-info').innerHTML =
                    '<span class="method-badge method-' + endpoint.method + '">' + endpoint.method + '</span>' +
                    '<strong>' + endpoint.path + '</strong><br>' +
                    '<small>' + endpoint.description + '</small>';
            }
        }

        function updateProfileInfo() {
            const select = document.getElementById('profile');
            const selectedType = select.value;
            const profile = profiles.find(p => p.type === selectedType);
            
            if (profile) {
                let profileIcon = '📊';
                switch (selectedType) {
                    case 'constant': profileIcon = '➖'; break;
                    case 'ramp_up': profileIcon = '📈'; break;
                    case 'spike': profileIcon = '⚡'; break;
                    case 'wave': profileIcon = '🌊'; break;
                    case 'step': profileIcon = '📶'; break;
                    case 'stress': profileIcon = '💥'; break;
                }

                document.getElementById('profile-info').innerHTML =
                    '<span style="font-size: 16px; margin-right: 8px;">' + profileIcon + '</span>' +
                    '<strong>' + selectedType + '</strong><br>' +
                    '<small>' + profile.description + '</small>';
            }
        }

        async function toggleLoadTest() {
            const button = document.getElementById('toggle-btn');
            const isRunning = button.classList.contains('running');
            button.disabled = true;

            if (isRunning) {
                try {
                    const response = await fetch('/stop', { method: 'POST' });
                    updateStatus();
                    stopStatusUpdates();
                } catch (error) {
                    alert('Ошибка подключения: ' + error.message);
                } finally {
                    button.disabled = false;
                }
            } else {
                const endpoint = document.getElementById('endpoint').value;
                const rps = document.getElementById('rps').value;
                const profile = document.getElementById('profile').value;

                if (!rps || rps <= 0) {
                    alert('Пожалуйста, введите корректное значение RPS');
                    button.disabled = false;
                    return;
                }

                try {
                    const response = await fetch('/start', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ endpoint: endpoint, rps: parseInt(rps), profile: profile })
                    });

                    if (response.ok) {
                        startStatusUpdates();
                    } else {
                        const result = await response.text();
                        alert('Ошибка: ' + result);
                    }
                } catch (error) {
                    alert('Ошибка подключения: ' + error.message);
                } finally {
                    button.disabled = false;
                }
            }
        }

        async function updateStatus() {
            try {
                const response = await fetch('/status');
                const status = await response.json();

                const button = document.getElementById('toggle-btn');
                if (button) {
                    if (status.running) {
                        button.classList.add('running');
                        button.innerHTML = '⏹️ Остановить';
                    } else {
                        button.classList.remove('running');
                        button.innerHTML = '▶️ Запустить';
                    }
                }

                const statusDiv = document.getElementById('status');
                const duration = status.running && status.start_time ?
                    Math.floor((Date.now() - new Date(status.start_time).getTime()) / 1000) : 0;

                const successRate = status.total_requests > 0 ?
                    ((status.success_requests / status.total_requests) * 100).toFixed(1) : 0;

                statusDiv.innerHTML =
                    '<div class="status-item ' + (status.running ? 'running' : 'stopped') + '">' +
                    '<div class="status-value">' + (status.running ? 'РАБОТАЕТ' : 'ОСТАНОВЛЕН') + '</div>' +
                    '<div class="status-label">Статус</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + (status.current_rps || status.rps) + '</div>' +
                    '<div class="status-label">Текущий RPS</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + status.rps + '</div>' +
                    '<div class="status-label">Макс. RPS</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + status.total_requests + '</div>' +
                    '<div class="status-label">Всего запросов</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + status.success_requests + '</div>' +
                    '<div class="status-label">Успешных</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + status.error_requests + '</div>' +
                    '<div class="status-label">Ошибок</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + successRate + '%</div>' +
                    '<div class="status-label">Успешность</div>' +
                    '</div>' +
                    '<div class="status-item">' +
                    '<div class="status-value">' + duration + 's</div>' +
                    '<div class="status-label">Время работы</div>' +
                    '</div>';
            } catch (error) {
                console.error('Ошибка обновления статуса:', error);
            }
        }

        function startStatusUpdates() {
            if (updateInterval) clearInterval(updateInterval);
            updateInterval = setInterval(updateStatus, 1000);
        }

        function stopStatusUpdates() {
            if (updateInterval) {
                clearInterval(updateInterval);
                updateInterval = null;
            }
        }

        document.addEventListener('DOMContentLoaded', function () {
            updateEndpointInfo();
            updateProfileInfo();
            updateStatus();
            startStatusUpdates();
        });
    </script>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🐾 Нагрузочное тестирование Приюта для животных</h1>
            <p>Инструмент для тестирования API приюта для животных</p>
        </div>
        <div class="content">
            <div class="control-panel">
                <div class="control-section">
                    <h3>⚙️ Настройки тестирования</h3>
                    <div class="form-group">
                        <label for="endpoint">Выберите эндпоинт:</label>
                        <select id="endpoint" onchange="updateEndpointInfo()">
                            ENDPOINTS_OPTIONS_PLACEHOLDER
                        </select>
                        <div id="endpoint-info" class="endpoint-info"></div>
                    </div>
                    <div class="form-group">
                        <label for="profile">Профиль нагрузки:</label>
                        <select id="profile" onchange="updateProfileInfo()">
                            PROFILES_OPTIONS_PLACEHOLDER
                        </select>
                        <div id="profile-info" class="endpoint-info"></div>
                    </div>
                    <div class="form-group">
                        <label for="rps">Максимальный RPS:</label>
                        <input type="number" id="rps" min="1" max="1000" value="10">
                        <small style="color: #6c757d; margin-top: 5px; display: block;">Для некоторых профилей это максимальное значение</small>
                    </div>
                    <div class="buttons">
                        <button id="toggle-btn" class="toggle-btn" onclick="toggleLoadTest()">▶️ Запустить</button>
                    </div>
                </div>
                <div class="control-section">
                    <h3>📊 Статус тестирования</h3>
                    <div id="status" class="status-grid">
                        <!-- Статус будет обновляться через JavaScript -->
                    </div>
                </div>
            </div>
        </div>
    </div>
</body>
</html>`

func main() {
	loadTester = &LoadTester{
		state: AppState{
			Running:    false,
			RPS:        10,
			CurrentRPS: 0,
			Endpoint:   "/animals",
			Profile:    "constant",
			Endpoints:  endpoints,
			Profiles:   profiles,
		},
		stopChan: make(chan struct{}),
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/start", startHandler)
	http.HandleFunc("/stop", stopHandler)
	http.HandleFunc("/status", statusHandler)

	port := getEnv("PORT", "3006")
	log.Printf("Starting Animal Shelter Load Tester on port %s", port)

	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-sigChan
	log.Println("Shutting down...")
	testerMutex.Lock()
	if loadTester != nil && loadTester.state.Running {
		loadTester.forceStop()
	}
	testerMutex.Unlock()
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	endpointsJSON, _ := json.Marshal(endpoints)
	profilesJSON, _ := json.Marshal(profiles)

	endpointsOptions := ""
	for _, ep := range endpoints {
		endpointsOptions += fmt.Sprintf(`<option value="%s">%s</option>`, ep.Path, ep.Name)
	}

	profilesOptions := ""
	for _, p := range profiles {
		profilesOptions += fmt.Sprintf(`<option value="%s">%s</option>`, p.Type, p.Name)
	}

	htmlBytes := []byte(indexHTML)
	htmlBytes = bytes.ReplaceAll(htmlBytes, []byte("ENDPOINTS_JSON_PLACEHOLDER"), endpointsJSON)
	htmlBytes = bytes.ReplaceAll(htmlBytes, []byte("PROFILES_JSON_PLACEHOLDER"), profilesJSON)
	htmlBytes = bytes.ReplaceAll(htmlBytes, []byte("ENDPOINTS_OPTIONS_PLACEHOLDER"), []byte(endpointsOptions))
	htmlBytes = bytes.ReplaceAll(htmlBytes, []byte("PROFILES_OPTIONS_PLACEHOLDER"), []byte(profilesOptions))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlBytes)
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Endpoint string `json:"endpoint"`
		RPS      int    `json:"rps"`
		Profile  string `json:"profile"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	testerMutex.Lock()
	defer testerMutex.Unlock()

	if loadTester.state.Running || atomic.LoadInt32(&loadTester.isRunning) == 1 {
		fmt.Fprintf(w, "Already running")
		return
	}

	if req.RPS <= 0 || req.RPS > 1000 {
		http.Error(w, "RPS must be between 1 and 1000", http.StatusBadRequest)
		return
	}

	loadTester.forceStop()
	loadTester.ctx, loadTester.cancel = context.WithCancel(context.Background())

	loadTester.mutex.Lock()
	loadTester.state.RPS = req.RPS
	loadTester.state.CurrentRPS = 1
	loadTester.state.Endpoint = req.Endpoint
	loadTester.state.Profile = req.Profile
	loadTester.state.Running = true
	loadTester.state.TotalReqs = 0
	loadTester.state.SuccessReqs = 0
	loadTester.state.ErrorReqs = 0
	loadTester.state.StartTime = time.Now()

	if req.Profile == "constant" || req.Profile == "spike" {
		loadTester.state.CurrentRPS = req.RPS
	}
	loadTester.mutex.Unlock()

	atomic.StoreInt32(&loadTester.isRunning, 1)
	go loadTester.runLoadTest()

	fmt.Fprintf(w, "Started load test for %s with %d RPS using %s profile", req.Endpoint, req.RPS, req.Profile)
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	testerMutex.Lock()
	defer testerMutex.Unlock()

	if !loadTester.state.Running {
		fmt.Fprintf(w, "Not running")
		return
	}

	loadTester.forceStop()
	fmt.Fprintf(w, "Stopped")
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	loadTester.mutex.RLock()
	data := loadTester.state
	loadTester.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (lt *LoadTester) forceStop() {
	log.Println("🛑 Force stop initiated")
	atomic.StoreInt32(&lt.isRunning, 0)

	if lt.cancel != nil {
		lt.cancel()
	}

	select {
	case lt.stopChan <- struct{}{}:
	default:
		close(lt.stopChan)
		lt.stopChan = make(chan struct{})
	}

	lt.mutex.Lock()
	lt.state.Running = false
	lt.state.CurrentRPS = 0
	lt.mutex.Unlock()

	done := make(chan struct{})
	go func() {
		lt.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("✅ All goroutines stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("⚠️ Warning: Some goroutines did not stop within timeout")
	}
}

func (lt *LoadTester) runLoadTest() {
	defer atomic.StoreInt32(&lt.isRunning, 0)
	targetURL := getEnv("TARGET_URL", "http://localhost:8000")

	lt.mutex.RLock()
	currentRPS := lt.state.CurrentRPS
	profile := lt.state.Profile
	maxRPS := lt.state.RPS
	lt.mutex.RUnlock()

	ticker := time.NewTicker(time.Second / time.Duration(currentRPS))
	defer ticker.Stop()

	profileStopChan := make(chan struct{})
	defer close(profileStopChan)

	go func() {
		profileTicker := time.NewTicker(1 * time.Second)
		defer profileTicker.Stop()

		for {
			select {
			case <-lt.ctx.Done():
				return
			case <-lt.stopChan:
				return
			case <-profileStopChan:
				return
			case <-profileTicker.C:
				if atomic.LoadInt32(&lt.isRunning) == 0 {
					return
				}

				newRPS := lt.calculateRPS(profile, maxRPS)
				if newRPS != currentRPS && newRPS > 0 {
					currentRPS = newRPS
					ticker.Reset(time.Second / time.Duration(currentRPS))

					lt.mutex.Lock()
					lt.state.CurrentRPS = currentRPS
					lt.mutex.Unlock()
				}
			}
		}
	}()

	for {
		select {
		case <-lt.ctx.Done():
			return
		case <-lt.stopChan:
			return
		case <-ticker.C:
			if atomic.LoadInt32(&lt.isRunning) == 0 {
				return
			}

			lt.wg.Add(1)
			go func() {
				defer lt.wg.Done()

				requestCtx, requestCancel := context.WithTimeout(lt.ctx, 5*time.Second)
				defer requestCancel()

				if atomic.LoadInt32(&lt.isRunning) == 0 {
					return
				}

				lt.mutex.RLock()
				endpoint := lt.state.Endpoint
				running := lt.state.Running
				lt.mutex.RUnlock()

				if !running || atomic.LoadInt32(&lt.isRunning) == 0 {
					return
				}

				err := makeRequestWithContext(requestCtx, targetURL+endpoint)

				if atomic.LoadInt32(&lt.isRunning) == 0 {
					return
				}

				lt.mutex.Lock()
				if lt.state.Running && atomic.LoadInt32(&lt.isRunning) == 1 {
					lt.state.TotalReqs++
					if err != nil {
						lt.state.ErrorReqs++
					} else {
						lt.state.SuccessReqs++
					}
				}
				lt.mutex.Unlock()
			}()
		}
	}
}

func (lt *LoadTester) calculateRPS(profile string, maxRPS int) int {
	lt.mutex.RLock()
	startTime := lt.state.StartTime
	lt.mutex.RUnlock()

	elapsed := time.Since(startTime).Seconds()

	switch profile {
	case "constant":
		return maxRPS
	case "ramp_up":
		duration := 60.0
		if elapsed >= duration {
			return maxRPS
		}
		progress := elapsed / duration
		return int(1 + float64(maxRPS-1)*progress)
	case "spike":
		if elapsed < 5 {
			return 1
		}
		return maxRPS
	case "wave":
		period := 60.0
		amplitude := float64(maxRPS) / 2
		baseline := amplitude
		wave := math.Sin(2 * math.Pi * elapsed / period)
		return int(baseline + amplitude*wave)
	case "step":
		step := int(elapsed / 30)
		stepSize := maxRPS / 5
		if stepSize == 0 {
			stepSize = 1
		}
		rps := (step + 1) * stepSize
		if rps > maxRPS {
			return maxRPS
		}
		return rps
	case "stress":
		baseRPS := maxRPS / 3
		if rand.Float64() < 0.1 {
			return maxRPS
		}
		variableRPS := baseRPS + rand.Intn(maxRPS/2)
		if variableRPS > maxRPS {
			return maxRPS
		}
		return variableRPS
	default:
		return maxRPS
	}
}

func generateRandomAnimal() map[string]interface{} {
	rand.Seed(time.Now().UnixNano())

	animalTypes := []string{"кот", "собака", "хомяк", "попугай", "черепаха"}
	healthStatuses := []string{"здоров", "на лечении", "реабилитация", "карантин"}
	names := []string{"Барсик", "Шарик", "Мурзик", "Рекс", "Пушок", "Тузик", "Васька", "Жучка"}

	return map[string]interface{}{
		"name":   names[rand.Intn(len(names))],
		"type":   animalTypes[rand.Intn(len(animalTypes))],
		"age":    rand.Intn(15) + 1, // Возраст от 1 до 15 лет
		"health": healthStatuses[rand.Intn(len(healthStatuses))],
	}
}

func makeRequestWithContext(ctx context.Context, url string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	loadTester.mutex.RLock()
	endpoint := loadTester.state.Endpoint
	loadTester.mutex.RUnlock()

	var method string
	var needsBody bool
	for _, ep := range endpoints {
		if ep.Path == endpoint {
			method = ep.Method
			needsBody = ep.NeedsBody
			break
		}
	}

	if method == "" {
		method = "GET"
	}

	var req *http.Request
	var err error

	if needsBody && method == "POST" && endpoint == "/api/animals" {
		animal := generateRandomAnimal()
		body, marshalErr := json.Marshal(animal)
		if marshalErr != nil {
			return marshalErr
		}

		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}

	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "AnimalShelter-LoadTester/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
