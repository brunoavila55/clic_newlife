package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/clic_newlife/backend/internal/config"
	"github.com/clic_newlife/backend/internal/infrastructure/integration"
)

func main() {
	cfg := config.LoadConfig()
	s := integration.NewMKIntegrationService(cfg)
	ctx := context.Background()
	token, _ := s.Authenticate(ctx)

	// Fetch Conexoes
	url := fmt.Sprintf("%s/mk/WSMKConexoesPorCliente.rule?sys=MK0&token=%s&cd_cliente=13198", cfg.MKApiURL, token)
	req, _ := http.NewRequest("GET", url, nil)
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	fmt.Println("Conexoes:", string(b))

	// Fetch Faturas
	url2 := fmt.Sprintf("%s/mk/WSMKFaturas.rule?sys=MK0&token=%s&cd_cliente=13198", cfg.MKApiURL, token)
	req2, _ := http.NewRequest("GET", url2, nil)
	resp2, _ := http.DefaultClient.Do(req2)
	b2, _ := io.ReadAll(resp2.Body)
	fmt.Println("Faturas:", string(b2))
}
