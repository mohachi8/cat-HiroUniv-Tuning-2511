package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"
)

type RobotService struct {
	store *repository.Store
}

func NewRobotService(store *repository.Store) *RobotService {
	return &RobotService{store: store}
}

// 注意：このメソッドは、現在、ordersテーブルのshipped_statusが"shipping"になっている注文"全件"を対象に配送計画を立てます。
// 注文の取得件数を制限した場合、ペナルティの対象になります。
func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	var plan model.DeliveryPlan

	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx)
			if err != nil {
				return err
			}
			plan, err = selectOrdersForDelivery(ctx, orders, robotID, capacity)
			if err != nil {
				return err
			}
			if len(plan.Orders) > 0 {
				orderIDs := make([]int64, len(plan.Orders))
				for i, order := range plan.Orders {
					orderIDs[i] = order.OrderID
				}

				if err := txStore.OrderRepo.UpdateStatuses(ctx, orderIDs, "delivering"); err != nil {
					return err
				}
				log.Printf("Updated status to 'delivering' for %d orders", len(orderIDs))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	return utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus)
	})
}

// selectOrdersForDelivery は動的計画法（DP）を使用して0/1ナップザック問題を解きます
// 時間計算量: O(n * capacity) - DFSのO(2^n)から大幅に改善
// 空間計算量: O(n * capacity) - DPテーブル
func selectOrdersForDelivery(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	n := len(orders)
	if n == 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  0,
			Orders:      []model.Order{},
		}, nil
	}

	// 容量が大きすぎる場合は、メモリ効率を考慮した実装にフォールバック
	// ただし、通常の容量範囲ではDPが高速
	const maxCapacityForDP = 100000
	if robotCapacity > maxCapacityForDP {
		return selectOrdersForDeliveryDFS(ctx, orders, robotID, robotCapacity)
	}

	// DPテーブル: dp[i][w] = i番目までの注文で容量w以下の最大価値
	// メモリ効率のため、2行のみ保持（現在行と前の行）
	dp := make([][]int, 2)
	dp[0] = make([]int, robotCapacity+1)
	dp[1] = make([]int, robotCapacity+1)

	// 復元用: 各容量でその注文を選んだかどうかを記録
	// choice[i][w] = true なら、i番目の注文を容量wで選んだ
	choice := make([][]bool, n)
	for i := range choice {
		choice[i] = make([]bool, robotCapacity+1)
	}

	// 各注文を処理
	for i := 0; i < n; i++ {
		// コンテキストキャンセレーションのチェック
		select {
		case <-ctx.Done():
			return model.DeliveryPlan{}, ctx.Err()
		default:
		}

		order := orders[i]
		current := i % 2
		prev := 1 - current

		// 前の行をコピー（現在の注文を選ばない場合）
		copy(dp[current], dp[prev])

		// 現在の注文を選ぶ場合を考慮
		for w := order.Weight; w <= robotCapacity; w++ {
			// 現在の注文を選んだ場合の価値
			valueWithOrder := dp[prev][w-order.Weight] + order.Value
			// 現在の注文を選ばない場合の価値
			valueWithoutOrder := dp[prev][w]
			// より大きい方を選択
			if valueWithOrder > valueWithoutOrder {
				dp[current][w] = valueWithOrder
				choice[i][w] = true
			} else {
				dp[current][w] = valueWithoutOrder
				choice[i][w] = false
			}
		}
	}

	// 最適解の価値を取得
	lastRow := (n - 1) % 2
	bestValue := dp[lastRow][robotCapacity]

	// 最適解の復元: どの注文を選んだかを逆算
	bestSet := make([]model.Order, 0, n)
	w := robotCapacity
	for i := n - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return model.DeliveryPlan{}, ctx.Err()
		default:
		}

		if choice[i][w] {
			bestSet = append(bestSet, orders[i])
			w -= orders[i].Weight
		}
	}

	// 注文を元の順序に戻す（必要に応じて）
	// ここでは逆順になっているので、必要ならreverseする
	// ただし、順序は結果に影響しないのでそのままでも可

	var totalWeight int
	for _, o := range bestSet {
		totalWeight += o.Weight
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      bestSet,
	}, nil
}

// selectOrdersForDeliveryDFS は容量が大きすぎる場合のフォールバック実装
// 元のDFS実装を保持（メモリ効率を優先）
func selectOrdersForDeliveryDFS(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	n := len(orders)
	bestValue := 0
	var bestSet []model.Order
	steps := 0
	checkEvery := 16384

	var dfs func(i, curWeight, curValue int, curSet []model.Order) bool
	dfs = func(i, curWeight, curValue int, curSet []model.Order) bool {
		if curWeight > robotCapacity {
			return false
		}
		steps++
		if checkEvery > 0 && steps%checkEvery == 0 {
			select {
			case <-ctx.Done():
				return true
			default:
			}
		}
		if i == n {
			if curValue > bestValue {
				bestValue = curValue
				bestSet = append([]model.Order{}, curSet...)
			}
			return false
		}

		if dfs(i+1, curWeight, curValue, curSet) {
			return true
		}

		order := orders[i]
		return dfs(i+1, curWeight+order.Weight, curValue+order.Value, append(curSet, order))
	}

	canceled := dfs(0, 0, 0, nil)
	if canceled {
		return model.DeliveryPlan{}, ctx.Err()
	}

	var totalWeight int
	for _, o := range bestSet {
		totalWeight += o.Weight
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      bestSet,
	}, nil
}
