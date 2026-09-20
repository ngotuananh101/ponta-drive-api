package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

// M20260920010000CreateCloudAccountsTable creates the `cloud_accounts` table
// that stores each user's personal cloud storage connections (spec 2.1).
//
// Notes on spec mapping:
//   - `user_id` is implemented as an indexed BIGINT referencing users.id. No hard
//     FOREIGN KEY constraint is added, mirroring the convention used by the
//     existing `users` / `password_reset_tokens` migrations (FK integrity is
//     enforced at the application layer in ponta_drive).
//   - `provider` is indexed (NOT unique): a single user may connect several
//     accounts of the same provider (e.g. two AWS S3 buckets with different
//     names). This deliberately diverges from the brief's example of a unique
//     index on `provider`, which would contradict spec 2.1.
//   - `credentials` is a TEXT column storing the AES-256 encrypted JSON payload
//     (see models.CloudAccount.SetCredentials/GetCredentials).
//   - A unique index on (user_id, name) prevents a user from creating two
//     cloud accounts that share a display name.
type M20260920010000CreateCloudAccountsTable struct{}

// Signature The unique signature for the migration.
func (r *M20260920010000CreateCloudAccountsTable) Signature() string {
	return "20260920010000_create_cloud_accounts_table"
}

// Up Run the migrations.
func (r *M20260920010000CreateCloudAccountsTable) Up() error {
	if !facades.Schema().HasTable("cloud_accounts") {
		return facades.Schema().Create("cloud_accounts", func(table schema.Blueprint) {
			table.ID()
			table.BigInteger("user_id")
			table.String("name", 100)
			table.String("provider", 50)
			table.Text("credentials")
			table.Boolean("is_default").Default(false)
			table.Boolean("is_active").Default(true)
			table.BigInteger("total_storage").Default(0)
			table.BigInteger("used_storage").Default(0)
			table.Timestamps()
			table.SoftDeletes()

			table.Index("user_id")
			table.Index("provider")
			table.Unique("user_id", "name")
		})
	}

	return nil
}

// Down Reverse the migrations.
func (r *M20260920010000CreateCloudAccountsTable) Down() error {
	return facades.Schema().DropIfExists("cloud_accounts")
}
