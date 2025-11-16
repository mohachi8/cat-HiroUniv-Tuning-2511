package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type Store struct {
	db          DBTX
	UserRepo    *UserRepository
	SessionRepo *SessionRepository
	ProductRepo *ProductRepository
	OrderRepo   *OrderRepository
}

func NewStore(db DBTX) *Store {
	return &Store{
		db:          db,
		UserRepo:    NewUserRepository(db),
		SessionRepo: NewSessionRepository(db),
		ProductRepo: NewProductRepository(db),
		OrderRepo:   NewOrderRepository(db),
	}
}

func (s *Store) ExecTx(ctx context.Context, fn func(txStore *Store) error) error {
	db, ok := s.db.(*sqlx.DB)
	if !ok {
		return fn(s)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	txStore := NewStore(tx)
	if err := fn(txStore); err != nil {
		return err
	}

	return tx.Commit()
}

// Close closes all prepared statements in repositories
func (s *Store) Close() error {
	var errs []error
	if err := s.UserRepo.Close(); err != nil {
		errs = append(errs, err)
	}
	// 他のRepositoryにもPrepared Statementを追加した場合はここに追加
	// if err := s.SessionRepo.Close(); err != nil {
	//     errs = append(errs, err)
	// }
	if len(errs) > 0 {
		return errs[0] // 最初のエラーを返す（簡易実装）
	}
	return nil
}
