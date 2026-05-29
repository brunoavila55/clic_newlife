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
	// 1. Get session token first
	sessionToken, err := uc.mkService.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	// 2. Fetch main client data from MK API
	client, err := uc.mkService.FetchClientByCPF(ctx, sessionToken, cpf)
	if err != nil {
		return nil, err
	}

	// 3. Fetch dependencies concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	aggregated := &domain.ClientAggregatedData{
		Client: *client,
	}

	// We are now fetching 5 parallel things
	wg.Add(5)

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

	// wait with timeout
	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		// all done
	case <-time.After(8 * time.Second):
		// timeout reached
	}

	return aggregated, nil
}
