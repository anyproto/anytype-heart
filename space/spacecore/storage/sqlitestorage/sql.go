package sqlitestorage

const sqlCreateTables = `
CREATE TABLE IF NOT EXISTS spaces  (
  id text not null primary key,
  header text not null,
  settingsId text not null,
  aclId text not null,
  hash text,
  oldHash text,
  isCreated boolean not null default false,
  isDeleted boolean not null default false                        
);

CREATE TABLE IF NOT EXISTS binds (
    objectId text not null primary key,
    spaceId text not null
);

CREATE TABLE IF NOT EXISTS trees (
    id text not null primary key,
    spaceId text not null,
    type int not null default 0,
    heads text,
    deleteStatus text
);
CREATE INDEX IF NOT EXISTS 'trees_spaceId' ON trees(spaceId);

CREATE TABLE IF NOT EXISTS changes (
    id text not null primary key,
    spaceId text not null,
    treeId text not null,
    data text not null
);
CREATE INDEX IF NOT EXISTS 'changes_spaceId' ON changes(spaceId);
CREATE INDEX IF NOT EXISTS 'changes_treeId' ON changes(treeId);
`

func initStmts(s *storageService) (err error) {
	if s.stmt.createSpace, err = s.db.Prepare(`INSERT INTO spaces(id, header, settingsId, aclId) VALUES (?, ?, ?, ?)`); err != nil {
		return
	}
	if s.stmt.createTree, err = s.db.Prepare(`INSERT INTO trees(id, spaceId, heads, type) VALUES(?, ?, ?, ?)`); err != nil {
		return
	}
	if s.stmt.createChange, err = s.db.Prepare(`INSERT INTO changes(id, spaceId, treeId, data) VALUES(?, ?, ?, ?)`); err != nil {
		return
	}
	if s.stmt.updateSpaceHash, err = s.db.Prepare(`UPDATE spaces SET hash = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.updateSpaceOldHash, err = s.db.Prepare(`UPDATE spaces SET oldHash = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.updateSpaceIsCreated, err = s.db.Prepare(`UPDATE spaces SET isCreated = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.updateSpaceIsDeleted, err = s.db.Prepare(`UPDATE spaces SET isDeleted = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.treeIdsBySpace, err = s.db.Prepare(`SELECT id FROM trees WHERE spaceId = ? AND type != 1`); err != nil {
		return
	}
	if s.stmt.updateTreeDelStatus, err = s.db.Prepare(`UPDATE trees SET deleteStatus = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.treeDelStatus, err = s.db.Prepare(`SELECT deleteStatus FROM trees WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.change, err = s.db.Prepare(`SELECT data FROM changes WHERE id = ? AND spaceId = ?`); err != nil {
		return
	}
	if s.stmt.hasTree, err = s.db.Prepare(`SELECT COUNT(*) FROM trees WHERE id = ? AND spaceId = ?`); err != nil {
		return
	}
	if s.stmt.hasChange, err = s.db.Prepare(`SELECT COUNT(*) FROM changes WHERE id = ? AND treeId = ?`); err != nil {
		return
	}
	if s.stmt.updateTreeHeads, err = s.db.Prepare(`UPDATE trees SET heads = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.deleteTree, err = s.db.Prepare(`DELETE FROM trees WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.deleteChangesByTree, err = s.db.Prepare(`DELETE FROM changes WHERE treeId = ?`); err != nil {
		return
	}
	if s.stmt.loadTreeHeads, err = s.db.Prepare(`SELECT heads FROM trees WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.loadSpace, err = s.db.Prepare(`SELECT header, settingsId, aclId, hash, oldHash FROM spaces WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.spaceIds, err = s.db.Prepare(`SELECT id FROM spaces`); err != nil {
		return
	}
	if s.stmt.spaceIsCreated, err = s.db.Prepare(`SELECT isCreated FROM spaces WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.getBind, err = s.db.Prepare(`SELECT spaceId FROM binds WHERE objectId = ?`); err != nil {
		return
	}
	if s.stmt.upsertBind, err = s.db.Prepare(`INSERT INTO binds (objectId, spaceId) VALUES (?, ?) ON CONFLICT (objectId) DO UPDATE SET spaceId = ?`); err != nil {
		return
	}
	return
}
