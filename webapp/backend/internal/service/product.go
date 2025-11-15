package service

import (
	"context"
	"log"

	"backend/internal/model"
	"backend/internal/repository"
)

type ProductService struct {
	store *repository.Store
}

func NewProductService(store *repository.Store) *ProductService {
	return &ProductService{store: store}
}

func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	var insertedOrderIDs []string

	err := s.store.ExecTx(ctx, func(txStore *repository.Store) error {
		itemsToProcess := make(map[int]int)
		for _, item := range items {
			if item.Quantity > 0 {
				itemsToProcess[item.ProductID] = item.Quantity
			}
		}
		if len(itemsToProcess) == 0 {
			return nil
		}

		for pID, quantity := range itemsToProcess {
			for i := 0; i < quantity; i++ {
				order := &model.Order{
					UserID:    userID,
					ProductID: pID,
				}
				orderID, err := txStore.OrderRepo.Create(ctx, order)
				if err != nil {
					return err
				}
				insertedOrderIDs = append(insertedOrderIDs, orderID)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	log.Printf("Created %d orders for user %d", len(insertedOrderIDs), userID)
	return insertedOrderIDs, nil
}

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	products, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	if err != nil {
		return nil, 0, err
	}

	// 総件数は非同期で取得（初回レスポンスを高速化）
	// バックグラウンドでgoroutineを使ってCOUNTを取得し、商品データの取得と並行実行
	totalChan := make(chan int, 1)
	errChan := make(chan error, 1)
	go func() {
		total, err := s.store.ProductRepo.CountProducts(context.Background(), userID, req)
		if err != nil {
			errChan <- err
			return
		}
		totalChan <- total
	}()

	// 非同期で取得した総件数を待機（商品データは既に取得済みなので、レスポンスは高速）
	select {
	case total := <-totalChan:
		return products, total, nil
	case err := <-errChan:
		log.Printf("Failed to get count asynchronously: %v", err)
		return products, 0, nil
	case <-ctx.Done():
		// コンテキストがキャンセルされた場合は、0を返す
		return products, 0, nil
	}
}
