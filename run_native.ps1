# run_native.ps1
Write-Host "Iniciando o Backend (Golang)..." -ForegroundColor Green
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd backend; go run cmd/api/main.go"

Write-Host "Iniciando o Frontend (Next.js)..." -ForegroundColor Blue
cd frontend
npm run dev
