// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

package dbcontrol

import (
	"database/sql"
	"runtime/debug"
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

// SetUsageTimeout sets a maximum time for connection usage since it was granted
// to the caller (i.e., usage starts when a spare connection could be withdrawn
// from the pool, in case connection limiting is in use; see SetConcurrency()).
// After the time has elapsed a notification will be sent to the provided
// channel including the stack trace for the offending consumer (at the time the
// connection was requested). Setting the timeout to zero (the default) disables
// this feature. Note that this function is safe to be called anytime.  Changes
// in the timeout will take effect for new requests; pending timers will still
// use the previous value. Changing the channel takes effect immediately,
// though. The previously set channel is guaranteed not to be used again after
// SetUsageTimeout() returns, thus allowing to safely close it if appropriate.
// Setting the channel to nil disables all pending and future notifications,
// until set to another valid channel. Note though that, in order to avoid
// needless resource usage, setting the channel to nil implies that no further
// timers will be started. (That is, you won't get a notification for a long
// running consumer that requested a connection when the channel was nil, even
// if you set the channel to a non-nil value before the timeout would expire.
// Consider fixing the channel and filtering events when reading from it if
// you're looking for that effect.) Note also that the timer expiring, whether
// notified or not, has no effect whatsoever on the routine using the
// connection.
func (db *DB) SetUsageTimeout(c chan<- string, timeout time.Duration) {
	db.usageTimeoutMux.Lock()
	defer db.usageTimeoutMux.Unlock()
	db.usageTimeoutCh = c

	if c != nil {
		db.usageTimeout = timeout
	} else {
		db.usageTimeout = 0
	}
}

func (db *DB) conn() func() {
	releaseLock := func() {}

	if db.sem != nil {
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

		releaseLock = func() {
			db.sem <- true
		}
	}

	db.usageTimeoutMux.RLock()
	usageTimeout := db.usageTimeout
	db.usageTimeoutMux.RUnlock()
	cancelUsageTimeout := func() {}

	if usageTimeout != 0 {
		cancelTimeoutCh := make(chan struct{}, 1)
		cancelUsageTimeout = func() {
			cancelTimeoutCh <- struct{}{}
			close(cancelTimeoutCh)
		}
		stack := debug.Stack()

		go func() {
			select {
			case <-time.After(usageTimeout):
				db.usageTimeoutMux.RLock()
				if db.usageTimeoutCh != nil {
					db.usageTimeoutCh <- string(stack)
				}
				db.usageTimeoutMux.RUnlock()
			case <-cancelTimeoutCh:
			}
		}()
	}

	return func() {
		releaseLock()
		cancelUsageTimeout()
	}
}

var dummyRelease func() = func() {}

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
	stmt *sql.Stmt
	db   *DB
}

func (db *DB) Prepare(query string) (*Stmt, error) {
	release := db.conn()
	defer release()

	stmt, err := db.DB.Prepare(query)
	if err != nil {
		return nil, err
	}

	return &Stmt{stmt: stmt, db: db}, nil
}

func (s *Stmt) Close() error {
	return s.stmt.Close()
}

func (s *Stmt) Exec(args ...interface{}) (sql.Result, error) {
	if s.db != nil {
		release := s.db.conn()
		defer release()
	}
	return s.stmt.Exec(args...)
}

func (s *Stmt) Query(args ...interface{}) (*Rows, error) {
	var release func()
	if s.db == nil {
		release = dummyRelease
	} else {
		release = s.db.conn()
	}

	rows, err := s.stmt.Query(args...)
	if err != nil {
		release()
		return nil, err
	}

	return &Rows{Rows: rows, release: release}, nil
}

func (s *Stmt) QueryRow(args ...interface{}) *Row {
	var release func()
	if s.db == nil {
		release = dummyRelease
	} else {
		release = s.db.conn()
	}
	row := s.stmt.QueryRow(args...)
	return &Row{Row: row, release: release}
}

type Tx struct {
	trn     *sql.Tx
	closed  bool
	release func()
}

func (db *DB) Begin() (*Tx, error) {
	release := db.conn()
	tx, err := db.DB.Begin()

	if err != nil {
		release()
		return nil, err
	}

	return &Tx{trn: tx, release: release}, nil
}

func (tx *Tx) Commit() error {
	if !tx.closed {
		defer func() {
			tx.release()
			tx.closed = true
		}()
	}
	return tx.trn.Commit()
}

func (tx *Tx) Exec(query string, args ...interface{}) (sql.Result, error) {
	return tx.trn.Exec(query, args...)
}

func (tx *Tx) Prepare(query string) (*Stmt, error) {
	stmt, err := tx.trn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &Stmt{stmt: stmt}, nil
}

func (tx *Tx) Query(query string, args ...interface{}) (*Rows, error) {
	rows, err := tx.trn.Query(query, args...)
	return &Rows{Rows: rows, release: dummyRelease}, err
}

func (tx *Tx) QueryRow(query string, args ...interface{}) *Row {
	row := tx.trn.QueryRow(query, args...)
	return &Row{Row: row, release: dummyRelease}
}

func (tx *Tx) Rollback() error {
	if !tx.closed {
		defer func() {
			tx.release()
			tx.closed = true
		}()
	}
	return tx.trn.Rollback()
}

func (tx *Tx) Stmt(stmt *Stmt) *Stmt {
	newStmt := tx.trn.Stmt(stmt.stmt)
	return &Stmt{stmt: newStmt}
}
