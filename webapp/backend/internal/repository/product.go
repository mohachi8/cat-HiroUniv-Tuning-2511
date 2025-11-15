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
	// UNIONを使用して各インデックスを効率的に活用し、重複を自動的に排除
	var countQuery string
	var selectQuery string
	var countArgs []interface{}
	var selectArgs []interface{}

	if req.Search != "" {
		searchPattern := "%" + req.Search + "%"
		// COUNTクエリ: UNIONを使用して重複を排除し、各インデックスを個別に使用
		// これにより、idx_products_nameとidx_products_descriptionを効率的に活用できる
		countQuery = `
			SELECT COUNT(DISTINCT product_id) FROM (
				SELECT product_id FROM products WHERE name LIKE ?
				UNION
				SELECT product_id FROM products WHERE description LIKE ?
			) AS combined_results
		`
		countArgs = []interface{}{searchPattern, searchPattern}

		// SELECTクエリ: UNIONで取得したproduct_idのリストを使って検索
		// その後、ソート・ページングを実行
		selectQuery = fmt.Sprintf(`
			SELECT product_id, name, value, weight, image, description
			FROM products
			WHERE product_id IN (
				SELECT product_id FROM (
					SELECT product_id FROM products WHERE name LIKE ?
					UNION
					SELECT product_id FROM products WHERE description LIKE ?
				) AS combined_results
			)
			ORDER BY %s %s, product_id ASC
			LIMIT ? OFFSET ?
		`, sortField, sortOrder)
		selectArgs = []interface{}{searchPattern, searchPattern, req.PageSize, req.Offset}
	} else {
		// 検索条件なしの場合
		countQuery = "SELECT COUNT(*) FROM products"
		countArgs = nil

		selectQuery = fmt.Sprintf(`
			SELECT product_id, name, value, weight, image, description
			FROM products
			ORDER BY %s %s, product_id ASC
			LIMIT ? OFFSET ?
		`, sortField, sortOrder)
		selectArgs = []interface{}{req.PageSize, req.Offset}
	}

	// 総件数を取得するクエリ（最適化されたクエリを使用）
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, countArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get count: %w", err)
	}

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
