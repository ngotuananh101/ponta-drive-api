package feature

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"ponta_drive/app/facades"
	"ponta_drive/app/models"
	"ponta_drive/app/services"
	"ponta_drive/tests"
)

type DrivePaginationTestSuite struct {
	suite.Suite
	tests.TestCase
	user    models.User
	token   string
	account models.CloudAccount
}

func TestDrivePaginationTestSuite(t *testing.T) {
	suite.Run(t, new(DrivePaginationTestSuite))
}

func (s *DrivePaginationTestSuite) SetupTest() {
	_ = facades.Artisan().Call("migrate")

	hashedPassword, _ := facades.Hash().Make("password123")
	s.user = models.User{
		UUID:     uuid.New().String(),
		Name:     "Pagination User",
		Username: "page_" + uuid.New().String()[:8],
		Email:    "page_" + uuid.New().String()[:8] + "@example.com",
		Password: hashedPassword,
	}
	err := facades.Orm().Query().Create(&s.user)
	s.Require().NoError(err)

	loginPayload, _ := json.Marshal(map[string]string{
		"email":    s.user.Email,
		"password": "password123",
	})
	resp, err := s.Http(s.T()).Post("/auth/login", bytes.NewBuffer(loginPayload))
	s.Require().NoError(err)
	resp.AssertOk()

	jsonBody, err := resp.Json()
	s.Require().NoError(err)
	s.token = jsonBody["data"].(map[string]any)["token"].(string)

	s.account = models.CloudAccount{
		UserID:    s.user.ID,
		Name:      "Pagination Storage",
		Provider:  models.ProviderS3,
		IsDefault: true,
		IsActive:  true,
	}
	_ = s.account.SetCredentials(&models.S3Credentials{
		Bucket:          "pagination-bucket",
		Region:          "us-east-1",
		AccessKeyID:     "AKIA...",
		SecretAccessKey: "secret...",
	})
	err = facades.Orm().Query().Create(&s.account)
	s.Require().NoError(err)
}

func (s *DrivePaginationTestSuite) TearDownTest() {
	if s.user.ID > 0 {
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.DriveItem{})
		_, _ = facades.Orm().Query().Where("user_id", s.user.ID).ForceDelete(&models.CloudAccount{})
		_, _ = facades.Orm().Query().ForceDelete(&s.user)
	}
}

// seedItems creates n files at the root of s.account, all with the same
// updated_at second so only the `id` tiebreaker can order them.
func (s *DrivePaginationTestSuite) seedItems(n int) []models.DriveItem {
	items := make([]models.DriveItem, 0, n)
	for i := 0; i < n; i++ {
		item := models.DriveItem{
			UUID:           uuid.New().String(),
			UserID:         s.user.ID,
			CloudAccountID: s.account.ID,
			Name:           fmt.Sprintf("file-%03d.txt", i),
			Type:           models.ItemTypeFile,
			Size:           int64(i + 1),
			Status:         models.ItemStatusReady,
		}
		s.Require().NoError(facades.Orm().Query().Create(&item))
		items = append(items, item)
	}
	return items
}

func (s *DrivePaginationTestSuite) TestCursorRoundTrip() {
	item := models.DriveItem{
		Name: "report.pdf",
		Type: models.ItemTypeFile,
		Size: 4096,
	}
	item.ID = 77

	encoded, err := services.EncodeDriveCursor(item, "name", "asc")
	s.Require().NoError(err)
	s.NotEmpty(encoded)

	decoded, err := services.DecodeDriveCursor(encoded)
	s.Require().NoError(err)
	s.Equal(models.ItemTypeFile, decoded.Type)
	s.Equal("report.pdf", decoded.Value)
	s.Equal(uint(77), decoded.ID)
	s.Equal("name", decoded.Sort)
	s.Equal("asc", decoded.Order)
	s.True(decoded.Matches("name", "asc"))
	s.False(decoded.Matches("size", "asc"), "a cursor must not match a different ordering")
}

// A cursor that is not valid base64, or decodes to JSON of the wrong shape,
// must produce an error rather than silently paginating from the beginning.
// The empty string is excluded: that is how a caller asks for the first page.
func (s *DrivePaginationTestSuite) TestDecodeRejectsGarbage() {
	for _, bad := range []string{"not-base64!!!", "Zm9vYmFy", "eyJ0IjoiZmlsZSIsInM6InNhbWUifQ"} {
		_, err := services.DecodeDriveCursor(bad)
		s.Error(err, "garbage cursor %q must be rejected", bad)
	}
}

// The empty cursor is the documented way to request the first page, so it must
// decode cleanly to the zero value rather than erroring.
func (s *DrivePaginationTestSuite) TestDecodeEmptyCursorMeansFirstPage() {
	c, err := services.DecodeDriveCursor("")
	s.Require().NoError(err)
	s.Equal(uint(0), c.ID)
	s.False(c.Matches("name", "asc"))
}

func (s *DrivePaginationTestSuite) TestNormalizeSortFieldWhitelists() {
	s.Equal("name", services.NormalizeSortField("name"))
	s.Equal("size", services.NormalizeSortField("size"))
	s.Equal("updated_at", services.NormalizeSortField("updated_at"))
	s.Equal("created_at", services.NormalizeSortField("created_at"))
	s.Equal("name", services.NormalizeSortField(""))
	// A hostile value must not reach the SQL string.
	s.Equal("name", services.NormalizeSortField("name; DROP TABLE users"))
	s.Equal("name", services.NormalizeSortField("password"))
}

func (s *DrivePaginationTestSuite) TestNormalizeSortOrder() {
	s.Equal("asc", services.NormalizeSortOrder("asc"))
	s.Equal("desc", services.NormalizeSortOrder("desc"))
	s.Equal("asc", services.NormalizeSortOrder(""))
	s.Equal("asc", services.NormalizeSortOrder("sideways"))
}

// A NULL timestamp sorts before every value, so a cursor resting on one carries
// an empty value. Binding `col > ”` would be false for every row and the walk
// would lose everything after it, so the column comparison has to be dropped
// rather than bound. The predicate must still be a syntactically valid
// expression, which is why it is asserted here and not only through HTTP.
func (s *DrivePaginationTestSuite) TestCursorPredicateDropsColumnComparisonForNullTimestamp() {
	cond, args := services.BuildCursorWhere(services.DriveCursor{
		Type: models.ItemTypeFolder,
		ID:   12,
	}, "updated_at", "asc")

	s.NotContains(cond, "updated_at", "an empty value must not produce a column comparison")
	s.Contains(cond, "id > ?", "the id tier must still carry the walk")
	s.Len(args, 3, "type, type, id")

	cond, args = services.BuildCursorWhere(services.DriveCursor{
		Type:  models.ItemTypeFile,
		Value: "2026-10-01 00:00:00",
		ID:    12,
	}, "updated_at", "asc")

	s.Contains(cond, "updated_at > ?")
	s.Len(args, 5, "type, type, value, value, id")
}

// listPage calls the list endpoint and returns the page's names plus the
// pagination metadata.
func (s *DrivePaginationTestSuite) listPage(query string) ([]string, bool, string) {
	resp, err := s.Http(s.T()).WithToken(s.token).
		Get(fmt.Sprintf("/v1/drive/items?cloud_account_uuid=%s%s", s.account.UUID, query))
	s.Require().NoError(err)
	resp.AssertOk()

	body, err := resp.Json()
	s.Require().NoError(err)

	raw := body["data"].([]any)
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		names = append(names, entry.(map[string]any)["name"].(string))
	}

	meta, ok := body["meta"].(map[string]any)
	s.Require().True(ok, "response must carry a meta object")
	hasMore := meta["has_more"].(bool)
	next, _ := meta["next_cursor"].(string)

	return names, hasMore, next
}

// walkAllPages follows next_cursor until has_more is false and returns every
// name seen, in order.
func (s *DrivePaginationTestSuite) walkAllPages(limit int) []string {
	return s.walkAllPagesUnder("", limit)
}

// walkAllPagesUnder is walkAllPages with extra query parameters (sort/order),
// so the same traversal can be exercised under each ordering the UI can send.
func (s *DrivePaginationTestSuite) walkAllPagesUnder(params string, limit int) []string {
	var all []string
	query := fmt.Sprintf("%s&limit=%d", params, limit)

	for page := 0; page < 50; page++ {
		names, hasMore, next := s.listPage(query)
		all = append(all, names...)

		if !hasMore {
			return all
		}
		s.Require().NotEmpty(next, "has_more=true must come with a next_cursor")
		query = fmt.Sprintf("%s&limit=%d&cursor=%s", params, limit, next)
	}

	s.FailNow("pagination did not terminate after 50 pages")
	return nil
}

// Walking every page must visit each item exactly once. A missing id tiebreaker
// makes rows with equal sort values shuffle between pages, which shows up here
// as duplicates or omissions.
func (s *DrivePaginationTestSuite) TestPaginationVisitsEveryItemExactlyOnce() {
	s.seedItems(25)

	seen := s.walkAllPages(10)

	s.Len(seen, 25, "every seeded item must appear exactly once")
	s.Equal(uniqueStrings(seen), seen, "no item may appear twice")
}

// The seeded items all share an updated_at second, so this is the case that
// only a total ordering survives.
func (s *DrivePaginationTestSuite) TestPaginationStableWhenTimestampsTie() {
	s.seedItems(15)

	seen := s.walkAllPages(4)

	s.Len(seen, 15)
	s.Equal(uniqueStrings(seen), seen)
}

// A descending order flips the comparison in the cursor predicate. If the
// predicate kept `>` for a desc sort, the second page would re-read rows the
// first page already returned; if it kept the column comparison but dropped the
// id tiebreaker, rows sharing a sort value would shuffle across the page
// boundary. Either way this shows up as a duplicate or a missing name.
func (s *DrivePaginationTestSuite) TestPaginationUnderDescendingOrders() {
	s.seedItems(25)

	for _, params := range []string{"&sort=name&order=desc", "&sort=size&order=desc"} {
		seen := s.walkAllPagesUnder(params, 10)

		s.Len(seen, 25, "every seeded item must appear exactly once under %s", params)
		s.Equal(uniqueStrings(seen), seen, "no item may appear twice under %s", params)
	}
}

// Folders must still sort before files after the CASE expression is replaced
// by ORDER BY type DESC.
func (s *DrivePaginationTestSuite) TestFoldersStillSortFirst() {
	folder := models.DriveItem{
		UUID:           uuid.New().String(),
		UserID:         s.user.ID,
		CloudAccountID: s.account.ID,
		Name:           "zzz-folder",
		Type:           models.ItemTypeFolder,
		Status:         models.ItemStatusReady,
	}
	s.Require().NoError(facades.Orm().Query().Create(&folder))
	s.seedItems(3)

	names, _, _ := s.listPage("")
	s.Require().NotEmpty(names)
	// "zzz-folder" sorts last alphabetically, so it can only be first if the
	// folder tier is doing the work.
	s.Equal("zzz-folder", names[0])
}

// Exactly `limit` items: there is no next page.
// One more item than `limit`: there is a next page.
func (s *DrivePaginationTestSuite) TestMetaReportsHasMoreAtTheBoundary() {
	// Exactly `limit` items: there is no next page.
	s.seedItems(5)
	names, hasMore, next := s.listPage("&limit=5")
	s.Len(names, 5)
	s.False(hasMore, "a page holding exactly `limit` items has no next page")
	s.Empty(next)

	// One more item than `limit`: there is a next page.
	s.seedItems(1)
	names, hasMore, next = s.listPage("&limit=5")
	s.Len(names, 5)
	s.True(hasMore, "a page with a further item must report has_more")
	s.NotEmpty(next)
}

// A cursor the client mangled must be rejected, not silently ignored. Ignoring
// it would restart the page from the top and an infinite scroller would never
// terminate.
func (s *DrivePaginationTestSuite) TestInvalidCursorIsRejected() {
	s.seedItems(3)

	// "!!" is not in the base64 alphabet, so decoding fails before any query.
	resp, err := s.Http(s.T()).WithToken(s.token).
		Get(fmt.Sprintf("/v1/drive/items?cloud_account_uuid=%s&cursor=!!", s.account.UUID))
	s.Require().NoError(err)
	resp.AssertStatus(http.StatusBadRequest)

	body, err := resp.Json()
	s.Require().NoError(err)
	s.Equal("error", body["status"])
}

// A caller asking for an unbounded page must be clamped, not obeyed.
func (s *DrivePaginationTestSuite) TestLimitIsClampedToTheCeiling() {
	s.seedItems(105)

	names, hasMore, _ := s.listPage("&limit=100000")
	s.Len(names, 100, "the ceiling is 100, so a 100000 request must come back as 100 rows")
	s.True(hasMore, "there are 105 rows, so a 100-row page has more")
}

// Below the ceiling the requested limit is honoured exactly.
func (s *DrivePaginationTestSuite) TestLimitBelowCeilingIsHonoured() {
	s.seedItems(20)

	names, hasMore, _ := s.listPage("&limit=7")
	s.Len(names, 7)
	s.True(hasMore)
}

// A cursor built under one ordering must not be replayed under another: the
// stored position would be compared against a column from a different sort.
// The service ignores it, so the request degrades to a first page.
func (s *DrivePaginationTestSuite) TestStaleCursorUnderDifferentSortFallsBackToFirstPage() {
	s.seedItems(10)

	_, hasMore, next := s.listPage("&limit=4&sort=name&order=asc")
	s.Require().True(hasMore)
	s.Require().NotEmpty(next)

	expected, _, _ := s.listPage("&limit=4&sort=name&order=desc")
	replayed, _, _ := s.listPage(
		fmt.Sprintf("&limit=4&sort=name&order=desc&cursor=%s", next))

	s.NotEmpty(expected)
	s.Equal(expected, replayed,
		"a cursor from another ordering must be ignored, not applied")
}

// An empty folder must produce an empty array, not null, so the client can
// iterate it without a guard.
func (s *DrivePaginationTestSuite) TestEmptyFolderReturnsEmptyArray() {
	names, hasMore, next := s.listPage("")
	s.Empty(names)
	s.False(hasMore)
	s.Empty(next)
}

// The index only helps if `type` is stored DESCENDING: an all-ascending
// composite index cannot serve `ORDER BY type DESC, name ASC` and MariaDB
// filesorts instead. Nothing in the code would fail if that column silently
// became ASC, so the direction is asserted here rather than left to a manual
// EXPLAIN. `collation` is 'D' for descending and 'A' for ascending in
// information_schema.statistics.
func (s *DrivePaginationTestSuite) TestListIndexStoresTypeDescending() {
	// `collation` is NULL for non-string columns (`user_id` is a bigint), so it
	// has to be scanned as nullable.
	type row struct {
		ColumnName string
		Collation  sql.NullString
		SeqInIndex int
	}

	var rows []row
	err := facades.Orm().Query().Raw(
		"SELECT column_name, collation, seq_in_index FROM information_schema.statistics " +
			"WHERE table_schema = DATABASE() AND table_name = 'drive_items' " +
			"AND index_name = 'idx_drive_items_list' ORDER BY seq_in_index",
	).Scan(&rows)
	s.Require().NoError(err)
	s.Require().Len(rows, 6, "the composite index must exist with six columns")

	for i, want := range []string{"user_id", "cloud_account_id", "parent_id", "type", "name", "id"} {
		s.Equal(want, rows[i].ColumnName, "column %d of the index", i+1)
	}

	s.Equal("D", rows[3].Collation.String,
		"`type` must be descending; an ascending column leaves the ORDER BY filesorting")

	for _, r := range append(rows[:3:3], rows[4:]...) {
		if r.Collation.Valid {
			s.Equal("A", r.Collation.String, "column %s should be ascending", r.ColumnName)
		}
	}
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
