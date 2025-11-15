package repository

import (
	"backend/internal/model"
	"context"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧を取得（SQLレベルでページング処理を行う）
// 商品データは常にMySQLから取得（順序が重要なため）
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, error) {
	var products []model.Product
	var baseQuery string
	args := []interface{}{}

	if req.Search != "" {
		// OR条件を使用（LIKE '%...%'ではインデックスが使えないため、UNIONよりシンプルなORの方が速い）
		searchPattern := "%" + req.Search + "%"
		baseQuery = `
			SELECT product_id, name, value, weight, image, description
			FROM products
			WHERE name LIKE ? OR description LIKE ?
		`
		args = append(args, searchPattern, searchPattern)
	} else {
		baseQuery = `
			SELECT product_id, name, value, weight, image, description
			FROM products
		`
	}

	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)

	err := r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		return nil, err
	}

	return products, nil
}

// 商品の総件数を取得
func (r *ProductRepository) CountProducts(ctx context.Context, userID int, req model.ListRequest) (int, error) {
	var count int
	var baseQuery string
	args := []interface{}{}

	if req.Search != "" {
		// OR条件を使用（LIKE '%...%'ではインデックスが使えないため、UNIONよりシンプルなORの方が速い）
		// DISTINCTで重複を排除して正確な件数を取得
		searchPattern := "%" + req.Search + "%"
		baseQuery = `
			SELECT COUNT(DISTINCT product_id) 
			FROM products
			WHERE name LIKE ? OR description LIKE ?
		`
		args = append(args, searchPattern, searchPattern)
	} else {
		baseQuery = "SELECT COUNT(*) FROM products"
	}

	err := r.db.GetContext(ctx, &count, baseQuery, args...)
	if err != nil {
		return 0, err
	}

	return count, nil
}
