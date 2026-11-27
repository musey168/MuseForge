-- Optional manual additive DDL. Startup AutoMigrate also adds these columns.
-- run_workspace_checkpoints.lease_renewed_at stamps each lease take and renewal
-- with the database clock, so a replica judges how long ago another replica
-- renewed without comparing two hosts' clocks. A lease taken before the column
-- existed has no stamp and is judged by its lease_until as before.
SET @ws_lease_renewed_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema=DATABASE() AND table_name='run_workspace_checkpoints' AND column_name='lease_renewed_at')=0,
 'ALTER TABLE run_workspace_checkpoints ADD COLUMN lease_renewed_at DATETIME(3) NULL',
 'SELECT 1');
PREPARE ws_lease_renewed_stmt FROM @ws_lease_renewed_ddl;
EXECUTE ws_lease_renewed_stmt;
DEALLOCATE PREPARE ws_lease_renewed_stmt;
