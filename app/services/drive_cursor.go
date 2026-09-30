package services

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"ponta_drive/app/models"
)

// DriveCursor marks the position of the last item of a page so the next page
// can resume from it.
//
// The list query orders by three tiers - type, then the caller's sort column,
// then id - so the cursor has to carry all three. A cursor holding only the
// sort value would resume at the wrong place whenever two rows share it.
//
// It also records which ordering produced it. A position only means something
// inside one ordering: replaying a name-positioned cursor against `sort=size`
// would compare a name against the size column and return rows the caller
// never asked for.
type DriveCursor struct {
	Type  string `json:"t"`
	Value string `json:"v,omitempty"`
	ID    uint   `json:"id"`
	Sort  string `json:"s"`
	Order string `json:"o"`
}

// Matches reports whether this cursor was produced under the given ordering.
func (c DriveCursor) Matches(orderCol, orderDir string) bool {
	return c.Sort == orderCol && c.Order == orderDir
}

// sortColumns maps a caller-supplied sort field to the real column. Only these
// four may ever reach the SQL string: the value is interpolated into the ORDER
// BY clause, so an unwhitelisted value would be an injection.
var sortColumns = map[string]string{
	"name":       "name",
	"size":       "size",
	"updated_at": "updated_at",
	"created_at": "created_at",
}

// NormalizeSortField returns a column name safe to interpolate into SQL.
// Anything unrecognised falls back to "name".
func NormalizeSortField(field string) string {
	if col, ok := sortColumns[field]; ok {
		return col
	}
	return "name"
}

// NormalizeSortOrder returns "asc" or "desc". Anything else means "asc".
func NormalizeSortOrder(order string) string {
	if order == "desc" {
		return "desc"
	}
	return "asc"
}

// cursorValue extracts the sort value from an item as a string. Values are
// carried as text because that is what the keyset comparison binds; SQL
// compares them against the real column, which MariaDB casts.
//
// `size` comes back from JSON as a float64, so it is formatted without an
// exponent; a timestamp is formatted with the same layout the column uses, so
// the string compares correctly against it. A NULL timestamp returns "", which
// buildCursorWhere treats as the start of that column's range.
func cursorValue(item models.DriveItem, orderCol string) string {
	switch orderCol {
	case "size":
		return strconv.FormatInt(item.Size, 10)
	case "updated_at":
		if item.UpdatedAt != nil {
			return item.UpdatedAt.Format("2006-01-02 15:04:05")
		}
		return ""
	case "created_at":
		if item.CreatedAt != nil {
			return item.CreatedAt.Format("2006-01-02 15:04:05")
		}
		return ""
	default:
		return item.Name
	}
}

// EncodeDriveCursor serialises the position of an item into an opaque token.
func EncodeDriveCursor(item models.DriveItem, orderCol, orderDir string) (string, error) {
	c := DriveCursor{
		Type:  item.Type,
		Value: cursorValue(item, orderCol),
		ID:    item.ID,
		Sort:  orderCol,
		Order: orderDir,
	}

	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("failed to marshal drive cursor: %w", err)
	}

	return base64.URLEncoding.EncodeToString(raw), nil
}

// DecodeDriveCursor parses a token produced by EncodeDriveCursor. A malformed
// token is an error, never a silent restart from page one: silently restarting
// would make an infinite scroller loop over the first page forever.
//
// The empty string is not malformed - it is how a caller asks for the first
// page - so it decodes to the zero cursor, which buildCursorWhere ignores.
func DecodeDriveCursor(raw string) (DriveCursor, error) {
	if raw == "" {
		return DriveCursor{}, nil
	}

	var c DriveCursor

	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return c, fmt.Errorf("drive cursor is not valid base64: %w", err)
	}

	if err := json.Unmarshal(decoded, &c); err != nil {
		return c, fmt.Errorf("drive cursor is not valid JSON: %w", err)
	}

	if c.ID == 0 {
		return c, errors.New("drive cursor is missing its id")
	}

	return c, nil
}

// BuildCursorWhere returns the keyset predicate selecting the rows that come
// after the cursor, plus its bind arguments.
//
// Ordering is: type DESC, orderCol <dir>, id ASC.
//
//   - type DESC is always a strict "<" comparison: 'folder' sorts before
//     'file', so anything after a 'folder' is a 'file', and nothing follows a
//     'file'.
//   - orderCol reverses with the requested direction.
//   - id is always ASC, and is what makes the ordering total. Without it, rows
//     sharing a sort value - which is the norm for updated_at, whose
//     granularity is one second - would shuffle between pages.
//
// A NULL timestamp sorts before every value in MySQL's ascending order, so the
// cursor value for one is "". That empty string cannot be compared with a
// bound parameter - `col > ”` is false for every row, so an ascending cursor
// sitting on a NULL would skip every remaining row. Both cases are therefore
// spelled out as SQL rather than bound: the same-column comparison is dropped
// when the value is empty, leaving the id tier to carry the walk.
func BuildCursorWhere(c DriveCursor, orderCol, orderDir string) (string, []any) {
	cmp := ">"
	if orderDir == "desc" {
		cmp = "<"
	}

	cond := "(type < ?) OR (type = ?"
	args := []any{c.Type, c.Type}

	if c.Value != "" {
		cond += fmt.Sprintf(" AND (%s %s ? OR (%s = ? AND id > ?))", orderCol, cmp, orderCol)
		args = append(args, c.Value, c.Value, c.ID)
	} else {
		cond += " AND id > ?"
		args = append(args, c.ID)
	}

	cond += ")"

	return cond, args
}
