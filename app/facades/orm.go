package facades

import (
	"fmt"

	contractsbinding "github.com/goravel/framework/contracts/binding"
	"github.com/goravel/framework/contracts/database/orm"
)

func Orm() orm.Orm {
	return App().MakeOrm()
}

// SafeQuery returns a valid orm.Query instance. If the database connection was
// not available during server boot, it attempts to reconnect automatically.
func SafeQuery() (orm.Query, error) {
	instance := Orm()
	if instance == nil || instance.Query() == nil {
		// Invalidate cached singleton to retry initialization in case DB was started after boot
		App().Fresh(contractsbinding.Orm)
		instance = Orm()
	}

	if instance == nil {
		return nil, fmt.Errorf("không thể kết nối tới cơ sở dữ liệu")
	}

	query := instance.Query()
	if query == nil {
		return nil, fmt.Errorf("không thể kết nối tới cơ sở dữ liệu")
	}

	return query, nil
}
