package repository

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"backend/internal/model"
	"github.com/jmoiron/sqlx"
)

type UserRepository struct {
	db                DBTX
	findByUserNameStmt *sqlx.Stmt
}

func NewUserRepository(db DBTX) *UserRepository {
	ur := &UserRepository{db: db}
	// Try to prepare statement if we have a *sqlx.DB
	if d, ok := db.(*sqlx.DB); ok {
		if stmt, err := d.Preparex("SELECT user_id, password_hash, user_name FROM users WHERE user_name = ?"); err == nil {
			ur.findByUserNameStmt = stmt
		} else {
			// prepare 失敗はログに残してフォールバック
			log.Printf("prepare failed for FindByUserName: %v", err)
		}
	}
	return ur
}

// Close closes prepared statements
func (r *UserRepository) Close() error {
	if r.findByUserNameStmt != nil {
		return r.findByUserNameStmt.Close()
	}
	return nil
}

// ユーザー名からユーザー情報を取得
// ログイン時に使用
func (r *UserRepository) FindByUserName(ctx context.Context, userName string) (*model.User, error) {
	var user model.User

	// Prepared statement が利用可能な場合は使用
	if r.findByUserNameStmt != nil {
		err := r.findByUserNameStmt.GetContext(ctx, &user, userName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			return nil, err
		}
		return &user, nil
	}

	// フォールバック: 通常のクエリ実行
	query := "SELECT user_id, password_hash, user_name FROM users WHERE user_name = ?"
	err := r.db.GetContext(ctx, &user, query, userName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, err
	}
	return &user, nil
}
