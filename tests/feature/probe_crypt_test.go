package feature

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/tests"
)

type ProbeCryptTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestProbeCryptTestSuite(t *testing.T) {
	suite.Run(t, new(ProbeCryptTestSuite))
}

func (s *ProbeCryptTestSuite) TestCryptRoundTrip() {
	enc, err := facades.Crypt().EncryptString("super-secret-access-key-123")
	s.Require().NoError(err)
	s.NotEqual("super-secret-access-key-123", enc)
	s.NotContains(enc, "access-key")

	dec, err := facades.Crypt().DecryptString(enc)
	s.Require().NoError(err)
	s.Equal("super-secret-access-key-123", dec)
}
