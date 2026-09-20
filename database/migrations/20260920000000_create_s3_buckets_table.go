package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/app/facades"
)

type M20260920000000CreateS3BucketsTable struct{}

// Signature The unique signature for the migration.
func (r *M20260920000000CreateS3BucketsTable) Signature() string {
	return "20260920000000_create_s3_buckets_table"
}

// Up Run the migrations - Placeholder for S3 multi-cloud storage setup
func (r *M20260920000000CreateS3BucketsTable) Up() error {
	// Placeholder migration - will be implemented in future tasks
	if !facades.Schema().HasTable("s3_buckets") {
		return facades.Schema().Create("s3_buckets", func(table schema.Blueprint) {
			table.ID()
			table.String("uuid", 36)
			table.String("name", 255)
			table.String("bucket", 255)
			table.String("provider", 50)
			table.Text("config").Nullable()
			table.Timestamps()
			table.SoftDeletes()

			table.Unique("uuid")
			table.Unique("bucket")
		})
	}

	return nil
}

// Down Reverse the migrations - Placeholder for S3 multi-cloud storage setup
func (r *M20260920000000CreateS3BucketsTable) Down() error {
	return facades.Schema().DropIfExists("s3_buckets")
}