package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var elog debug.Log           // Para logs no Event Log do Windows
var globalLogger *log.Logger // Para logs em arquivo e console (debug mode)

// Estrutura para o diretório de pendências
var pendingDir string

// Config - Estrutura para o config.json
type Config struct {
	Setor     string `json:"setor"`
	IDEmpresa int    `json:"idEmpresa"` // CORRIGIDO: Tipo alterado para int
	// Agora, armazenaremos apenas o diretório base dos logs do PaperCut
	PapercutLogDir  string `json:"papercutLogDir"`
	ApiBaseURL      string `json:"apiBaseUrl"` // Novo campo: apenas o endereço base
	PollingInterval int    `json:"pollingIntervalSeconds"`
}

// PrintData representa a estrutura do JSON a ser enviado para a API.
type PrintData struct {
	Data        string `json:"data"`
	Hora        string `json:"hora"`
	Usuario     string `json:"usuario"`
	Setor       string `json:"setor"`
	Paginas     int    `json:"paginas"`
	Copias      int    `json:"copias"`
	Impressora  string `json:"impressora"`
	NomeArquivo string `json:"nomearquivo"`
	Tipo        string `json:"tipo"`
	NomePC      string `json:"nomepc"`
	TipoPage    string `json:"tipopage"`
	Cor         string `json:"cor"`
	Tamanho     string `json:"tamanho"`
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	IDEmpresa   int    `json:"empresa"` // CORRIGIDO: Tag JSON para corresponder ao schema do Prisma
}

// lastReadOffsets guarda o offset para CADA arquivo de log lido, usando o caminho completo como chave
var lastReadOffsets = make(map[string]int64)

type myservice struct{}

// Função para configurar o log em arquivo
func setupFileLogging(serviceName string) error {
	logDir := filepath.Join(os.Getenv("PROGRAMDATA"), serviceName+"ServiceLogs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory '%s': %w", logDir, err)
	}

	logFilePath := filepath.Join(logDir, "printwatch_service.log")

	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file '%s': %w", logFilePath, err)
	}

	multiWriter := io.MultiWriter(file, os.Stdout)

	globalLogger = log.New(multiWriter, "PRINTWATCH: ", log.Ldate|log.Ltime|log.Lshortfile)
	return nil
}

// Execute é o método principal onde a lógica do seu serviço roda.
func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (s_succeeded bool, s_errNo uint32) {
	if err := setupFileLogging("PrintWatch"); err != nil {
		elog.Error(1, fmt.Sprintf("Failed to set up file logging: %v", err))
		return false, 1
	}
	globalLogger.Println("Service Execute method started.")

	changes <- svc.Status{State: svc.StartPending}
	elog.Info(1, "PrintWatch Service starting...")

	cfg, err := readConfig()
	if err != nil {
		elog.Error(1, fmt.Sprintf("Failed to read config.json: %v", err))
		globalLogger.Println(fmt.Sprintf("ERROR: Failed to read config.json: %v", err))
		return false, 1
	}

	// NOVO: Configurar o diretório de pendências
	if err := setupPendingDir(); err != nil {
		elog.Error(1, fmt.Sprintf("Failed to set up pending directory: %v", err))
		globalLogger.Println(fmt.Sprintf("CRITICAL: Failed to set up pending directory: %v", err))
		return false, 1 // Falha ao iniciar se não puder criar o diretório
	}

	elog.Info(1, fmt.Sprintf("Config loaded: Setor=%s, IDEmpresa=%d, LogDir=%s, ApiBaseUrl=%s",
		cfg.Setor, cfg.IDEmpresa, cfg.PapercutLogDir, cfg.ApiBaseURL))
	globalLogger.Println(fmt.Sprintf("Config loaded: Setor=%s, IDEmpresa=%d, LogDir=%s, ApiBaseUrl=%s",
		cfg.Setor, cfg.IDEmpresa, cfg.PapercutLogDir, cfg.ApiBaseURL))

	// ** ALTERADO: Processar logs e pendências imediatamente ao iniciar **
	globalLogger.Println("PrintWatch: Executando tarefa inicial de processamento de pendências...")
	processPendingImpressions(cfg)

	globalLogger.Println("PrintWatch: Executando tarefa inicial de monitoramento de logs...")
	err = processPapercutLogs(cfg)
	if err != nil {
		// Apenas loga o erro, não impede o serviço de iniciar.
		globalLogger.Println(fmt.Sprintf("ERROR during initial log processing: %v", err))
		elog.Warning(1, fmt.Sprintf("Error during initial log processing: %v", err))
	}

	pollingInterval := time.Duration(cfg.PollingInterval) * time.Second
	if pollingInterval == 0 {
		pollingInterval = 10 * time.Second
	}
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	elog.Info(1, "PrintWatch Service started successfully.")
	globalLogger.Println("PrintWatch Service started successfully.")

	for {
		select {
		case <-ticker.C:
			globalLogger.Println("PrintWatch: Executando tarefa de monitoramento de logs...")
			err := processPapercutLogs(cfg)
			if err != nil {
				globalLogger.Println(fmt.Sprintf("ERROR during log processing: %v", err))
				elog.Warning(1, fmt.Sprintf("Error processing logs: %v", err))
			}
			// NOVO: Processar a fila de pendências a cada ciclo
			globalLogger.Println("PrintWatch: Executando tarefa de processamento de pendências...")
			processPendingImpressions(cfg)

		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				elog.Info(1, "PrintWatch Service received stop/shutdown command.")
				globalLogger.Println("PrintWatch Service received stop/shutdown command.")
				changes <- svc.Status{State: svc.StopPending}
				return true, 0
			case svc.Interrogate:
				elog.Info(1, "PrintWatch Service received interrogate command.")
				globalLogger.Println("PrintWatch Service received interrogate command.")
				changes <- c.CurrentStatus
			default:
				elog.Info(1, fmt.Sprintf("PrintWatch Service received unexpected control request #%d", c.Cmd))
				globalLogger.Println(fmt.Sprintf("PrintWatch Service received unexpected control request #%d", c.Cmd))
			}
		}
	}
}

// runService é uma função auxiliar para executar o serviço.
func runService(name string, isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(name)
	} else {
		elog, err = eventlog.Open(name)
		if err != nil {
			log.Fatalf("Failed to open event log: %v", err)
		}
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("%s Service is attempting to start...", name))
	err = svc.Run(name, &myservice{})
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s Service failed: %v", name, err))
		if globalLogger != nil {
			globalLogger.Println(fmt.Sprintf("ERROR: %s Service failed: %v", name, err))
		}
		log.Fatalf("%s Service failed: %v", name, err)
	}
	elog.Info(1, fmt.Sprintf("%s Service stopped.", name))
	if globalLogger != nil {
		globalLogger.Println(fmt.Sprintf("%s Service stopped.", name))
	}
}

// installService instala o serviço no Windows Service Control Manager.
func installService(name, displayName string, exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", name)
	}
	s, err = m.CreateService(name, exePath, mgr.Config{
		DisplayName: displayName,
		Description: "Monitors PaperCut logs and sends print data to the API.",
		StartType:   mgr.StartAutomatic,
	}, "is", "auto-started")
	if err != nil {
		return err
	}
	defer s.Close()

	err = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: time.Second * 10},
		{Type: mgr.ServiceRestart, Delay: time.Second * 20},
		{Type: mgr.ServiceRestart, Delay: time.Second * 40},
	}, 60)
	if err != nil {
		log.Printf("Warning: Failed to set service recovery actions: %v", err)
	}
	return nil
}

// removeService desinstala o serviço.
func removeService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("service %s is not installed", name)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	return nil
}

// startService inicia o serviço.
func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not open service %s: %v", name, err)
	}
	defer s.Close()
	err = s.Start("is", "auto-started")
	if err != nil {
		return fmt.Errorf("could not start service %s: %v", name, err)
	}
	return nil
}

// controlService envia comandos ao serviço e espera por um estado.
func controlService(name string, cmd svc.Cmd, desiredState svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not open service %s: %v", name, err)
	}
	defer s.Close()
	status, err := s.Control(cmd)
	if err != nil {
		return fmt.Errorf("could not send control %d to service %s: %v", cmd, name, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != desiredState {
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for service %s to reach state %d", name, desiredState)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not query service %s: %v", name, err)
		}
	}
	return nil
}

// main é o ponto de entrada do programa.
func main() {
	const serviceName = "PrintWatch"
	const serviceDisplayName = "PrintWatch Service (Monitor de Impressão)"

	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		log.Fatalf("failed to determine if interactive session: %v", err)
	}

	if !isIntSess {
		runService(serviceName, false)
		return
	}

	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		log.Fatalf("Failed to get absolute executable path: %v", err)
	}

	switch cmd {
	case "install":
		err = installService(serviceName, serviceDisplayName, exePath)
		if err != nil {
			log.Fatalf("failed to install %s: %v", serviceName, err)
		}
		log.Printf("Service %s installed\n", serviceName)
	case "remove":
		err = removeService(serviceName)
		if err != nil {
			log.Fatalf("failed to remove %s: %v", serviceName, err)
		}
		log.Printf("Service %s removed\n", serviceName)
	case "start":
		err = startService(serviceName)
		if err != nil {
			log.Fatalf("failed to start %s: %v", serviceName, err)
		}
		log.Printf("Service %s started\n", serviceName)
	case "stop":
		err = controlService(serviceName, svc.Stop, svc.Stopped)
		if err != nil {
			log.Fatalf("failed to stop %s: %v", serviceName, err)
		}
		log.Printf("Service %s stopped\n", serviceName)
	default:
		log.Printf("Running in interactive debug mode. Use 'install', 'remove', 'start', 'stop' to control service.")
		runService(serviceName, true)
	}
}

// readConfig lê o arquivo config.json.
func readConfig() (*Config, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	configPath := filepath.Join(filepath.Dir(exePath), "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json at '%s': %w", configPath, err)
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	if config.PapercutLogDir == "" {
		config.PapercutLogDir = "C:\\Program Files (x86)\\PaperCut Print Logger\\logs\\csv\\daily"
		globalLogger.Println("WARNING: papercutLogDir not set in config.json, using default: " + config.PapercutLogDir)
	}
	if config.ApiBaseURL == "" {
		config.ApiBaseURL = "http://localhost:3005" // Valor padrão - será substituído pelo config.json
		globalLogger.Println("WARNING: apiBaseUrl not set in config.json, using default: " + config.ApiBaseURL)
	}
	if config.PollingInterval == 0 {
		config.PollingInterval = 10
		globalLogger.Println("WARNING: pollingIntervalSeconds not set in config.json, using default: 10 seconds")
	}

	return &config, nil
}

// getPapercutLogPath constrói o caminho completo para o arquivo de log do dia atual.
func getPapercutLogPath(logDir string, t time.Time) string {
	// Formato: papercut-print-log-YYYY-MM-DD.csv
	fileName := fmt.Sprintf("papercut-print-log-%s.csv", t.Format("2006-01-02"))
	return filepath.Join(logDir, fileName)
}

// NOVO: tryProcessImpression tenta enviar uma impressão e retorna true em sucesso, false em falha recuperável
func tryProcessImpression(cfg *Config, data PrintData, sourceFile string) bool {
	verifyURL := cfg.ApiBaseURL + "/central/verifyimpression"
	exists, err := verifyImpressionExists(verifyURL, data)
	if err != nil {
		globalLogger.Println(fmt.Sprintf("API_COMM_FAIL (Verify) for user %s from source '%s'. Error: %v", data.Usuario, sourceFile, err))
		return false
	}

	if exists {
		globalLogger.Println(fmt.Sprintf("Impression for user %s from source '%s' already exists. Skipping.", data.Usuario, sourceFile))
		return true
	}

	sendURL := cfg.ApiBaseURL + "/central/receptprintreq"
	err = sendDataToAPI(sendURL, data)
	if err != nil {
		globalLogger.Println(fmt.Sprintf("API_COMM_FAIL (Send) for user %s from source '%s'. Error: %v", data.Usuario, sourceFile, err))
		return false
	}

	globalLogger.Println(fmt.Sprintf("Successfully sent print data for user %s from source '%s'.", data.Usuario, sourceFile))
	return true
}

// NOVO: setupPendingDir inicializa o diretório para armazenar impressões pendentes.
func setupPendingDir() error {
	logDir := filepath.Join(os.Getenv("PROGRAMDATA"), "PrintWatchServiceLogs")
	pendingDir = filepath.Join(logDir, "pending")
	if err := os.MkdirAll(pendingDir, 0755); err != nil {
		return fmt.Errorf("failed to create pending directory '%s': %w", pendingDir, err)
	}
	globalLogger.Println("Pending impressions directory initialized at:", pendingDir)
	return nil
}

// NOVO: savePendingImpression salva uma impressão falha na fila local.
func savePendingImpression(data PrintData) error {
	fileName := fmt.Sprintf("%d.json", time.Now().UnixNano())
	filePath := filepath.Join(pendingDir, fileName)

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal pending impression data: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write pending impression to file '%s': %w", filePath, err)
	}

	globalLogger.Println(fmt.Sprintf("Saved impression for user %s to pending queue: %s", data.Usuario, filePath))
	return nil
}

// NOVO: processPendingImpressions lê a fila local e tenta reenviar as impressões.
func processPendingImpressions(cfg *Config) {
	files, err := os.ReadDir(pendingDir)
	if err != nil {
		globalLogger.Println(fmt.Sprintf("ERROR: Could not read pending directory '%s': %v", pendingDir, err))
		return
	}

	if len(files) == 0 {
		globalLogger.Println("No pending impressions to process.")
		return
	}

	globalLogger.Println(fmt.Sprintf("Found %d pending impression(s) to process.", len(files)))

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(pendingDir, file.Name())
		fileData, err := os.ReadFile(filePath)
		if err != nil {
			globalLogger.Println(fmt.Sprintf("ERROR: Failed to read pending file '%s': %v", filePath, err))
			continue
		}

		var printData PrintData
		if err := json.Unmarshal(fileData, &printData); err != nil {
			globalLogger.Println(fmt.Sprintf("ERROR: Failed to unmarshal pending file '%s'. Deleting corrupt file. Error: %v", filePath, err))
			os.Remove(filePath) // Remove arquivo corrompido para não bloquear a fila
			continue
		}

		// Tenta processar a impressão da fila
		success := tryProcessImpression(cfg, printData, filePath)
		if success {
			// Se foi sucesso (enviado ou já existe), remove da fila
			globalLogger.Println(fmt.Sprintf("Successfully processed pending impression '%s'. Removing from queue.", filePath))
			if err := os.Remove(filePath); err != nil {
				globalLogger.Println(fmt.Sprintf("ERROR: Failed to remove processed pending file '%s': %v", filePath, err))
			}
		} else {
			// Se falhou, deixa na fila para a próxima tentativa
			globalLogger.Println(fmt.Sprintf("Failed to process pending impression '%s'. Will retry later.", filePath))
		}
	}
}

// processPapercutLogs lê novas linhas do log e as envia para a API.
func processPapercutLogs(cfg *Config) error {
	// Obtém o caminho do log para o dia atual
	papercutLogPath := getPapercutLogPath(cfg.PapercutLogDir, time.Now())

	file, err := os.OpenFile(papercutLogPath, os.O_RDONLY, 0644)
	if err != nil {
		// Se o arquivo do dia ainda não existe, não é um erro fatal, apenas ignora por enquanto.
		if os.IsNotExist(err) {
			globalLogger.Println(fmt.Sprintf("INFO: PaperCut log file for today (%s) does not exist yet. Skipping this cycle.", papercutLogPath))
			return nil
		}
		return fmt.Errorf("failed to open PaperCut log file '%s': %w", papercutLogPath, err)
	}
	defer file.Close()

	// Obtém o offset para o arquivo de log atual
	currentOffset, found := lastReadOffsets[papercutLogPath]
	if !found {
		// Se for a primeira vez que vemos este arquivo, o offset é 0
		currentOffset = 0
		globalLogger.Println(fmt.Sprintf("INFO: Starting to read new log file: %s from offset 0", papercutLogPath))
	}

	_, err = file.Seek(currentOffset, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to last offset %d in log file '%s': %w", currentOffset, papercutLogPath, err)
	}

	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // **NOVO:** Permite um número variável de campos por linha

	// Se o offset for 0 (novo arquivo ou primeira leitura), lê o cabeçalho.
	if currentOffset == 0 {
		_, err = reader.Read() // Ignora a linha do cabeçalho
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read CSV header from '%s': %w", papercutLogPath, err)
		}
	}

	// Processar linhas uma por uma
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break // Fim do arquivo
		}
		if err != nil {
			globalLogger.Println(fmt.Sprintf("WARNING: Failed to read CSV record from '%s', skipping: %v", papercutLogPath, err))
			continue
		}

		// Garante que o registro tenha colunas suficientes para os dados que você precisa
		// Com base no novo cabeçalho: Time,User,Pages,Copies,Printer,Document Name,Client,Paper Size,Language,Height,Width,Duplex,Grayscale,Size
		if len(record) < 14 {
			globalLogger.Println(fmt.Sprintf("WARNING: Skipping malformed record (not enough columns) from '%s': %v", papercutLogPath, record))
			continue
		}

		// --- Montar a estrutura de dados para a API com os novos campos ---
		// Tenta analisar o timestamp. Formato novo: "2006-01-02 15:04:05"
		parsedTime, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(record[0]))
		if err != nil {
			globalLogger.Println(fmt.Sprintf("WARNING: Could not parse timestamp '%s', skipping record: %v", record[0], err))
			continue
		}

		paginas, err := strconv.Atoi(strings.TrimSpace(record[2])) // Coluna "Pages"
		if err != nil {
			globalLogger.Println(fmt.Sprintf("WARNING: Could not parse pages '%s' to int, using 0: %v", record[2], err))
			paginas = 0 // Define 0 se a conversão falhar
		}

		copias, err := strconv.Atoi(strings.TrimSpace(record[3])) // Coluna "Copies"
		if err != nil {
			globalLogger.Println(fmt.Sprintf("WARNING: Could not parse copies '%s' to int, using 1: %v", record[3], err))
			copias = 1 // Define 1 se a conversão falhar
		}

		// NOVO: O campo "tipo" agora é a extensão do arquivo
		documentName := strings.TrimSpace(record[5])
		fileExtension := strings.TrimPrefix(filepath.Ext(documentName), ".")

		// --- Capturar informações de rede ---
		ip, mac, err := getNetworkInfo()
		if err != nil {
			globalLogger.Println(fmt.Sprintf("WARNING: Could not get network info: %v. IP and MAC will be empty.", err))
		}

		printData := PrintData{
			Data:        parsedTime.Format("2006-01-02"),
			Hora:        parsedTime.Format("15:04:05"),
			Usuario:     strings.TrimSpace(record[1]), // "User"
			Setor:       cfg.Setor,
			Paginas:     paginas,
			Copias:      copias,
			Impressora:  strings.TrimSpace(record[4]),  // "Printer"
			NomeArquivo: documentName,                  // "Document Name"
			Tipo:        fileExtension,                 // Extensão do arquivo (ex: "pdf")
			NomePC:      strings.TrimSpace(record[6]),  // "Client"
			TipoPage:    strings.TrimSpace(record[7]),  // "Paper Size"
			Cor:         strings.TrimSpace(record[12]), // CORRIGIDO: Usa o valor original de Grayscale (ex: "GRAYSCALE")
			Tamanho:     strings.TrimSpace(record[13]), // "Size"
			IP:          ip,                            // Capturado localmente
			MAC:         mac,                           // Capturado localmente
			IDEmpresa:   cfg.IDEmpresa,                 // NOVO: Adicionado do config
		}

		// ** ALTERADO: Tentar processar e, se falhar, colocar na fila **
		success := tryProcessImpression(cfg, printData, papercutLogPath)
		if !success {
			// A falha já foi logada dentro de tryProcessImpression. Agora, apenas enfileiramos.
			if err := savePendingImpression(printData); err != nil {
				// Este é um erro crítico, pois a fila não está funcionando.
				globalLogger.Println(fmt.Sprintf("CRITICAL_ERROR: FAILED TO SAVE PENDING IMPRESSION for user %s. Data may be lost. Error: %v", printData.Usuario, err))
			}
		}
	}

	// Atualizar o lastReadOffset para a posição atual do arquivo APENAS para o arquivo atual
	currentFileOffset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current file offset for '%s': %w", papercutLogPath, err)
	}
	lastReadOffsets[papercutLogPath] = currentFileOffset
	globalLogger.Println(fmt.Sprintf("Updated lastReadOffset for '%s' to: %d", papercutLogPath, currentFileOffset))

	return nil
}

// verifyImpressionExists verifica se uma impressão já existe na API enviando os dados completos.
func verifyImpressionExists(verifyApiEndpoint string, data PrintData) (bool, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSON data for verification: %w", err)
	}

	globalLogger.Println(fmt.Sprintf("Verifying impression existence at %s", verifyApiEndpoint))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(verifyApiEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, fmt.Errorf("failed to make HTTP POST request to verification endpoint %s: %w", verifyApiEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("verification API %s returned non-200 status: %d - %s", verifyApiEndpoint, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body from verification API: %w", err)
	}

	globalLogger.Println(fmt.Sprintf("Verification API raw response: %s", string(bodyBytes)))

	// NOVO: Parsear a resposta JSON da API de verificação
	var verifyResp struct {
		Status string `json:"status"`
	}
	err = json.Unmarshal(bodyBytes, &verifyResp)
	if err != nil {
		// Se a resposta não for um JSON válido, loga e assume que a impressão não existe para tentar enviar.
		globalLogger.Println(fmt.Sprintf("WARNING: Could not parse JSON from verification API, assuming impression does not exist. Error: %v", err))
		return false, nil
	}

	if verifyResp.Status == "true" {
		globalLogger.Println("Verification API returned status:true, impression exists.")
		return true, nil
	}

	// Qualquer outro valor de status (incluindo "false") significa que a impressão não existe.
	globalLogger.Println("Verification API returned status:false, impression does not exist.")
	return false, nil
}

// sendDataToAPI envia um payload JSON via HTTP POST.
func sendDataToAPI(apiEndpoint string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON data: %w", err)
	}

	globalLogger.Println(fmt.Sprintf("Sending data to API %s: %s", apiEndpoint, string(jsonData)))

	resp, err := http.Post(apiEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to make HTTP POST request to %s: %w", apiEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API %s returned non-200/201 status: %d - %s", apiEndpoint, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	globalLogger.Println(fmt.Sprintf("API response status for %s: %s", apiEndpoint, resp.Status))
	return nil
}

// getNetworkInfo encontra o primeiro endereço IPv4 e MAC de uma interface de rede ativa.
func getNetworkInfo() (string, string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "", err
	}

	for _, iface := range interfaces {
		// Pula interfaces que estão "down" ou são de loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Ignora interfaces virtuais comuns para evitar IPs incorretos
		if strings.Contains(strings.ToLower(iface.Name), "virtual") || strings.Contains(strings.ToLower(iface.Name), "vmware") || strings.Contains(strings.ToLower(iface.Name), "hyper-v") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Converte para IPv4. Se for nil, não é um endereço IPv4.
			ip = ip.To4()
			if ip == nil {
				continue
			}

			// Retorna o primeiro IPv4 válido encontrado e o MAC correspondente
			mac := iface.HardwareAddr.String()
			if mac != "" {
				return ip.String(), mac, nil
			}
		}
	}

	return "", "", fmt.Errorf("no suitable network interface found")
}
