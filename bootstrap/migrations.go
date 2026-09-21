package bootstrap

import (
	"github.com/goravel/framework/contracts/database/schema"

	"ponta_drive/database/migrations"
)

func Migrations() []schema.Migration {
	return []schema.Migration{
		&migrations.M20210101000001CreateJobsTable{},
		&migrations.M20260917135741CreateUsersTable{},
		&migrations.M20260917150000CreatePasswordResetTokensTable{},
		&migrations.M20260920000000CreateS3BucketsTable{},         // Placeholder for S3 multi-cloud storage
		&migrations.M20260920010000CreateCloudAccountsTable{},     // Multi-cloud storage accounts (spec 2.1)
		&migrations.M20260920020000CreateDriveItemsTable{},        // Virtual hierarchy items (spec 2.2)
		&migrations.M20260920030000CreateUploadSessionsTable{},    // Multipart upload sessions (spec 2.3)
		&migrations.M20260920120000CreateActivityLogsTable{},      // User activity audit trail (home dashboard)
		&migrations.M20260920120100AddSyncStatusToCloudAccounts{}, // Sync status tracking for cloud accounts
	}
}
