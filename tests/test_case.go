package tests

import (
	"github.com/goravel/framework/testing"

	"ponta_drive/bootstrap"
)

func init() {
	bootstrap.Boot()
}

type TestCase struct {
	testing.TestCase
}
