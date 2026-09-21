package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

// M20260920120000CreateActivityLogsTable creates the `activity_logs` table used
// as an audit trail of user actions shown on the home dashboard.
//
// Notes:
//   - `user_id` is an indexed BIGINT referencing users.id. No hard FOREIGN KEY
//     constraint is added, mirroring the convention used by the existing
//     `cloud_accounts` / `drive_items` migrations (FK integrity is enforced at
//     the application layer).
//   - `target_uuid` is a nullable CHAR(36)-style string so the same table can
//     log actions against drive items, cloud accounts and other UUID-keyed
//     resources.
//   - `metadata` is a nullable JSON column for flexible, action-specific extra
//     data without requiring schema changes.
//   - A composite index on (user_id, created_at) supports the dashboard's
//     "recent activity for this user" query pattern.
type M20260920120000CreateActivityLogsTable struct{}

// Signature The unique signature for the migration.
func (r *M20260920120000CreateActivityLogsTable) Signature() string {
	return "20260920120000_create_activity_logs_table"
}

// Up Run the migrations.
func (r *M20260920120000CreateActivityLogsTable) Up() error {
	if !facades.Schema().HasTable("activity_logs") {
		return facades.Schema().Create("activity_logs", func(table schema.Blueprint) {
			table.ID()
			table.BigInteger("user_id")
			table.String("action", 50)
			table.String("target_name", 255)
			table.String("target_uuid", 36).Nullable()
			table.BigInteger("cloud_account_id").Nullable()
			table.String("ip_address", 45)
			table.Text("user_agent")
			table.Json("metadata").Nullable()
			table.Timestamps()

			table.Index("user_id")
			table.Index("ip_address")
			table.Index("user_id", "created_at").Name("idx_user_created")
		})
	}

	return nil
}

// Down Reverse the migrations.
func (r *M20260920120000CreateActivityLogsTable) Down() error {
	return facades.Schema().DropIfExists("activity_logs")
}
