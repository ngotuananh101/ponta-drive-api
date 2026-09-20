package feature

import (
	"context"
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

func (s *ProbeCryptTestSuite) TestLangResolvesNewFiles() {
	// Default locale: vi
	ctxVi := context.Background()
	s.Equal("Chưa xác thực", facades.Lang(ctxVi).Get("common.unauthorized"))
	s.Equal("Kết nối thành công", facades.Lang(ctxVi).Get("cloud.connection_success"))
	s.Equal("Mục đã được xóa thành công", facades.Lang(ctxVi).Get("drive.item_deleted"))
	s.Equal("Đã hủy phiên tải lên đa phần thành công", facades.Lang(ctxVi).Get("upload.aborted"))

	// Explicit locale: en
	ctxEn := facades.Lang(context.Background()).SetLocale("en")
	s.Equal("Unauthorized", facades.Lang(ctxEn).Get("common.unauthorized"))
	s.Equal("Connection successful", facades.Lang(ctxEn).Get("cloud.connection_success"))
	s.Equal("Drive item deleted successfully", facades.Lang(ctxEn).Get("drive.item_deleted"))
	s.Equal("Multipart upload aborted successfully", facades.Lang(ctxEn).Get("upload.aborted"))
}
