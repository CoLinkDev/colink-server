package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) WithTx(tx *gorm.DB) *UserRepository {
	return &UserRepository{db: tx}
}

func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	if err := r.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) FindByID(userID uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) UpdatePassword(userID uuid.UUID, passwordHash string) error {
	return r.db.Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{"password_hash": passwordHash}).
		Error
}
