package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

// M20260920120100AddSyncStatusToCloudAccounts adds sync tracking columns to the
// existing `cloud_accounts` table so the dashboard can show whether a cloud
// account is currently being synchronised and when it was last synced.
//
// Notes:
//   - `sync_status` is an ENUM of idle/syncing/error, defaulting to `idle`, so
//     new and pre-existing rows are immediately valid.
//   - `last_synced_at` is nullable: NULL means the account has never been
//     synced.
type M20260920120100AddSyncStatusToCloudAccounts struct{}

// Signature The unique signature for the migration.
func (r *M20260920120100AddSyncStatusToCloudAccounts) Signature() string {
	return "20260920120100_add_sync_status_to_cloud_accounts"
}

// Up Run the migrations.
func (r *M20260920120100AddSyncStatusToCloudAccounts) Up() error {
	return facades.Schema().Table("cloud_accounts", func(table schema.Blueprint) {
		table.Enum("sync_status", []any{"idle", "syncing", "error"}).Default("idle").After("is_active")
		table.Timestamp("last_synced_at").Nullable().After("sync_status")
	})
}

// Down Reverse the migrations.
func (r *M20260920120100AddSyncStatusToCloudAccounts) Down() error {
	return facades.Schema().Table("cloud_accounts", func(table schema.Blueprint) {
		table.DropColumn("sync_status")
		table.DropColumn("last_synced_at")
	})
}
