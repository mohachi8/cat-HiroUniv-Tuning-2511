package repository

import (
	"backend/internal/model"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (string, error) {
	query := `INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES (?, ?, 'shipping', NOW())`
	result, err := r.db.ExecContext(ctx, query, order.UserID, order.ProductID)
	if err != nil {
		return "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

// 複数の注文IDのステータスを一括で更新
// 主に配送ロボットが注文を引き受けた際に一括更新をするために使用
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, orderIDs)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	query := `
        SELECT
            o.order_id,
            p.weight,
            p.value
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.shipped_status = 'shipping'
    `
	err := r.db.SelectContext(ctx, &orders, query)
	return orders, err
}

// 許可されたソートフィールドのホワイトリスト
var allowedOrderSortFields = map[string]bool{
	"order_id":       true,
	"product_name":   true,
	"created_at":     true,
	"shipped_status": true,
	"arrived_at":     true,
}

// 許可されたソート順のホワイトリスト
var allowedOrderSortOrders = map[string]bool{
	"ASC":  true,
	"DESC": true,
	"asc":  true,
	"desc": true,
}

// 注文の総件数を取得
func (r *OrderRepository) CountOrders(ctx context.Context, userID int, req model.ListRequest) (int, error) {
	// WHERE句の構築
	whereClause := "WHERE o.user_id = ?"
	whereArgs := []interface{}{userID}

	// 検索条件の追加
	if req.Search != "" {
		if req.Type == "prefix" {
			whereClause += " AND p.name LIKE ?"
			whereArgs = append(whereArgs, req.Search+"%")
		} else {
			// partial (デフォルト)
			whereClause += " AND p.name LIKE ?"
			whereArgs = append(whereArgs, "%"+req.Search+"%")
		}
	}

	var count int
	var err error
	if req.Search == "" {
		// 検索条件が無ければ JOIN は不要なので orders のみでカウントして高速化
		countQuery := "SELECT COUNT(*) FROM orders WHERE user_id = ?"
		err = r.db.GetContext(ctx, &count, countQuery, userID)
	} else {
		// 検索がある場合は product に対する条件があるため JOIN が必要
		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM orders o
			JOIN products p ON o.product_id = p.product_id
			%s
		`, whereClause)
		err = r.db.GetContext(ctx, &count, countQuery, whereArgs...)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get count: %w", err)
	}

	return count, nil
}

// 注文履歴一覧を取得
// データベース側でJOIN、フィルタリング、ソート、ページングを実行
func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, error) {
	// ソートフィールドとソート順の検証
	sortField := req.SortField
	if !allowedOrderSortFields[sortField] {
		sortField = "order_id"
	}
	sortOrder := strings.ToUpper(req.SortOrder)
	if !allowedOrderSortOrders[sortOrder] {
		sortOrder = "DESC"
	}

	// WHERE句の構築
	whereClause := "WHERE o.user_id = ?"
	whereArgs := []interface{}{userID}

	// 検索条件の追加
	if req.Search != "" {
		if req.Type == "prefix" {
			whereClause += " AND p.name LIKE ?"
			whereArgs = append(whereArgs, req.Search+"%")
		} else {
			// partial (デフォルト)
			whereClause += " AND p.name LIKE ?"
			whereArgs = append(whereArgs, "%"+req.Search+"%")
		}
	}

	// ORDER BY句の構築
	orderByClause := "ORDER BY "
	switch sortField {
	case "product_name":
		orderByClause += fmt.Sprintf("p.name %s, o.order_id ASC", sortOrder)
	case "created_at":
		orderByClause += fmt.Sprintf("o.created_at %s, o.order_id ASC", sortOrder)
	case "shipped_status":
		orderByClause += fmt.Sprintf("o.shipped_status %s, o.order_id ASC", sortOrder)
	case "arrived_at":
		// NULL値の処理: NULLは最後に配置（MySQL互換性のためISNULL()を使用）
		if sortOrder == "DESC" {
			orderByClause += "ISNULL(o.arrived_at) ASC, o.arrived_at DESC, o.order_id ASC"
		} else {
			orderByClause += "ISNULL(o.arrived_at) ASC, o.arrived_at ASC, o.order_id ASC"
		}
	case "order_id":
		fallthrough
	default:
		orderByClause += fmt.Sprintf("o.order_id %s", sortOrder)
	}

	// ページングされた注文を取得するクエリ
	// JOINを使って商品名を一度に取得（N+1クエリ問題を解決）
	selectQuery := fmt.Sprintf(`
		SELECT 
			o.order_id,
			o.product_id,
			p.name AS product_name,
			o.shipped_status,
			o.created_at,
			o.arrived_at
		FROM orders o
		JOIN products p ON o.product_id = p.product_id
		%s
		%s
		LIMIT ? OFFSET ?
	`, whereClause, orderByClause)

	// SELECTクエリ用の引数（WHERE句の引数 + LIMIT + OFFSET）
	selectArgs := make([]interface{}, len(whereArgs))
	copy(selectArgs, whereArgs)
	selectArgs = append(selectArgs, req.PageSize, req.Offset)

	type orderRow struct {
		OrderID       int64        `db:"order_id"`
		ProductID     int          `db:"product_id"`
		ProductName   string       `db:"product_name"`
		ShippedStatus string       `db:"shipped_status"`
		CreatedAt     sql.NullTime `db:"created_at"`
		ArrivedAt     sql.NullTime `db:"arrived_at"`
	}

	var ordersRaw []orderRow
	err := r.db.SelectContext(ctx, &ordersRaw, selectQuery, selectArgs...)
	if err != nil {
		if err == sql.ErrNoRows {
			return []model.Order{}, nil
		}
		return nil, fmt.Errorf("failed to select orders: %w", err)
	}

	// orderRowからOrderに変換
	orders := make([]model.Order, len(ordersRaw))
	for i, o := range ordersRaw {
		orders[i] = model.Order{
			OrderID:       o.OrderID,
			ProductID:     o.ProductID,
			ProductName:   o.ProductName,
			ShippedStatus: o.ShippedStatus,
			CreatedAt:     o.CreatedAt.Time,
			ArrivedAt:     o.ArrivedAt,
		}
	}

	return orders, nil
}
