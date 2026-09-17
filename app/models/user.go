package models

import (
	"net/url"
	"strings"

	"github.com/goravel/framework/database/orm"
)

type User struct {
	orm.Model
	UUID     string `gorm:"column:uuid;uniqueIndex;size:36" json:"uuid"`
	Name     string `gorm:"column:name;size:255" json:"name"`
	Username string `gorm:"column:username;uniqueIndex;size:100" json:"username"`
	Email    string `gorm:"column:email;uniqueIndex;size:255" json:"email"`
	Password string `gorm:"column:password;size:255" json:"-"`
	Avatar   string `gorm:"column:avatar;size:255" json:"avatar"`
	orm.SoftDeletes
}

func (u *User) GetAvatar() string {
	if strings.TrimSpace(u.Avatar) != "" {
		return u.Avatar
	}
	name := strings.TrimSpace(u.Name)
	if name == "" {
		name = u.Username
	}
	if name == "" {
		name = "User"
	}
	return "https://ui-avatars.com/api/?name=" + url.QueryEscape(name) + "&background=random"
}

func (u *User) ToResponse() map[string]any {
	return map[string]any{
		"id":         u.ID,
		"uuid":       u.UUID,
		"name":       u.Name,
		"username":   u.Username,
		"email":      u.Email,
		"avatar":     u.GetAvatar(),
		"created_at": u.CreatedAt,
		"updated_at": u.UpdatedAt,
	}
}
