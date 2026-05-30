package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/clic_newlife/backend/internal/domain"
	"github.com/clic_newlife/backend/internal/infrastructure/integration"
)

type FetchClientDataUseCase struct {
	mkService *integration.MKIntegrationService
}

func NewFetchClientDataUseCase(mkService *integration.MKIntegrationService) *FetchClientDataUseCase {
	return &FetchClientDataUseCase{
		mkService: mkService,
	}
}

func (uc *FetchClientDataUseCase) Execute(ctx context.Context, cpf string) (*domain.ClientAggregatedData, error) {
	// 1. Obtém token de sessão (reutiliza do cache se disponível)
	sessionToken, err := uc.mkService.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Busca dados cadastrais do cliente (necessário para obter o InternalID)
	client, err := uc.mkService.FetchClientByCPF(ctx, sessionToken, cpf)
	if err != nil {
		return nil, err
	}

	// 3. Busca todos os dados dependentes em paralelo (6 goroutines simultâneas)
	var wg sync.WaitGroup
	var mu sync.Mutex

	aggregated := &domain.ClientAggregatedData{
		Client: *client,
	}

	wg.Add(6)

	go func() {
		defer wg.Done()
		conexoes, err := uc.mkService.FetchConexoes(ctx, sessionToken, client.InternalID)
		if err != nil {
			fmt.Println("Error FetchConexoes:", err)
		}
		mu.Lock()
		aggregated.Conexoes = conexoes
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		atendimentos, _ := uc.mkService.FetchAtendimentos(ctx, sessionToken, client.InternalID)
		mu.Lock()
		aggregated.Atendimentos = atendimentos
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		faturas, _ := uc.mkService.FetchFaturas(ctx, sessionToken, client.InternalID)
		mu.Lock()
		aggregated.Faturas = faturas
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		financeiro, _ := uc.mkService.FetchFinanceiro(ctx, sessionToken, client.InternalID)
		if financeiro != nil {
			mu.Lock()
			aggregated.Financeiro = *financeiro
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		contratos, _ := uc.mkService.FetchContratos(ctx, sessionToken, client.InternalID)
		mu.Lock()
		aggregated.Contratos = contratos
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		equipamentos, err := uc.mkService.FetchEquipamentos(ctx, sessionToken, client.InternalID)
		if err != nil {
			fmt.Println("Error FetchEquipamentos:", err)
		}
		mu.Lock()
		aggregated.Equipamentos = equipamentos
		mu.Unlock()
	}()

	// Aguarda todas as goroutines com timeout global de 12 segundos
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// tudo concluído
	case <-time.After(12 * time.Second):
		fmt.Println("Warning: timeout atingido aguardando APIs MK Solutions")
	}

	return aggregated, nil
}
