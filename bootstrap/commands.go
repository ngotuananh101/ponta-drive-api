package bootstrap

import (
	"github.com/goravel/framework/contracts/console"

	"ponta_drive/app/console/commands"
)

func Commands() []console.Command {
	return []console.Command{
		&commands.CreateUser{},
	}
}
