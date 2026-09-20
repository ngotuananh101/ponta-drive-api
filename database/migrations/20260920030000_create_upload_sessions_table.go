package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

// M20260920030000CreateUploadSessionsTable creates the `upload_sessions` table (spec 2.3).
type M20260920030000CreateUploadSessionsTable struct{}

// Signature The unique signature for the migration.
func (r *M20260920030000CreateUploadSessionsTable) Signature() string {
	return "20260920030000_create_upload_sessions_table"
}

// Up Run the migrations.
func (r *M20260920030000CreateUploadSessionsTable) Up() error {
	if !facades.Schema().HasTable("upload_sessions") {
		return facades.Schema().Create("upload_sessions", func(table schema.Blueprint) {
			table.ID()
			table.String("session_id", 36)
			table.String("upload_id", 255)
			table.BigInteger("user_id")
			table.BigInteger("cloud_account_id")
			table.BigInteger("parent_id").Nullable()
			table.String("file_name", 255)
			table.String("mime_type", 100).Nullable()
			table.BigInteger("total_size")
			table.BigInteger("chunk_size")
			table.Integer("total_parts")
			table.Text("uploaded_parts")
			table.Text("storage_path")
			table.DateTime("expires_at")
			table.Timestamps()

			table.Unique("session_id")
			table.Index("user_id")
			table.Index("cloud_account_id")
			table.Index("expires_at")
		})
	}

	return nil
}

// Down Reverse the migrations.
func (r *M20260920030000CreateUploadSessionsTable) Down() error {
	return facades.Schema().DropIfExists("upload_sessions")
}
