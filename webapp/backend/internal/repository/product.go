package repository

import (
	"backend/internal/model"
	"context"
	"unicode/utf8"
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
		// 検索文字列の長さに応じて最適なクエリを選択
		// 5文字未満: LIKEのみを使用（MATCH() AGAINST()はN-gramパーサーで効果がないため）
		// 5文字以上: MATCH() AGAINST()を使用（FULLTEXT INDEXで高速化）
		searchLen := utf8.RuneCountInString(req.Search)

		if searchLen >= 5 {
			// 5文字以上: FULLTEXT INDEXを使用して高速検索
			baseQuery += " WHERE MATCH(search_text) AGAINST(? IN BOOLEAN MODE)"
			args = append(args, req.Search)
		} else {
			// 5文字未満: LIKEを使用（MATCH()を試さないことで無駄な処理を回避）
			searchPattern := "%" + req.Search + "%"
			baseQuery += " WHERE search_text LIKE ?"
			args = append(args, searchPattern)
		}
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

	if req.Search == "" {
		// 検索条件がない場合は全件カウント
		baseQuery := "SELECT COUNT(*) FROM products"
		err := r.db.GetContext(ctx, &count, baseQuery)
		if err != nil {
			return 0, err
		}
		return count, nil
	}

	// 検索文字列の長さに応じて最適なクエリを選択
	// 5文字未満: LIKEのみを使用（MATCH() AGAINST()はN-gramパーサーで効果がないため）
	// 5文字以上: MATCH() AGAINST()を使用（FULLTEXT INDEXで高速化）
	searchLen := utf8.RuneCountInString(req.Search)

	var baseQuery string
	if searchLen >= 5 {
		// 5文字以上: FULLTEXT INDEXを使用して高速検索
		baseQuery = "SELECT COUNT(*) FROM products WHERE MATCH(search_text) AGAINST(? IN BOOLEAN MODE)"
		err := r.db.GetContext(ctx, &count, baseQuery, req.Search)
		if err != nil {
			return 0, err
		}
	} else {
		// 5文字未満: LIKEを使用（MATCH()を試さないことで無駄な処理を回避）
		searchPattern := "%" + req.Search + "%"
		baseQuery = "SELECT COUNT(*) FROM products WHERE search_text LIKE ?"
		err := r.db.GetContext(ctx, &count, baseQuery, searchPattern)
		if err != nil {
			return 0, err
		}
	}

	return count, nil
}
