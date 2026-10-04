package migrations

import (
	"github.com/google/uuid"
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

// M20261004000000AddUUIDToCloudAccounts adds the public uuid column to
// `cloud_accounts`. Numeric `id` stays the internal key; `uuid` is what the API
// and the drive URL expose.
//
// Existing rows are backfilled here so no account is served with a blank uuid.
type M20261004000000AddUUIDToCloudAccounts struct{}

// Signature The unique signature for the migration.
func (r *M20261004000000AddUUIDToCloudAccounts) Signature() string {
	return "20261004000000_add_uuid_to_cloud_accounts"
}

// Up Run the migrations.
func (r *M20261004000000AddUUIDToCloudAccounts) Up() error {
	if !facades.Schema().HasTable("cloud_accounts") {
		return nil
	}

	if !facades.Schema().HasColumn("cloud_accounts", "uuid") {
		if err := facades.Schema().Table("cloud_accounts", func(table schema.Blueprint) {
			table.String("uuid", 36).Nullable().After("id")
		}); err != nil {
			return err
		}
	}

	// Backfill: one uuid per existing row.
	var ids []uint
	if err := facades.Orm().Query().Table("cloud_accounts").
		Where("uuid IS NULL OR uuid = ''").
		Pluck("id", &ids); err != nil {
		return err
	}
	for _, id := range ids {
		_, _ = facades.Orm().Query().Table("cloud_accounts").
			Where("id", id).
			Update("uuid", uuid.New().String())
	}

	if !facades.Schema().HasIndex("cloud_accounts", "cloud_accounts_uuid_unique") {
		if err := facades.Schema().Table("cloud_accounts", func(table schema.Blueprint) {
			table.Unique("uuid")
		}); err != nil {
			return err
		}
	}

	return nil
}

// Down Reverse the migrations.
func (r *M20261004000000AddUUIDToCloudAccounts) Down() error {
	return facades.Schema().Table("cloud_accounts", func(table schema.Blueprint) {
		table.DropColumn("uuid")
	})
}
