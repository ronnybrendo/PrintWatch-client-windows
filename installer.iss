; PrintWatchInstaller.iss - Script de instalação para o PrintWatch

[Setup]
AppName=PrintWatch
AppVersion=1.0.0
AppPublisher=NOVO TEMPO - CPD
DefaultDirName={pf}\PrintWatch
DefaultGroupName=PrintWatch
AllowNoIcons=yes
OutputBaseFilename=PrintWatch_Installer_Go_Installer
Compression=lzma
SolidCompression=yes
WizardStyle=modern
; --- NOVAS DIRETIVAS DE PERSONALIZAÇÃO ---
//WizardImageFile=Source\logo_grande.bmp
//WizardSmallImageFile=Source\logo_pequeno.bmp
//LicenseFile=Source\licenca.txt
//SetupIconFile=Source\icone_app.ico
UninstallDisplayIcon={app}\PrintWatchService.exe

[CustomMessages]
CustomMessage_ConfigPage_Caption=Configuração do PrintWatch
CustomMessage_ConfigPage_Description=Por favor, insira as informações de configuração para o PrintWatch:

; --- NOVA SEÇÃO PARA MENSAGENS PERSONALIZADAS ---
[Messages]
WelcomeLabel1=Bem-vindo ao Assistente de Instalação do [name]!
SetupAppTitle=Instalando o PrintWatch
BeveledLabel=Este assistente instalará o PrintWatch em seu computador. Recomendamos que você feche todos os outros aplicativos antes de continuar.

[Dirs]
Name: "{app}"
; Não precisa mais da pasta 'daemon' para node-windows

[Files]
; Copia o executável do seu serviço Go
; ATENÇÃO: SUBSTITUA "caminho\para\PrintWatchService.exe" pelo caminho REAL do seu binário compilado.
Source: "PrintWatchService.exe"; DestDir: "{app}"; Flags: ignoreversion

; --- Página de entrada de dados (Setor e ID da Empresa) ---
[Code]
var
  ConfigPage: TWizardPage;
  SetorEdit: TNewEdit;
  IdEmpresaEdit: TNewEdit;

procedure InitializeWizard;
begin
  ConfigPage := CreateCustomPage(
    wpWelcome,
    CustomMessage('CustomMessage_ConfigPage_Caption'),
    CustomMessage('CustomMessage_ConfigPage_Description')
  );

  with TNewStaticText.Create(ConfigPage) do
  begin
    Parent := ConfigPage.Surface;
    Caption := 'Setor:';
    Left := ScaleX(0);
    Top := ScaleY(5);
    Width := ScaleX(80);
    Height := ScaleY(15);
  end;

  SetorEdit := TNewEdit.Create(ConfigPage);
  with SetorEdit do
  begin
    Parent := ConfigPage.Surface;
    Left := ScaleX(90);
    Top := ScaleY(5);
    Width := ScaleX(200);
    Height := ScaleY(21);
  end;

  with TNewStaticText.Create(ConfigPage) do
  begin
    Parent := ConfigPage.Surface;
    Caption := 'ID da Empresa:';
    Left := ScaleX(0);
    Top := ScaleY(35);
    Width := ScaleX(80);
    Height := ScaleY(15);
  end;

  IdEmpresaEdit := TNewEdit.Create(ConfigPage);
  with IdEmpresaEdit do
  begin
    Parent := ConfigPage.Surface;
    Left := ScaleX(90);
    Top := ScaleY(35);
    Width := ScaleX(200);
    Height := ScaleY(21);
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  FileName: string;
  JsonContentArray: Array of string;
begin
  if CurStep = ssPostInstall then
  begin
    FileName := ExpandConstant('{app}\config.json');

    SetArrayLength(JsonContentArray, 4);
    JsonContentArray[0] := '{';
    JsonContentArray[1] := '  "setor": "' + SetorEdit.Text + '",';
    JsonContentArray[2] := '  "idEmpresa": ' + IdEmpresaEdit.Text;
    JsonContentArray[3] := '}';

    if not SaveStringsToFile(FileName, JsonContentArray, False) then
    begin
      MsgBox('Erro ao criar ou gravar config.json.', mbError, MB_OK);
    end;
  end;
end;

[Icons]
Name: "{group}\PrintWatch"; Filename: "{app}\PrintWatchService.exe"
Name: "{group}\Uninstall PrintWatch"; Filename: "{uninstallexe}"

[Run]
; Instala o serviço Go (ele mesmo se instala/desinstala)
; O "WorkingDir" deve ser a pasta onde o executável está
Filename: "{app}\PrintWatchService.exe"; Parameters: "install"; WorkingDir: "{app}"; StatusMsg: "Instalando serviço PrintWatch..."; Flags: runhidden
; Inicia o serviço
Filename: "{app}\PrintWatchService.exe"; Parameters: "start"; WorkingDir: "{app}"; StatusMsg: "Iniciando serviço PrintWatch..."; Flags: runhidden

[UninstallRun]
; Para o serviço
Filename: "{app}\PrintWatchService.exe"; Parameters: "stop"; WorkingDir: "{app}"; StatusMsg: "Parando serviço PrintWatch..."; Flags: runhidden
; Remove o serviço
Filename: "{app}\PrintWatchService.exe"; Parameters: "remove"; WorkingDir: "{app}"; StatusMsg: "Desinstalando serviço PrintWatch..."; Flags: runhidden

[UninstallDelete]
; Remove o executável do serviço Go e o config.json
Type: files; Name: "{app}\PrintWatchService.exe"
Type: files; Name: "{app}\config.json"
; Se o seu serviço Go criar pasta de logs, adicione aqui
; Type: filesandordirs; Name: "{commonappdata}\PrintWatchServiceLogs"