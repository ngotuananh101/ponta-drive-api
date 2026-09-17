package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

type M20260917135741CreateUsersTable struct{}

// Signature The unique signature for the migration.
func (r *M20260917135741CreateUsersTable) Signature() string {
	return "20260917135741_create_users_table"
}

// Up Run the migrations.
func (r *M20260917135741CreateUsersTable) Up() error {
	if !facades.Schema().HasTable("users") {
		return facades.Schema().Create("users", func(table schema.Blueprint) {
			table.ID()
			table.String("uuid", 36)
			table.String("name", 255)
			table.String("username", 100)
			table.String("email", 255)
			table.String("password", 255)
			table.String("avatar", 255).Nullable()
			table.Timestamps()
			table.SoftDeletes()

			table.Unique("uuid")
			table.Unique("username")
			table.Unique("email")
		})
	}

	return nil
}

// Down Reverse the migrations.
func (r *M20260917135741CreateUsersTable) Down() error {
	return facades.Schema().DropIfExists("users")
}
