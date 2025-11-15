package repository

import (
	"backend/internal/model"
	"context"
	"strings"
	"unicode/utf8"
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
		// 検索文字列の長さに応じて最適なクエリを選択
		// 5文字未満: LIKEのみを使用（MATCH() AGAINST()はN-gramパーサーで効果がないため）
		// 5文字以上: MATCH() AGAINST()を使用（FULLTEXT INDEXで高速化）
		searchLen := utf8.RuneCountInString(req.Search)
		searchPattern := "%" + req.Search + "%"

		if searchLen >= 5 {
			// 5文字以上: FULLTEXT INDEXを使用して高速検索
			// search_textカラムが存在しない場合のフォールバックとして、nameとdescriptionを直接検索
			baseQuery += " WHERE MATCH(search_text) AGAINST(? IN BOOLEAN MODE)"
			args = append(args, req.Search)
		} else {
			// 5文字未満: LIKEを使用（MATCH()を試さないことで無駄な処理を回避）
			// search_textカラムが存在しない場合のフォールバックとして、nameとdescriptionを直接検索
			baseQuery += " WHERE search_text LIKE ?"
			args = append(args, searchPattern)
		}
	}

	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, req.Offset)

	err := r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		// search_textカラムが存在しない場合のエラーをキャッチしてフォールバック
		// エラーメッセージに"search_text"が含まれている場合は、nameとdescriptionで検索
		errMsg := err.Error()
		if contains(errMsg, "search_text") || contains(errMsg, "Unknown column") {
			// フォールバック: nameとdescriptionで検索
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

	// 検索文字列の長さに応じて最適なクエリを選択
	// 5文字未満: LIKEのみを使用（MATCH() AGAINST()はN-gramパーサーで効果がないため）
	// 5文字以上: MATCH() AGAINST()を使用（FULLTEXT INDEXで高速化）
	// search_textカラムが存在しない場合のフォールバックとして、nameとdescriptionを直接検索
	searchLen := utf8.RuneCountInString(req.Search)
	searchPattern := "%" + req.Search + "%"

	var baseQuery string
	if searchLen >= 5 {
		// 5文字以上: FULLTEXT INDEXを使用して高速検索
		baseQuery = "SELECT COUNT(*) FROM products WHERE MATCH(search_text) AGAINST(? IN BOOLEAN MODE)"
		err := r.db.GetContext(ctx, &count, baseQuery, req.Search)
		if err != nil {
			// search_textカラムが存在しない場合のエラーをキャッチしてフォールバック
			errMsg := err.Error()
			if contains(errMsg, "search_text") || contains(errMsg, "Unknown column") {
				// フォールバック: nameとdescriptionで検索
				fallbackQuery := "SELECT COUNT(*) FROM products WHERE (name LIKE ? OR description LIKE ?)"
				err = r.db.GetContext(ctx, &count, fallbackQuery, searchPattern, searchPattern)
				if err != nil {
					return 0, err
				}
				return count, nil
			}
			return 0, err
		}
	} else {
		// 5文字未満: LIKEを使用（MATCH()を試さないことで無駄な処理を回避）
		baseQuery = "SELECT COUNT(*) FROM products WHERE search_text LIKE ?"
		err := r.db.GetContext(ctx, &count, baseQuery, searchPattern)
		if err != nil {
			// search_textカラムが存在しない場合のエラーをキャッチしてフォールバック
			errMsg := err.Error()
			if contains(errMsg, "search_text") || contains(errMsg, "Unknown column") {
				// フォールバック: nameとdescriptionで検索
				fallbackQuery := "SELECT COUNT(*) FROM products WHERE (name LIKE ? OR description LIKE ?)"
				err = r.db.GetContext(ctx, &count, fallbackQuery, searchPattern, searchPattern)
				if err != nil {
					return 0, err
				}
				return count, nil
			}
			return 0, err
		}
	}

	return count, nil
}
