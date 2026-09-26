# 在 Windows 上安装 X Console 代理，登录时自动启动。
# 用法（普通 PowerShell，不需要管理员）：
#   .\install-agent-windows.ps1 -Server https://console.example.com -Code ABCD-EFGH -Binary .\x-console-agent.exe
param(
    [Parameter(Mandatory = $true)][string]$Server,
    [Parameter(Mandatory = $true)][string]$Code,
    [string]$Binary = ".\x-console-agent.exe"
)
$ErrorActionPreference = "Stop"
$dir = Join-Path $env:LOCALAPPDATA "x-console-agent"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$exe = Join-Path $dir "x-console-agent.exe"
Copy-Item $Binary $exe -Force

& $exe pair --server $Server --code $Code
if ($LASTEXITCODE -ne 0) { throw "配对失败" }

# 用任务计划程序在登录时启动。它跑在你的用户会话里，能用剪贴板、git 凭据和 Claude Code 登录状态。
$action = New-ScheduledTaskAction -Execute $exe -Argument "run" -WorkingDirectory $dir
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit 0 -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName "X Console Agent" -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName "X Console Agent"
Write-Host "已安装并启动。任务名：X Console Agent"
