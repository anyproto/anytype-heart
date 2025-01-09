package sqlitestorage

const sqlCreateTables = `
CREATE TABLE IF NOT EXISTS spaces  (
  id text not null primary key,
  header text not null,
  settingsId text not null,
  aclId text not null,
  hash text not null default '',
  oldHash text not null default '',
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
	if s.stmt.createSpace, err = s.writeDb.Prepare(`INSERT INTO spaces(id, header, settingsId, aclId) VALUES (?, ?, ?, ?)`); err != nil {
		return
	}
	if s.stmt.createTree, err = s.writeDb.Prepare(`INSERT INTO trees(id, spaceId, heads, type) VALUES(?, ?, ?, ?)`); err != nil {
		return
	}
	if s.stmt.createChange, err = s.writeDb.Prepare(`INSERT INTO changes(id, spaceId, treeId, data) VALUES(?, ?, ?, ?)`); err != nil {
		return
	}
	if s.stmt.updateSpaceHash, err = s.writeDb.Prepare(`UPDATE spaces SET hash = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.updateSpaceOldHash, err = s.writeDb.Prepare(`UPDATE spaces SET oldHash = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.updateSpaceIsCreated, err = s.writeDb.Prepare(`UPDATE spaces SET isCreated = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.updateSpaceIsDeleted, err = s.writeDb.Prepare(`UPDATE spaces SET isDeleted = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.treeIdsBySpace, err = s.readDb.Prepare(`SELECT id FROM trees WHERE spaceId = ? AND type != 1 AND deleteStatus IS NULL`); err != nil {
		return
	}
	if s.stmt.deleteTree, err = s.writeDb.Prepare(`
			INSERT INTO trees (id, spaceId, heads) VALUES(?, ?, NULL)
			ON CONFLICT (id) DO UPDATE SET heads = NULL
	`); err != nil {
		return
	}
	if s.stmt.updateTreeDelStatus, err = s.writeDb.Prepare(`
			INSERT INTO trees (id, deleteStatus, spaceId) VALUES(?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET deleteStatus = ?
	`); err != nil {
		return
	}
	if s.stmt.treeDelStatus, err = s.readDb.Prepare(`SELECT deleteStatus FROM trees WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.allTreeDelStatus, err = s.readDb.Prepare(`SELECT id FROM trees WHERE spaceId = ? AND deleteStatus = ?`); err != nil {
		return
	}
	if s.stmt.change, err = s.readDb.Prepare(`SELECT data FROM changes WHERE id = ? AND spaceId = ?`); err != nil {
		return
	}
	if s.stmt.hasTree, err = s.readDb.Prepare(`SELECT COUNT(*) FROM trees WHERE id = ? AND spaceId = ? AND heads IS NOT NULL`); err != nil {
		return
	}
	if s.stmt.hasChange, err = s.readDb.Prepare(`SELECT COUNT(*) FROM changes WHERE id = ? AND treeId = ?`); err != nil {
		return
	}
	if s.stmt.updateTreeHeads, err = s.writeDb.Prepare(`UPDATE trees SET heads = ? WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.deleteChangesByTree, err = s.writeDb.Prepare(`DELETE FROM changes WHERE treeId = ?`); err != nil {
		return
	}
	if s.stmt.loadTreeHeads, err = s.readDb.Prepare(`SELECT heads FROM trees WHERE id = ? AND heads IS NOT NULL`); err != nil {
		return
	}
	if s.stmt.loadSpace, err = s.readDb.Prepare(`SELECT header, settingsId, aclId, hash, oldHash, isDeleted FROM spaces WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.spaceIds, err = s.readDb.Prepare(`SELECT id FROM spaces`); err != nil {
		return
	}
	if s.stmt.spaceIsCreated, err = s.readDb.Prepare(`SELECT isCreated FROM spaces WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.getBind, err = s.readDb.Prepare(`SELECT spaceId FROM binds WHERE objectId = ?`); err != nil {
		return
	}
	if s.stmt.upsertBind, err = s.writeDb.Prepare(`INSERT INTO binds (objectId, spaceId) VALUES (?, ?) ON CONFLICT (objectId) DO UPDATE SET spaceId = ?`); err != nil {
		return
	}
	if s.stmt.deleteSpace, err = s.writeDb.Prepare(`DELETE FROM spaces WHERE id = ?`); err != nil {
		return
	}
	if s.stmt.deleteTreesBySpace, err = s.writeDb.Prepare(`DELETE FROM trees WHERE spaceId = ?`); err != nil {
		return
	}
	if s.stmt.deleteChangesBySpace, err = s.writeDb.Prepare(`DELETE FROM changes WHERE spaceId = ?`); err != nil {
		return
	}
	if s.stmt.deleteBindsBySpace, err = s.writeDb.Prepare(`DELETE FROM binds WHERE spaceId = ?`); err != nil {
		return
	}
	return
}
