package repository

import (
	"backend/internal/model"
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 許可されたソートフィールドのホワイトリスト
var allowedSortFields = map[string]bool{
	"product_id": true,
	"name":       true,
	"value":      true,
	"weight":     true,
}

// 許可されたソート順のホワイトリスト
var allowedSortOrders = map[string]bool{
	"ASC":  true,
	"DESC": true,
	"asc":  true,
	"desc": true,
}

// 商品一覧をデータベース側でページング処理して取得
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	// ソートフィールドとソート順の検証
	sortField := req.SortField
	if !allowedSortFields[sortField] {
		sortField = "product_id"
	}
	sortOrder := strings.ToUpper(req.SortOrder)
	if !allowedSortOrders[sortOrder] {
		sortOrder = "ASC"
	}

	// WHERE句の構築
	whereClause := ""
	whereArgs := []interface{}{}
	if req.Search != "" {
		whereClause = " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		whereArgs = append(whereArgs, searchPattern, searchPattern)
	}

	// 総件数を取得するクエリ
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM products%s", whereClause)
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, whereArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get count: %w", err)
	}

	// ページングされた商品を取得するクエリ
	// LIMITとOFFSETを使用してデータベース側でページング処理
	selectQuery := fmt.Sprintf(`
		SELECT product_id, name, value, weight, image, description
		FROM products
		%s
		ORDER BY %s %s, product_id ASC
		LIMIT ? OFFSET ?
	`, whereClause, sortField, sortOrder)

	// SELECTクエリ用の引数（WHERE句の引数 + LIMIT + OFFSET）
	selectArgs := make([]interface{}, len(whereArgs))
	copy(selectArgs, whereArgs)
	selectArgs = append(selectArgs, req.PageSize, req.Offset)

	var products []model.Product
	err = r.db.SelectContext(ctx, &products, selectQuery, selectArgs...)
	if err != nil {
		if err == sql.ErrNoRows {
			return []model.Product{}, total, nil
		}
		return nil, 0, fmt.Errorf("failed to select products: %w", err)
	}

	return products, total, nil
}
