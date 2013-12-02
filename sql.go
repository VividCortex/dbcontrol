// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

package dbcontrol

import (
	"database/sql"
	"time"
)

// SetBlockDurationCh sets a channel used to report blocks on connections. Each
// time a connection has to be waited for due to the limit imposed by
// SetConcurrency(), this channel will receive the duration for that wait as
// soon as the connection becomes available. Setting the channel to nil (i.e.,
// calling SetBlockDurationCh(nil)) will close the previously assigned channel,
// if any. Note also that this operation is safe to be called in parallel with
// other database requests.
func (db *DB) SetBlockDurationCh(c chan<- time.Duration) {
	db.blockChMux.Lock()
	defer db.blockChMux.Unlock()

	if db.blockCh != nil {
		close(db.blockCh)
	}

	db.blockCh = c
}

func (db *DB) conn() func() {
	if db.sem == nil {
		// Not using tokens
		return func() {}
	}

	select {
	case <-db.sem:
	default:
		start := time.Now()
		<-db.sem

		db.blockChMux.RLock()
		if db.blockCh != nil {
			db.blockCh <- time.Now().Sub(start)
		}
		db.blockChMux.RUnlock()
	}

	return func() {
		db.sem <- true
	}
}

// SetMaxIdleConns sets the maximum number of idle connections to the database.
// However, note that this only makes sense if you're not limiting the number
// of concurrent connections. Databases opened under SetConcurrency(n) for n>0
// will silently ignore this call. (The maximum number of connections in that
// case will match the concurrency value n.)
func (db *DB) SetMaxIdleConns(n int) {
	if db.sem == nil {
		// Not using tokens
		db.DB.SetMaxIdleConns(n)
	}
}

func (db *DB) Ping() error {
	release := db.conn()
	defer release()
	return db.DB.Ping()
}

func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	release := db.conn()
	defer release()
	return db.DB.Exec(query, args...)
}

type Rows struct {
	*sql.Rows
	closed  bool
	release func()
}

func (db *DB) Query(query string, args ...interface{}) (*Rows, error) {
	release := db.conn()
	rows, err := db.DB.Query(query, args...)

	if err != nil {
		release()
		return nil, err
	}

	return &Rows{Rows: rows, release: release}, nil
}

func (rows *Rows) Next() bool {
	if rows.closed {
		return false
	}

	next := rows.Rows.Next()
	if !next && rows.Rows.Err() == nil {
		// EOF: the result set was closed by Rows.Next()
		rows.release()
		rows.closed = true
	}

	return next
}

func (rows *Rows) Close() error {
	err := rows.Rows.Close()

	if !rows.closed {
		rows.release()
		rows.closed = true
	}

	return err
}

type Row struct {
	*sql.Row
	closed  bool
	release func()
}

func (db *DB) QueryRow(query string, args ...interface{}) *Row {
	release := db.conn()
	row := db.DB.QueryRow(query, args...)
	return &Row{Row: row, release: release}
}

func (row *Row) Scan(dest ...interface{}) error {
	err := row.Row.Scan(dest...)

	if !row.closed {
		row.release()
		row.closed = true
	}

	return err
}

type Stmt struct {
	*sql.Stmt
	db *DB
}

func (db *DB) Prepare(query string) (*Stmt, error) {
	release := db.conn()
	defer release()

	stmt, err := db.DB.Prepare(query)
	if err != nil {
		return nil, err
	}

	return &Stmt{Stmt: stmt, db: db}, nil
}

func (s *Stmt) Exec(args ...interface{}) (sql.Result, error) {
	release := s.db.conn()
	defer release()
	return s.Stmt.Exec(args...)
}

func (s *Stmt) Query(args ...interface{}) (*Rows, error) {
	release := s.db.conn()
	rows, err := s.Stmt.Query(args...)

	if err != nil {
		release()
		return nil, err
	}

	return &Rows{Rows: rows, release: release}, nil
}

func (s *Stmt) QueryRow(args ...interface{}) *Row {
	release := s.db.conn()
	row := s.Stmt.QueryRow(args...)
	return &Row{Row: row, release: release}
}

type Tx struct {
	*sql.Tx
	release func()
}

func (db *DB) Begin() (*Tx, error) {
	release := db.conn()
	tx, err := db.DB.Begin()

	if err != nil {
		release()
		return nil, err
	}

	return &Tx{Tx: tx, release: release}, nil
}

func (tx *Tx) Commit() error {
	defer tx.release()
	return tx.Tx.Commit()
}

func (tx *Tx) Rollback() error {
	defer tx.release()
	return tx.Tx.Rollback()
}
