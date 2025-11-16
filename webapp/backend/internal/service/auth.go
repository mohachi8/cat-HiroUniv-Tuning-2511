package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"time"

	"backend/internal/repository"
	"backend/internal/service/utils"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	tracer := otel.Tracer("service.auth")
	ctx, span := tracer.Start(ctx, "AuthService.Login")
	defer span.End()

	span.SetAttributes(attribute.String("user.name", userName))

	var sessionID string
	var expiresAt time.Time
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		// ユーザー検索のスパン
		ctx, findUserSpan := tracer.Start(ctx, "Login.FindUser")
		user, err := s.store.UserRepo.FindByUserName(ctx, userName)
		if err != nil {
			findUserSpan.RecordError(err)
			findUserSpan.SetAttributes(attribute.Bool("user.found", false))
			if errors.Is(err, sql.ErrNoRows) {
				findUserSpan.SetAttributes(attribute.String("error.type", "user_not_found"))
				findUserSpan.End()
				log.Printf("[Login] ユーザー検索失敗(userName: %s): %v", userName, err)
				return ErrUserNotFound
			}
			findUserSpan.SetAttributes(attribute.String("error.type", "internal_error"))
			findUserSpan.End()
			log.Printf("[Login] ユーザー検索失敗(userName: %s): %v", userName, err)
			return ErrInternalServer
		}
		findUserSpan.SetAttributes(
			attribute.Bool("user.found", true),
			attribute.Int("user.id", user.UserID),
		)
		findUserSpan.End()

		// パスワード検証のスパン
		// SHA-256 + ソルトを使用した高速なパスワード検証
		// マイグレーションでbcryptからSHA-256に変換済みのため、SHA-256のみをサポート
		ctx, verifyPasswordSpan := tracer.Start(ctx, "Login.VerifyPassword")

		// SHA-256による高速なパスワード検証
		passwordValid := verifyPasswordHash(password, user.PasswordHash)

		// 検証結果を記録
		verifyPasswordSpan.SetAttributes(
			attribute.Bool("password.valid", passwordValid),
			attribute.Int("password.hash_length", len(user.PasswordHash)),
			attribute.String("password.hash_algorithm", "sha256"),
		)

		if !passwordValid {
			verifyPasswordSpan.SetAttributes(attribute.String("error.type", "invalid_password"))
			verifyPasswordSpan.End()
			log.Printf("[Login] パスワード検証失敗")
			span.RecordError(ErrInvalidPassword)
			return ErrInvalidPassword
		}

		verifyPasswordSpan.End()

		// セッション作成のスパン
		ctx, createSessionSpan := tracer.Start(ctx, "Login.CreateSession")
		sessionDuration := 24 * time.Hour
		sessionID, expiresAt, err = s.store.SessionRepo.Create(ctx, user.UserID, sessionDuration)
		if err != nil {
			createSessionSpan.RecordError(err)
			createSessionSpan.SetAttributes(attribute.String("error.type", "session_creation_failed"))
			createSessionSpan.End()
			log.Printf("[Login] セッション生成失敗: %v", err)
			return ErrInternalServer
		}
		createSessionSpan.SetAttributes(
			attribute.String("session.id", sessionID),
			attribute.String("session.expires_at", expiresAt.Format("2006-01-02T15:04:05Z07:00")),
			attribute.String("session.duration", sessionDuration.String()),
		)
		createSessionSpan.End()
		return nil
	})
	if err != nil {
		span.RecordError(err)
		return "", time.Time{}, err
	}
	log.Printf("Login successful for UserName '%s', session created.", userName)
	span.SetAttributes(attribute.Bool("login.success", true))
	return sessionID, expiresAt, nil
}
