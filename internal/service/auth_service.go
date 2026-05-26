package service

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

type AuthResult struct {
	UserID       string `json:"userId"`
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

type RefreshResult struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

type MeResult struct {
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
}

type AuthService struct {
	db         *gorm.DB
	userRepo   *repository.UserRepository
	tokenRepo  *repository.TokenRepository
	jwtSecret  string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewAuthService(
	db *gorm.DB,
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	jwtSecret string,
	accessTTL time.Duration,
	refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		db:         db,
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtSecret:  jwtSecret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *AuthService) Register(email string, username string, password string) (*AuthResult, error) {
	email = normalizeEmail(email)
	username = normalizeUsername(username)
	if !validateEmail(email) {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidEmailFormat, "invalid email format")
	}
	if err := validateUsername(username); err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	passwordHash, err := pkg.HashPassword(password)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	var result AuthResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		userRepo := s.userRepo.WithTx(tx)
		tokenRepo := s.tokenRepo.WithTx(tx)

		user := &model.User{
			Email:        email,
			Username:     username,
			PasswordHash: passwordHash,
		}
		if err := userRepo.Create(user); err != nil {
			if appErr := mapUserUniqueViolation(err); appErr != nil {
				return appErr
			}
			return pkg.InternalError(err)
		}

		session, err := s.issueSession(tokenRepo, user.ID)
		if err != nil {
			return err
		}

		result = AuthResult{
			UserID:       user.ID.String(),
			Token:        session.Token,
			RefreshToken: session.RefreshToken,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *AuthService) Login(identifier string, password string) (*AuthResult, error) {
	user, err := s.findUserByIdentifier(identifier)
	if err != nil {
		return nil, err
	}
	if user.Disabled {
		return nil, pkg.NewAppError(http.StatusForbidden, pkg.CodeAccountDisabled, "account disabled")
	}
	if err := pkg.ComparePassword(user.PasswordHash, password); err != nil {
		return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidCredentials, "invalid credentials")
	}

	session, err := s.issueSession(s.tokenRepo, user.ID)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		UserID:       user.ID.String(),
		Token:        session.Token,
		RefreshToken: session.RefreshToken,
	}, nil
}

func (s *AuthService) Refresh(refreshToken string) (*RefreshResult, error) {
	tokenHash := pkg.HashToken(refreshToken)
	tokenRecord, err := s.tokenRepo.FindByTokenHash(tokenHash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidRefreshToken, "invalid refresh token")
		}
		return nil, pkg.InternalError(err)
	}
	if tokenRecord.Revoked {
		return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeRefreshTokenRevoked, "token revoked")
	}
	if !tokenRecord.ExpiresAt.After(time.Now().UTC()) {
		return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidRefreshToken, "invalid refresh token")
	}

	var result RefreshResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		tokenRepo := s.tokenRepo.WithTx(tx)
		if err := tokenRepo.RevokeByTokenHash(tokenHash); err != nil {
			return pkg.InternalError(err)
		}

		session, err := s.issueSession(tokenRepo, tokenRecord.UserID)
		if err != nil {
			return err
		}

		result = RefreshResult{
			Token:        session.Token,
			RefreshToken: session.RefreshToken,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *AuthService) Logout(userID string, refreshToken string) error {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return err
	}

	tokenHash := pkg.HashToken(refreshToken)
	tokenRecord, err := s.tokenRepo.FindByTokenHash(tokenHash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidRefreshToken, "invalid refresh token")
		}
		return pkg.InternalError(err)
	}
	if tokenRecord.UserID != userUUID || !tokenRecord.ExpiresAt.After(time.Now().UTC()) {
		return pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidRefreshToken, "invalid refresh token")
	}

	if err := s.tokenRepo.RevokeByTokenHash(tokenHash); err != nil {
		return pkg.InternalError(err)
	}

	return nil
}

func (s *AuthService) ChangePassword(userID string, oldPassword string, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	userUUID, err := parseUUID(userID)
	if err != nil {
		return err
	}

	user, err := s.userRepo.FindByID(userUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return pkg.InternalError(err)
	}
	if err := pkg.ComparePassword(user.PasswordHash, oldPassword); err != nil {
		return pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidCredentials, "invalid credentials")
	}

	newHash, err := pkg.HashPassword(newPassword)
	if err != nil {
		return pkg.InternalError(err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		userRepo := s.userRepo.WithTx(tx)
		tokenRepo := s.tokenRepo.WithTx(tx)

		if err := userRepo.UpdatePassword(userUUID, newHash); err != nil {
			return pkg.InternalError(err)
		}
		if err := tokenRepo.RevokeAllByUserID(userUUID); err != nil {
			return pkg.InternalError(err)
		}
		return nil
	})
}

func (s *AuthService) UpdateUsername(userID string, username string) error {
	username = normalizeUsername(username)
	if err := validateUsername(username); err != nil {
		return err
	}

	userUUID, err := parseUUID(userID)
	if err != nil {
		return err
	}

	user, err := s.userRepo.FindByID(userUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return pkg.InternalError(err)
	}
	if user.Username == username {
		return nil
	}

	if err := s.userRepo.UpdateUsername(userUUID, username); err != nil {
		if appErr := mapUserUniqueViolation(err); appErr != nil {
			return appErr
		}
		return pkg.InternalError(err)
	}

	return nil
}

func (s *AuthService) Me(userID string) (*MeResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.FindByID(userUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return nil, pkg.InternalError(err)
	}

	return &MeResult{
		UserID:    user.ID.String(),
		Email:     user.Email,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
	}, nil
}

func (s *AuthService) findUserByIdentifier(identifier string) (*model.User, error) {
	email := normalizeEmail(identifier)
	if validateEmail(email) {
		user, err := s.userRepo.FindByEmail(email)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.InternalError(err)
		}
	}

	username := normalizeUsername(identifier)
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeInvalidCredentials, "invalid credentials")
		}
		return nil, pkg.InternalError(err)
	}

	return user, nil
}

type issuedSession struct {
	Token        string
	RefreshToken string
}

func (s *AuthService) issueSession(tokenRepo *repository.TokenRepository, userID uuid.UUID) (*issuedSession, error) {
	token, err := pkg.GenerateAccessToken(s.jwtSecret, userID.String(), s.accessTTL)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	refreshToken, err := pkg.GenerateOpaqueToken(48)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	record := &model.RefreshToken{
		UserID:    userID,
		TokenHash: pkg.HashToken(refreshToken),
		ExpiresAt: time.Now().UTC().Add(s.refreshTTL),
	}
	if err := tokenRepo.Create(record); err != nil {
		return nil, pkg.InternalError(err)
	}

	return &issuedSession{
		Token:        token,
		RefreshToken: refreshToken,
	}, nil
}

func mapUserUniqueViolation(err error) *pkg.AppError {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}

	switch pgErr.ConstraintName {
	case "idx_users_email":
		return pkg.NewAppError(http.StatusConflict, pkg.CodeEmailAlreadyExists, "email already exists")
	case "idx_users_username":
		return pkg.NewAppError(http.StatusConflict, pkg.CodeUsernameAlreadyExists, "username already exists")
	default:
		return nil
	}
}
