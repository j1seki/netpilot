; Inno Setup script for NetPilot
; Bundles NetPilot GUI + net-merge engine + net-merge-tray + wintun driver
#ifndef Version
  #define Version "0.1.0"
#endif

[Setup]
AppId={{9C1E8F2A-5B3D-4E7A-9C41-2D6B8E0F7B24}
AppName=NetPilot
AppVersion={#Version}
AppVerName=NetPilot {#Version}
AppPublisher=NetPilot Project
#ifdef RepoUrl
AppPublisherURL={#RepoUrl}
AppSupportURL={#RepoUrl}/issues
AppUpdatesURL={#RepoUrl}/releases
#endif
VersionInfoVersion={#Version}
DefaultDirName={autopf}\NetPilot
DefaultGroupName=NetPilot
DisableProgramGroupPage=yes
; The TUN adapter and routes need admin, and so does writing to Program Files.
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.17763
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
LicenseFile=LICENSE
SourceDir=..
OutputDir=dist
OutputBaseFilename=netpilot-setup-{#Version}
UninstallDisplayIcon={app}\netpilot.ico
CloseApplications=no
UsedUserAreasWarning=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Shortcuts:"; Flags: unchecked

[Files]
Source: "dist\netpilot.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\net-merge.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\net-merge-tray.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\wintun.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\wintun-LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "dist\netpilot.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "THIRD-PARTY-NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "README.md"; DestDir: "{app}"; Flags: ignoreversion isreadme

[Icons]
Name: "{autoprograms}\NetPilot"; Filename: "{app}\netpilot.exe"; IconFilename: "{app}\netpilot.ico"; Comment: "NetPilot - Multi-Network Manager & Bandwidth Aggregator"
Name: "{autodesktop}\NetPilot"; Filename: "{app}\netpilot.exe"; IconFilename: "{app}\netpilot.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\netpilot.exe"; Description: "Launch NetPilot now"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\net-merge.exe"; Parameters: "quit"; Flags: runhidden; RunOnceId: "StopEngine"

[UninstallDelete]
Type: filesandordirs; Name: "{localappdata}\net-merge"
Type: filesandordirs; Name: "{localappdata}\netpilot"

[Code]
procedure StopNetMerge;
var
  rc: Integer;
begin
  if FileExists(ExpandConstant('{app}\net-merge.exe')) then
    Exec(ExpandConstant('{app}\net-merge.exe'), 'quit', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM netpilot.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM net-merge-tray.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM net-merge.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  StopNetMerge;
  Result := '';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    StopNetMerge;
end;
