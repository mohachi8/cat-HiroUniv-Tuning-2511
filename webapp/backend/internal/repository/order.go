package repository

import (
	"backend/internal/model"
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
// 最適化: 大量のorderIDsをバッチ処理に分割して、DBアクセス回数を削減
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	if len(orderIDs) == 0 {
		return nil
	}

	// MySQLのIN句の制限（通常65535個）を考慮し、バッチサイズを1000に設定
	// これにより、大量のorderIDsでも効率的に処理できる
	const batchSize = 1000

	for i := 0; i < len(orderIDs); i += batchSize {
		end := i + batchSize
		if end > len(orderIDs) {
			end = len(orderIDs)
		}
		batch := orderIDs[i:end]

		query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, batch)
		if err != nil {
			return err
		}
		query = r.db.Rebind(query)
		_, err = r.db.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateStatusesConditional updates statuses only when current status equals expectedCurrent.
// Returns number of rows affected.
func (r *OrderRepository) UpdateStatusesConditional(ctx context.Context, orderIDs []int64, newStatus string, expectedCurrent string) (int64, error) {
	if len(orderIDs) == 0 {
		return 0, nil
	}

	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?) AND shipped_status = ?", newStatus, orderIDs, expectedCurrent)
	if err != nil {
		return 0, err
	}
	query = r.db.Rebind(query)
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	tracer := otel.Tracer("backend/repository.OrderRepository")
	ctx, span := tracer.Start(ctx, "GetShippingOrders")
	defer span.End()

	var orders []model.Order

	// build-query span (child)
	_, buildSpan := tracer.Start(ctx, "build-query")
	const defaultCandidateLimit = 2000
	query := `
		SELECT
			o.order_id,
			p.weight,
			p.value
		FROM orders o
		JOIN products p ON o.product_id = p.product_id
		WHERE o.shipped_status = 'shipping'
		ORDER BY (p.value / NULLIF(p.weight, 0)) DESC
		LIMIT ?
	`
	buildSpan.SetAttributes(attribute.String("db.statement_snippet", "SELECT o.order_id, p.weight, p.value FROM orders JOIN products WHERE shipped_status = 'shipping' ORDER BY (p.value/p.weight) DESC LIMIT ?"))
	buildSpan.End()

	// db select span (child) - the otelsql instrumentation will produce its own `sql.rows` span,
	// but create an explicit parent child to make the hierarchy clear
	selCtx, selSpan := tracer.Start(ctx, "db.select")

	// Use QueryContext + manual rows.Scan loop so we can trace per-row scanning.
	rows, err := r.db.QueryxContext(selCtx, query, defaultCandidateLimit)
	if err != nil {
		selSpan.RecordError(err)
		selSpan.SetStatus(codes.Error, err.Error())
		selSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	// scan-loop span (child of db.select)
	_, scanLoopSpan := tracer.Start(selCtx, "scan-loop")
	count := 0
	// collect a small sample of order IDs to record as attribute (avoid per-row spans)
	var sampleIDs []int64
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.OrderID, &o.Weight, &o.Value); err != nil {
			scanLoopSpan.RecordError(err)
			scanLoopSpan.SetStatus(codes.Error, err.Error())
			scanLoopSpan.End()
			selSpan.RecordError(err)
			selSpan.SetStatus(codes.Error, err.Error())
			selSpan.End()
			return nil, err
		}
		orders = append(orders, o)
		if len(sampleIDs) < 5 {
			sampleIDs = append(sampleIDs, o.OrderID)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		scanLoopSpan.RecordError(err)
		scanLoopSpan.SetStatus(codes.Error, err.Error())
		scanLoopSpan.End()
		selSpan.RecordError(err)
		selSpan.SetStatus(codes.Error, err.Error())
		selSpan.End()
		return nil, err
	}
	// record count and a small sample of order IDs for debugging without creating many spans
	scanLoopSpan.SetAttributes(attribute.Int("orders.fetched", count))
	if len(sampleIDs) > 0 {
		// build comma separated string
		var sids []string
		for _, id := range sampleIDs {
			sids = append(sids, strconv.FormatInt(id, 10))
		}
		scanLoopSpan.SetAttributes(attribute.String("orders.sample_ids", strings.Join(sids, ",")))
	}
	scanLoopSpan.End()

	selSpan.SetAttributes(attribute.Int("orders.fetched", len(orders)))
	selSpan.End()

	// process-rows span (grandchild)
	_, procSpan := tracer.Start(ctx, "process-rows")
	procSpan.SetAttributes(attribute.Int("orders.count", len(orders)))
	// minimal processing here; heavy processing should be traced where it happens
	procSpan.End()

	return orders, nil
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
