# Supervisor del bot (Windows): compila bot\bin\bot.exe y lo vuelve a lanzar cada vez que termina.
# Si el bot está caído más de 30 s se pierde la partida por ausencia: este script lo evita.
#
# Uso, desde cualquier carpeta:   powershell -ExecutionPolicy Bypass -File scripts\run-bot.ps1
# Variables: las del entorno y, si existe, las de .env en la raíz (ver .env.example). Nunca se imprimen.
# Ctrl+C detiene el bot y el supervisor.

$root = Split-Path -Parent $PSScriptRoot # raíz del repositorio
$bin = Join-Path $root 'bot\bin\bot.exe'

$envFile = Join-Path $root '.env'
if (Test-Path $envFile) {
    foreach ($line in Get-Content $envFile) {
        $line = $line.Trim()
        if ($line -eq '' -or $line.StartsWith('#') -or -not $line.Contains('=')) { continue }
        $name, $value = $line.Split('=', 2)
        # Solo para este proceso y sus hijos: no modifica las variables del sistema.
        [Environment]::SetEnvironmentVariable($name.Trim(), $value.Trim(), 'Process')
    }
}

Write-Host "supervisor: compilando $bin"
Push-Location (Join-Path $root 'bot')
go build -o $bin ./cmd/bot
$buildCode = $LASTEXITCODE
Pop-Location
if ($buildCode -ne 0) { exit 1 }

$restarts = 0
while ($true) {
    # El bot corre en la misma consola: Ctrl+C lo detiene a él y también corta este script.
    & $bin
    $code = $LASTEXITCODE
    $restarts++
    Write-Host "$(Get-Date -Format s) supervisor: el bot terminó con código $code; reinicio #$restarts en 1 s"
    Start-Sleep -Seconds 1
}
