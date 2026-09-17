package models

import (
	"github.com/goravel/framework/support/carbon"
)

type PasswordResetToken struct {
	Email     string           `gorm:"column:email;size:255;index" json:"email"`
	Token     string           `gorm:"column:token;size:255" json:"token"`
	CreatedAt *carbon.DateTime `gorm:"autoCreateTime;column:created_at" json:"created_at"`
}

func (r *PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}
