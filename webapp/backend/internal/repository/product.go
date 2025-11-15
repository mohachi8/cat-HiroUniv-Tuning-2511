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
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, error) {
	var products []model.Product
	baseQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	`
	args := []interface{}{}

	if req.Search != "" {
		// FULLTEXT INDEX with N-gramパーサーを使用して部分一致検索を高速化
		// MATCH() AGAINST()とLIKEを組み合わせることで、確実に結果を返しつつ高速化
		// MATCH() AGAINST()が使えない場合（短い文字列など）でもLIKEでフォールバック
		searchPattern := "%" + req.Search + "%"
		baseQuery += " WHERE ((MATCH(name) AGAINST(? IN BOOLEAN MODE) OR MATCH(description) AGAINST(? IN BOOLEAN MODE)) OR (name LIKE ? OR description LIKE ?))"
		args = append(args, req.Search, req.Search, searchPattern, searchPattern)
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
	baseQuery := "SELECT COUNT(*) FROM products"
	args := []interface{}{}

	if req.Search != "" {
		// FULLTEXT INDEX with N-gramパーサーを使用して部分一致検索を高速化
		// MATCH() AGAINST()とLIKEを組み合わせることで、確実に結果を返しつつ高速化
		// MATCH() AGAINST()が使えない場合（短い文字列など）でもLIKEでフォールバック
		searchPattern := "%" + req.Search + "%"
		baseQuery += " WHERE ((MATCH(name) AGAINST(? IN BOOLEAN MODE) OR MATCH(description) AGAINST(? IN BOOLEAN MODE)) OR (name LIKE ? OR description LIKE ?))"
		args = append(args, req.Search, req.Search, searchPattern, searchPattern)
	}

	err := r.db.GetContext(ctx, &count, baseQuery, args...)
	if err != nil {
		return 0, err
	}

	return count, nil
}
