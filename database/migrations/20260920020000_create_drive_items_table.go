package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

// M20260920020000CreateDriveItemsTable creates the `drive_items` table (spec 2.2).
type M20260920020000CreateDriveItemsTable struct{}

// Signature The unique signature for the migration.
func (r *M20260920020000CreateDriveItemsTable) Signature() string {
	return "20260920020000_create_drive_items_table"
}

// Up Run the migrations.
func (r *M20260920020000CreateDriveItemsTable) Up() error {
	if !facades.Schema().HasTable("drive_items") {
		return facades.Schema().Create("drive_items", func(table schema.Blueprint) {
			table.ID()
			table.String("uuid", 36)
			table.BigInteger("user_id")
			table.BigInteger("cloud_account_id")
			table.BigInteger("parent_id").Nullable()
			table.String("name", 255)
			table.String("type", 20)
			table.String("mime_type", 100).Nullable()
			table.BigInteger("size").Default(0)
			table.String("extension", 50).Nullable()
			table.Text("storage_path").Nullable()
			table.String("etag", 100).Nullable()
			table.Boolean("is_starred").Default(false)
			table.String("status", 20).Default("ready")
			table.Timestamps()
			table.SoftDeletes()

			table.Unique("uuid")
			table.Index("user_id")
			table.Index("cloud_account_id")
			table.Index("parent_id")
			table.Index("type")
			table.Index("is_starred")
			table.Index("status")
		})
	}

	return nil
}

// Down Reverse the migrations.
func (r *M20260920020000CreateDriveItemsTable) Down() error {
	return facades.Schema().DropIfExists("drive_items")
}
