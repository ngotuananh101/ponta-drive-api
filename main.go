package main

import (
	"ponta_drive/bootstrap"
)

func main() {
	app := bootstrap.Boot()

	app.Start()
}
