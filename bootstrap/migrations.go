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
	}
}
