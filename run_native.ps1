# run_native.ps1
Write-Host "Iniciando o Backend (Golang)..." -ForegroundColor Green
Start-Process powershell -ArgumentList "-NoExit", "-Command", "go run cmd/api/main.go"
