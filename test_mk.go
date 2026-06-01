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

	url2 := fmt.Sprintf("%s/mk/WSMKPlanosAcesso.rule?sys=MK0&token=%s&codigo_plano=1089&cancelado=S&suspenso=S&aguarda_ativacao=S", cfg.MKApiURL, token)
	req2, _ := http.NewRequest("GET", url2, nil)
	resp2, _ := http.DefaultClient.Do(req2)
	b2, _ := io.ReadAll(resp2.Body)
	fmt.Println("WSMKPlanosAcesso.rule:", string(b2))
}
