package repository

import (
	"backend/internal/model"
	"context"
	"strings"
)

// contains は文字列に部分文字列が含まれているかチェック
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

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
		// search_textのFULLTEXT INDEXを使用して高速検索
		// ngram_token_size=1の設定により、すべての検索文字列（1文字など）でもインデックスが有効に機能する
		baseQuery += " WHERE MATCH(search_text) AGAINST(? IN BOOLEAN MODE)"
		args = append(args, req.Search)
	}

	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)

	err := r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		// FULLTEXT INDEXが存在しない場合のエラーをキャッチしてフォールバック
		// エラーメッセージに"search_text"や"FULLTEXT"が含まれている場合は、LIKEで検索
		errMsg := err.Error()
		if contains(errMsg, "search_text") || contains(errMsg, "FULLTEXT") || contains(errMsg, "Unknown column") {
			// フォールバック: nameとdescriptionでLIKE検索
			searchPattern := "%" + req.Search + "%"
			fallbackQuery := `
				SELECT product_id, name, value, weight, image, description
				FROM products
				WHERE (name LIKE ? OR description LIKE ?)
			`
			fallbackQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
			fallbackQuery += " LIMIT ? OFFSET ?"
			err = r.db.SelectContext(ctx, &products, fallbackQuery, searchPattern, searchPattern, req.PageSize, req.Offset)
			if err != nil {
				return nil, err
			}
			return products, nil
		}
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

	// search_textのFULLTEXT INDEXを使用して高速検索
	// ngram_token_size=1の設定により、すべての検索文字列（1文字など）でもインデックスが有効に機能する
	// COUNT(*)でも、FULLTEXT INDEXを使ってマッチする行だけをカウントできるため、全件スキャンは不要
	baseQuery := "SELECT COUNT(*) FROM products WHERE MATCH(search_text) AGAINST(? IN BOOLEAN MODE)"
	err := r.db.GetContext(ctx, &count, baseQuery, req.Search)

	// FULLTEXT INDEXが機能しない場合（エラー時）はLIKE検索にフォールバック
	if err != nil {
		errMsg := err.Error()
		if contains(errMsg, "search_text") || contains(errMsg, "FULLTEXT") || contains(errMsg, "Unknown column") {
			// フォールバック: LIKE検索（全件スキャンになるが、FULLTEXT INDEXが使えない場合は仕方ない）
			// 注意: LIKE '%pattern%'ではインデックスが使えないため、全件スキャンが発生する
			searchPattern := "%" + req.Search + "%"
			fallbackQuery := "SELECT COUNT(*) FROM products WHERE (name LIKE ? OR description LIKE ?)"
			err = r.db.GetContext(ctx, &count, fallbackQuery, searchPattern, searchPattern)
			if err != nil {
				return 0, err
			}
			return count, nil
		}
		return 0, err
	}

	return count, nil
}
