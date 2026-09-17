package commands

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
)

type CreateUser struct {
}

// Signature The name and signature of the console command.
func (r *CreateUser) Signature() string {
	return "user:create"
}

// Description The console command description.
func (r *CreateUser) Description() string {
	return "Create a new user"
}

// Extend The console command extend.
func (r *CreateUser) Extend() command.Extend {
	return command.Extend{
		Category: "user",
		Flags: []command.Flag{
			&command.StringFlag{
				Name:    "username",
				Aliases: []string{"u"},
				Usage:   "User username",
			},
			&command.StringFlag{
				Name:    "email",
				Aliases: []string{"e"},
				Usage:   "User email",
			},
			&command.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "User display name",
			},
			&command.StringFlag{
				Name:    "password",
				Aliases: []string{"p"},
				Usage:   "User password",
			},
			&command.StringFlag{
				Name:    "avatar",
				Aliases: []string{"a"},
				Usage:   "User avatar URL",
			},
		},
	}
}

// Handle Execute the console command.
func (r *CreateUser) Handle(ctx console.Context) error {
	username := strings.TrimSpace(ctx.Option("username"))
	email := strings.TrimSpace(ctx.Option("email"))
	name := strings.TrimSpace(ctx.Option("name"))
	password := ctx.Option("password")
	avatar := strings.TrimSpace(ctx.Option("avatar"))

	var err error
	if username == "" {
		username, err = ctx.Ask("Enter username:")
		if err != nil || strings.TrimSpace(username) == "" {
			ctx.Error("Username is required")
			return nil
		}
		username = strings.TrimSpace(username)
	}

	if email == "" {
		email, err = ctx.Ask("Enter email:")
		if err != nil || strings.TrimSpace(email) == "" {
			ctx.Error("Email is required")
			return nil
		}
		email = strings.TrimSpace(email)
	}

	if name == "" {
		name, err = ctx.Ask("Enter display name (optional, press Enter to use username):")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = username
		}
	}

	if password == "" {
		password, err = ctx.Secret("Enter password:")
		if err != nil || password == "" {
			ctx.Error("Password is required")
			return nil
		}
	}

	// Check if username or email exists
	count, err := facades.Orm().Query().Model(&models.User{}).
		Where("username = ? OR email = ?", username, email).
		Count()
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to check existing user: %v", err))
		return err
	}
	if count > 0 {
		ctx.Error("User with this username or email already exists")
		return nil
	}

	// Hash password
	hashedPassword, err := facades.Hash().Make(password)
	if err != nil {
		ctx.Error(fmt.Sprintf("Failed to hash password: %v", err))
		return err
	}

	user := models.User{
		UUID:     uuid.New().String(),
		Username: username,
		Email:    email,
		Name:     name,
		Password: hashedPassword,
		Avatar:   avatar,
	}

	// If avatar is empty, generate from ui-avatars.com
	if user.Avatar == "" {
		user.Avatar = user.GetAvatar()
	}

	if err := facades.Orm().Query().Create(&user); err != nil {
		ctx.Error(fmt.Sprintf("Failed to create user: %v", err))
		return err
	}

	ctx.Success(fmt.Sprintf("User [%s] (%s) created successfully! (UUID: %s)", user.Username, user.Email, user.UUID))
	return nil
}
