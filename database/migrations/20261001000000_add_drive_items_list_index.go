package migrations

import (
	"ponta_drive/app/facades"
)

// M20261001000000AddDriveItemsListIndex adds the composite index the drive list
// query needs.
//
// The list query filters on user_id + cloud_account_id + parent_id and orders
// by `type DESC, <sort column> <dir>, id ASC`. Before this index the table only
// had single-column indexes, so MariaDB had to filesort every listing.
//
// This is raw SQL rather than table.Index(...) because the index must store
// `type` in DESCENDING order, and Goravel's blueprint cannot express that:
// Grammar.CompileIndex passes every column through wrap.Columnize, which wraps
// each name in backticks, so "type DESC" would be emitted as `type DESC` - one
// identifier, and the DDL fails.
//
// The descending column is what makes the index usable at all. An all-ascending
// index cannot satisfy `ORDER BY type DESC, name ASC` - mixing directions over
// one index is what forces the filesort. Verified with EXPLAIN on MariaDB 13.1
// against a 20k-row folder (a smaller table lets the optimiser skip the index,
// which makes the comparison meaningless):
//
//	(user_id, cloud_account_id, parent_id, type, name, id)      -> Using filesort
//	(user_id, cloud_account_id, parent_id, type DESC, name, id) -> Using index condition
//
// Only the default ordering is covered. The index stores `name` ascending, so
// `sort=name&order=desc` filesorts too - not just `sort=size` and
// `sort=updated_at`. Measured, per query shape:
//
//	sort=name  asc   -> Using index condition
//	+ search         -> Using index condition
//	type=folder      -> Using index condition
//	sort=size  desc  -> Using index condition; Using filesort
//	sort=name  desc  -> Using index condition; Using filesort
//
// Covering every ordering would mean an index per sort column per direction,
// multiplying write cost on a table written on every sync. The default is what
// the UI sends, so that is the one indexed.
type M20261001000000AddDriveItemsListIndex struct{}

// Signature The unique signature for the migration.
func (r *M20261001000000AddDriveItemsListIndex) Signature() string {
	return "20261001000000_add_drive_items_list_index"
}

// Up Run the migrations.
func (r *M20261001000000AddDriveItemsListIndex) Up() error {
	if !facades.Schema().HasTable("drive_items") {
		return nil
	}

	if facades.Schema().HasIndex("drive_items", "idx_drive_items_list") {
		return nil
	}

	return facades.Schema().Sql(
		"CREATE INDEX `idx_drive_items_list` ON `drive_items` " +
			"(`user_id`, `cloud_account_id`, `parent_id`, `type` DESC, `name`, `id`)",
	)
}

// Down Reverse the migrations.
func (r *M20261001000000AddDriveItemsListIndex) Down() error {
	if !facades.Schema().HasTable("drive_items") {
		return nil
	}

	if !facades.Schema().HasIndex("drive_items", "idx_drive_items_list") {
		return nil
	}

	return facades.Schema().Sql("DROP INDEX `idx_drive_items_list` ON `drive_items`")
}
