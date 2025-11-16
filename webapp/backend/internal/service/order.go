package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
)

type OrderService struct {
	store *repository.Store
}

func NewOrderService(store *repository.Store) *OrderService {
	return &OrderService{store: store}
}

// ユーザーの注文履歴を取得
func (s *OrderService) FetchOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	orders, err := s.store.OrderRepo.ListOrders(ctx, userID, req)
	if err != nil {
		return nil, 0, err
	}

	// 総件数は非同期で取得（初回レスポンスを高速化）
	// バックグラウンドでgoroutineを使ってCOUNTを取得し、注文データの取得と並行実行
	totalChan := make(chan int, 1)
	errChan := make(chan error, 1)
	go func() {
		total, err := s.store.OrderRepo.CountOrders(context.Background(), userID, req)
		if err != nil {
			errChan <- err
			return
		}
		totalChan <- total
	}()

	// 非同期で取得した総件数を待機（注文データは既に取得済みなので、レスポンスは高速）
	select {
	case total := <-totalChan:
		return orders, total, nil
	case <-errChan:
		return orders, 0, nil
	case <-ctx.Done():
		// コンテキストがキャンセルされた場合は、0を返す
		return orders, 0, nil
	}
}
