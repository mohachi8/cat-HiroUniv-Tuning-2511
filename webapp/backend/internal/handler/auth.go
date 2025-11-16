package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"backend/internal/model"
	"backend/internal/service"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type AuthHandler struct {
	AuthSvc *service.AuthService
}

func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{AuthSvc: authSvc}
}

// ログイン時にセッションを発行し、Cookieにセットする
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	tracer := otel.Tracer("handler.auth")
	ctx, span := tracer.Start(r.Context(), "AuthHandler.Login")
	defer span.End()

	log.Println("-> Received request for /api/login")

	// JSONデコードのスパン
	ctx, decodeSpan := tracer.Start(ctx, "Login.DecodeRequest")
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		decodeSpan.RecordError(err)
		decodeSpan.End()
		span.RecordError(err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	decodeSpan.SetAttributes(attribute.String("user.name", req.UserName))
	decodeSpan.End()

	span.SetAttributes(
		attribute.String("user.name", req.UserName),
	)

	// ログイン処理のスパン
	ctx, loginSpan := tracer.Start(ctx, "Login.AuthService")
	sessionID, expiresAt, err := h.AuthSvc.Login(ctx, req.UserName, req.Password)
	loginSpan.End()

	if err != nil {
		span.RecordError(err)
		if errors.Is(err, service.ErrUserNotFound) {
			span.SetAttributes(attribute.String("error.type", "user_not_found"))
			http.Error(w, "Unauthorized: Invalid credentials", http.StatusUnauthorized)
		} else if errors.Is(err, service.ErrInvalidPassword) {
			span.SetAttributes(attribute.String("error.type", "invalid_password"))
			http.Error(w, "Unauthorized: Invalid credentials", http.StatusUnauthorized)
		} else {
			span.SetAttributes(attribute.String("error.type", "internal_error"))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Cookie設定のスパン
	_, cookieSpan := tracer.Start(ctx, "Login.SetCookie")
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Expires:  expiresAt,
		HttpOnly: true,
		Path:     "/",
	})
	cookieSpan.SetAttributes(
		attribute.String("cookie.name", "session_id"),
		attribute.String("cookie.expires_at", expiresAt.Format("2006-01-02T15:04:05Z07:00")),
	)
	cookieSpan.End()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Login successful"})

	span.SetAttributes(attribute.Bool("login.success", true))
}
