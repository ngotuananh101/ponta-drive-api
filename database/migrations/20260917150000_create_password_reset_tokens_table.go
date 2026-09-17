package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

type M20260917150000CreatePasswordResetTokensTable struct{}

func (r *M20260917150000CreatePasswordResetTokensTable) Signature() string {
	return "20260917150000_create_password_reset_tokens_table"
}

func (r *M20260917150000CreatePasswordResetTokensTable) Up() error {
	if !facades.Schema().HasTable("password_reset_tokens") {
		return facades.Schema().Create("password_reset_tokens", func(table schema.Blueprint) {
			table.String("email", 255)
			table.String("token", 255)
			table.Timestamp("created_at").Nullable()

			table.Index("email")
		})
	}

	return nil
}

func (r *M20260917150000CreatePasswordResetTokensTable) Down() error {
	return facades.Schema().DropIfExists("password_reset_tokens")
}
