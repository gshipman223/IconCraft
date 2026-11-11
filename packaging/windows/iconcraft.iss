; Inno Setup scaffolding for IconCraft. This file is not compiled in CI
; and nothing here is code-signed. The owner decides on signing.
; Build with: iscc packaging\windows\iconcraft.iss
; after scripts\build_ports.sh has produced the amd64 exe.

#define MyAppName "IconCraft"
#define MyAppVersion "0.14.1"
#define MyAppPublisher "IconCraft"
#define MyAppURL "https://github.com/gshipman223/IconCraft"
#define MyAppExeName "IconCraft.exe"

[Setup]
AppId={{A7C3E1B2-4F58-4C1A-9B0E-6D2F8A1C4E70}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir=..\..\dist\windows
OutputBaseFilename=IconCraft-windows-amd64-{#MyAppVersion}-setup
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "..\..\build\iconcraft-windows-amd64.exe"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional icons:"

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Flags: nowait postinstall skipifsilent
