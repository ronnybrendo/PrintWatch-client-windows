# PrintWatch Client - Windows

Cliente Windows para monitoramento de impressões usando PaperCut Print Logger.

## 📋 Índice

- [Visão Geral](#visão-geral)
- [Requisitos](#requisitos)
- [Instalação](#instalação)
- [Configuração](#configuração)
- [Uso](#uso)
- [Estrutura do Projeto](#estrutura-do-projeto)
- [Troubleshooting](#troubleshooting)
- [Desenvolvimento](#desenvolvimento)

## 🎯 Visão Geral

O PrintWatch Client é um serviço Windows desenvolvido em Go que monitora automaticamente os logs do PaperCut Print Logger e envia os dados de impressão para uma API centralizada. O sistema inclui:

- **Monitoramento automático** dos logs do PaperCut
- **Verificação de duplicatas** antes do envio
- **Sistema de fila** para impressões pendentes
- **Logs detalhados** para auditoria
- **Instalação simplificada** via instalador

## ⚙️ Requisitos

### Sistema
- Windows 10/11 ou Windows Server 2016+
- .NET Framework 4.5+ (para o instalador)
- PaperCut Print Logger instalado e configurado

### Software
- **PaperCut Print Logger**: Deve estar configurado e gerando logs CSV
- **API PrintWatch**: Servidor da API deve estar acessível

## 🚀 Instalação

### Método 1: Instalador Automático (Recomendado)

#### Gerando o Instalador

1. **Instale o Inno Setup Compiler**
   - Download: https://jrsoftware.org/isdl.php
   - Instale a versão mais recente

2. **Compile o instalador**
   ```cmd
   # Navegue até a pasta do projeto
   cd PrintWacth-client-windows
   
   # Compile usando o Inno Setup
   iscc installer.iss
   ```

3. **Arquivo gerado**
   - O instalador será criado: `PrintWatch_Installer_Go_Installer.exe`

#### Usando o Instalador

1. **Execute o instalador**
   - Clique duas vezes no arquivo `.exe` gerado
   - Siga o assistente de instalação

2. **Configure durante a instalação**
   - **Setor**: Nome do setor/departamento
   - **ID da Empresa**: Identificador numérico da empresa
   - **URL da API**: Endereço da API (ex: `http://localhost:3005`)

3. **Instalação automática**
   - O instalador copia os arquivos necessários
   - Instala o serviço Windows automaticamente
   - Inicia o serviço

### Método 2: Instalação Manual

1. **Compile o projeto**
   ```bash
   go build -o PrintWatchService.exe main.go
   ```

2. **Crie o arquivo de configuração**
   ```json
   {
     "setor": "CPD",
     "idEmpresa": 2,
     "apiBaseUrl": "http://localhost:3005"
   }
   ```

3. **Instale o serviço**
   ```cmd
   PrintWatchService.exe install
   PrintWatchService.exe start
   ```

## ⚙️ Configuração

### Arquivo config.json

O arquivo `config.json` deve estar no mesmo diretório do executável:

```json
{
  "setor": "CPD",
  "idEmpresa": 2,
  "papercutLogDir": "C:\\Program Files (x86)\\PaperCut Print Logger\\logs\\csv\\daily",
  "apiBaseUrl": "http://localhost:3005",
  "pollingIntervalSeconds": 10
}
```

### Parâmetros de Configuração

| Campo | Descrição | Padrão |
|-------|-----------|---------|
| `setor` | Nome do setor/departamento | - |
| `idEmpresa` | ID numérico da empresa | - |
| `papercutLogDir` | Diretório dos logs do PaperCut | `C:\Program Files (x86)\PaperCut Print Logger\logs\csv\daily` |
| `apiBaseUrl` | URL base da API | `http://localhost:3005` |
| `pollingIntervalSeconds` | Intervalo de verificação (segundos) | `10` |

### Configuração do PaperCut

1. **Verifique o diretório de logs**
   - Padrão: `C:\Program Files (x86)\PaperCut Print Logger\logs\csv\daily`
   - Arquivos: `papercut-print-log-YYYY-MM-DD.csv`

2. **Formato esperado dos logs**
   ```
   Time,User,Pages,Copies,Printer,Document Name,Client,Paper Size,Language,Height,Width,Duplex,Grayscale,Size
   ```

## 🔧 Uso

### Comandos do Serviço

```cmd
# Instalar o serviço
PrintWatchService.exe install

# Iniciar o serviço
PrintWatchService.exe start

# Parar o serviço
PrintWatchService.exe stop

# Remover o serviço
PrintWatchService.exe remove

# Modo debug (console)
PrintWatchService.exe
```

### Verificação do Status

1. **Serviços Windows**
   - Abra "Serviços" (services.msc)
   - Procure por "PrintWatch Service"
   - Status deve ser "Em execução"

2. **Logs do Sistema**
   - Localização: `C:\ProgramData\PrintWatchServiceLogs\`
   - Arquivo: `printwatch_service.log`
   - Diretório de pendências: `pending\`

### Monitoramento

O serviço executa automaticamente:

1. **Ao iniciar**: Processa impressões pendentes
2. **A cada ciclo**: Monitora novos logs do PaperCut
3. **Verificação**: Confirma se a impressão já existe na API
4. **Envio**: Transmite dados para a API
5. **Fila**: Salva impressões falhadas para retry

## 📁 Estrutura do Projeto

```
PrintWacth-client-windows/
├── main.go                 # Código principal do serviço
├── installer.iss          # Script do instalador
├── go.mod                 # Dependências Go
├── go.sum                 # Checksums das dependências
├── LICENSE                # Licença MIT
├── README.md             # Este arquivo
└── .gitignore            # Arquivos ignorados pelo Git
```

### Principais Componentes

- **`main.go`**: Lógica principal do serviço Windows
- **`installer.iss`**: Script do instalador Inno Setup
- **Configuração**: Leitura do `config.json`
- **Logs**: Sistema de logging em arquivo
- **API**: Comunicação HTTP com o servidor
- **Fila**: Sistema de impressões pendentes

## 🔍 Troubleshooting

### Problemas Comuns

#### 1. Serviço não inicia
```
Erro: "Failed to read config.json"
```
**Solução**: Verifique se o arquivo `config.json` existe e está válido

#### 2. API não acessível
```
Erro: "API_COMM_FAIL"
```
**Solução**: 
- Verifique a conectividade de rede
- Confirme a URL da API no `config.json`
- Teste a API manualmente

#### 3. Logs do PaperCut não encontrados
```
Erro: "PaperCut log file does not exist"
```
**Solução**:
- Verifique o caminho em `papercutLogDir`
- Confirme se o PaperCut está gerando logs
- Aguarde a criação do arquivo do dia

#### 4. Impressões não sendo enviadas
```
Log: "No pending impressions to process"
```
**Solução**:
- Verifique se há novos logs do PaperCut
- Confirme a conectividade com a API
- Verifique os logs de erro

### Logs de Debug

Para ativar logs detalhados:

1. **Modo debug**
   ```cmd
   PrintWatchService.exe
   ```

2. **Logs do sistema**
   - Event Viewer → Windows Logs → Application
   - Procure por eventos do "PrintWatch"

3. **Logs do arquivo**
   - Localização: `C:\ProgramData\PrintWatchServiceLogs\printwatch_service.log`

## 🛠️ Desenvolvimento

### Ambiente de Desenvolvimento

1. **Instale o Go**
   ```bash
   # Versão mínima: 1.24.5
   go version
   ```

2. **Clone o repositório**
   ```bash
   git clone <repository-url>
   cd PrintWacth-client-windows
   ```

3. **Instale dependências**
   ```bash
   go mod tidy
   ```

4. **Compile**
   ```bash
   go build -o PrintWatchService.exe main.go
   ```

### Estrutura do Código

- **`main()`**: Ponto de entrada e controle de comandos
- **`myservice.Execute()`**: Lógica principal do serviço
- **`readConfig()`**: Leitura da configuração
- **`processPapercutLogs()`**: Processamento dos logs
- **`tryProcessImpression()`**: Envio para API
- **`processPendingImpressions()`**: Fila de pendências

### Testes

```bash
# Teste de compilação
go build

# Teste de execução (modo debug)
./PrintWatchService.exe

# Teste de instalação
./PrintWatchService.exe install
./PrintWatchService.exe start
```

### Build do Instalador

1. **Instale o Inno Setup**
   - Download: https://jrsoftware.org/isdl.php

2. **Compile o instalador**
   ```cmd
   iscc installer.iss
   ```

3. **Arquivo gerado**
   - `PrintWatch_Installer_Go_Installer.exe`

## 📄 Licença

Este projeto está licenciado sob a Licença MIT - veja o arquivo [LICENSE](LICENSE) para detalhes.

## 🤝 Contribuição

1. Fork o projeto
2. Crie uma branch para sua feature (`git checkout -b feature/AmazingFeature`)
3. Commit suas mudanças (`git commit -m 'Add some AmazingFeature'`)
4. Push para a branch (`git push origin feature/AmazingFeature`)
5. Abra um Pull Request

## 📞 Suporte

Para suporte técnico ou dúvidas:

- **Issues**: Abra uma issue no GitHub
- **Documentação**: Consulte este README
- **Logs**: Verifique os logs em `C:\ProgramData\PrintWatchServiceLogs\`

---

**Desenvolvido por Ronnybrendo**  
**Versão**: 1.0.0  
**Última atualização**: 2025
