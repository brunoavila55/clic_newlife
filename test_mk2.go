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

	// Try without codigo_plano
	url := fmt.Sprintf("%s/mk/WSMKPlanosAcesso.rule?sys=MK0&token=%s&cancelado=S&suspenso=S&aguarda_ativacao=S", cfg.MKApiURL, token)
	req, _ := http.NewRequest("GET", url, nil)
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	fmt.Println("No codigo_plano:", string(b)[:min(200, len(b))])

	// Try to get all planos? Maybe WSMKPlanos.rule exists?
	url2 := fmt.Sprintf("%s/mk/WSMKPlanos.rule?sys=MK0&token=%s", cfg.MKApiURL, token)
	req2, _ := http.NewRequest("GET", url2, nil)
	resp2, _ := http.DefaultClient.Do(req2)
	b2, _ := io.ReadAll(resp2.Body)
	fmt.Println("WSMKPlanos.rule:", string(b2)[:min(200, len(b2))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
