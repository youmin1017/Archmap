-- name: InsertChangeset :one
INSERT INTO changesets (title, description, author_id, base_version, status, reverts)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetChangeset :one
SELECT * FROM changesets WHERE id = $1;

-- name: GetChangesetForUpdate :one
SELECT * FROM changesets WHERE id = $1 FOR UPDATE;

-- name: ListChangesets :many
-- Direct edits of main (ADR-072) are versions, not drafts: they are not listed.
SELECT * FROM changesets
WHERE kind = 'draft'
  AND (sqlc.narg('status')::changeset_status IS NULL OR status = sqlc.narg('status')::changeset_status)
  AND (sqlc.narg('author_id')::uuid IS NULL OR author_id = sqlc.narg('author_id')::uuid)
ORDER BY created_at DESC, id DESC;

-- name: MarkChangesetMerged :exec
UPDATE changesets SET status = 'merged', merged_at = now() WHERE id = $1;

-- name: CurrentModelVersion :one
SELECT COALESCE(MAX(number), 0)::int FROM model_versions;

-- name: InsertModelVersion :exec
INSERT INTO model_versions (number, changeset_id, merged_by, summary)
VALUES ($1, $2, $3, $4);

-- name: ListActiveChanges :many
SELECT * FROM changes WHERE changeset_id = $1 AND undone_at IS NULL ORDER BY seq;

-- name: ListChanges :many
SELECT * FROM changes WHERE changeset_id = $1 ORDER BY seq;

-- name: MaxChangeSeq :one
SELECT COALESCE(MAX(seq), 0)::int FROM changes WHERE changeset_id = $1;

-- name: DeleteUndoneChanges :exec
DELETE FROM changes WHERE changeset_id = $1 AND undone_at IS NOT NULL;

-- name: InsertChange :one
INSERT INTO changes (changeset_id, seq, batch_id, op, target_type, target_id, patch, before)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UndoLastBatch :many
UPDATE changes AS ch SET undone_at = now()
WHERE ch.changeset_id = $1
  AND ch.undone_at IS NULL
  AND ch.batch_id = (
      SELECT c.batch_id FROM changes c
      WHERE c.changeset_id = $1 AND c.undone_at IS NULL
      ORDER BY c.seq DESC LIMIT 1)
RETURNING ch.*;

-- name: RedoNextBatch :many
UPDATE changes AS ch SET undone_at = NULL
WHERE ch.changeset_id = $1
  AND ch.batch_id = (
      SELECT c.batch_id FROM changes c
      WHERE c.changeset_id = $1 AND c.undone_at IS NOT NULL
      ORDER BY c.seq ASC LIMIT 1)
RETURNING ch.*;

-- name: LockMain :exec
SELECT pg_advisory_xact_lock(7201);

-- name: GetSnapshot :one
SELECT data FROM model_snapshots WHERE version = $1;

-- name: InsertSnapshot :exec
INSERT INTO model_snapshots (version, data) VALUES ($1, $2);

-- name: ListModelVersions :many
SELECT v.number, v.changeset_id, v.merged_by, v.merged_at, v.summary, c.kind, c.reverts
FROM model_versions v JOIN changesets c ON c.id = v.changeset_id
ORDER BY v.number DESC;

-- name: VersionOfChangeset :one
SELECT number FROM model_versions WHERE changeset_id = $1;

-- name: IsReverted :one
SELECT EXISTS (SELECT 1 FROM changesets WHERE reverts = $1 AND status = 'merged');

-- name: MarkChangesetDirect :exec
UPDATE changesets SET kind = 'direct' WHERE id = $1;

-- name: ApproveChangeset :one
UPDATE changesets SET approved_by = $2, approved_at = now() WHERE id = $1 RETURNING *;

-- name: SetChangesetStatus :one
UPDATE changesets SET status = $2 WHERE id = $1 RETURNING *;

-- name: ListResolutions :many
SELECT target_type, target_id, field, choice FROM merge_resolutions WHERE changeset_id = $1 ORDER BY target_id, field;

-- name: UpsertResolution :exec
INSERT INTO merge_resolutions (changeset_id, target_type, target_id, field, choice)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (changeset_id, target_type, target_id, field) DO UPDATE SET choice = EXCLUDED.choice;
