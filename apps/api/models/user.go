package models

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex;not null" json:"username"`
	Email     string    `gorm:"uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName sets the table name
func (User) TableName() string {
	return "users"
}

// BeforeSave hook to hash password
func (u *User) BeforeSave(tx *gorm.DB) error {
	if tx.Statement.Changed("Password") {
		hash := sha256.Sum256([]byte(u.Password))
		u.Password = hex.EncodeToString(hash[:])
	}
	return nil
}

// VerifyPassword checks if the provided password matches the stored hash
func (u *User) VerifyPassword(password string) bool {
	hash := sha256.Sum256([]byte(password))
	return u.Password == hex.EncodeToString(hash[:])
}

// UserClaims represents JWT claims
type UserClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}
