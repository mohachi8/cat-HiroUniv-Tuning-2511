package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"backend/internal/repository"
	"backend/internal/service/utils"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInternalServer  = errors.New("internal server error")
)

type AuthService struct {
	store *repository.Store
}

func NewAuthService(store *repository.Store) *AuthService {
	return &AuthService{store: store}
}

// verifyPasswordHash SHA-256 + ソルトを使用した高速なパスワード検証
// マイグレーションでbcryptからSHA-256に変換済みのため、SHA-256のみをサポート
// レギュレーションにより「不可逆であれば、どのような方式に変更してもかまいません」とあるため、
// SHA-256を使用して高速化を実現
func verifyPasswordHash(password, storedHash string) bool {
	const salt = "cat-hiro-univ-tuning-2511-salt"

	// パスワード + ソルトをハッシュ化
	hash := sha256.Sum256([]byte(password + salt))
	computedHash := hex.EncodeToString(hash[:])

	// 保存されているハッシュと比較
	return computedHash == storedHash
}

func (s *AuthService) Login(ctx context.Context, userName, password string) (string, time.Time, error) {
	var sessionID string
	var expiresAt time.Time
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		user, err := s.store.UserRepo.FindByUserName(ctx, userName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrUserNotFound
			}
			return ErrInternalServer
		}

		// SHA-256による高速なパスワード検証
		passwordValid := verifyPasswordHash(password, user.PasswordHash)
		if !passwordValid {
			return ErrInvalidPassword
		}

		sessionDuration := 24 * time.Hour
		sessionID, expiresAt, err = s.store.SessionRepo.Create(ctx, user.UserID, sessionDuration)
		if err != nil {
			return ErrInternalServer
		}
		return nil
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return sessionID, expiresAt, nil
}
