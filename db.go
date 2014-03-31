// Copyright (c) 2013 VividCortex. Please see the LICENSE file for license terms.

/*
Package dbcontrol limits the number of active connections for database/sql.

Implementations of Go's database/sql package prior to Go 1.2, don't let the user
put a limit on the number of active connections to the underlying DB.  If enough
concurrent requests are made, so that the package runs out of available
connections, then more are requested from the driver no matter how many you have
at the pool already. Hence, unless you take precautions in your design/code, or
there's specific support from the actual DB driver you're using, you are likely
to hit some other limit (DB engine itself, OS) or simply run out of resources.

None of those situations are desirable, of course. If you hit DB or OS limits on
the number of connections, then many of your statements will start failing cause
no connection is available for them to use. You can get around it if you're
lucky enough to have driver support, but then you are tied to a particular DB.

This package is a wrapper on Go's standard database/sql, providing a general
mechanism so that you're free to use statements as usual, yet have the number of
active connections limited. A wrapper DB type is declared, that supports all
standard operations from database/sql. To use it, you should set the maximum
number of connections you want to allow, just like:

	dbcontrol.SetConcurrency(10)

All databases opened by the package afterwards will use a maximum of 10
connections. You can change this setting as often as you wish, but keep in mind
that the number is bound to databases as they are opened, i.e., changing this
concurrency setting has no effect on already-opened databases. Note also that
you can get the default non-limited behavior by setting concurrency to zero. To
open a DB you proceed just like with the database/sql package, like so:

	db, err := dbcontrol.Open("mysql", dsn)

Note that sql.Row, sql.Rows and sql.Stmt types are overridden by this package,
but that's probably transparent unless you declare the types explicitly. If you
declare variables using the := operator you'll be fine. Usage now follows the
expected pattern from database/sql:

	rows, err := db.Query("select id, name from customers")
	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatal(err)
		}

		fmt.Println(id, name)
	}

The full set of features at database/sql is supported, including transactions,
even though not all functions need to be overridden. This package was designed
to provide the feature with minimum overhead, and thus uses knowledge of
database/sql internals to know when locking is required/appropriate. As an
extension, you can set a channel to receive the locking duration each time a
connection has to be waited for. This can work as an aid to help you tune the
pool size or otherwise work around concurrency problems. You can also set a
channel where notifications will be sent every time a connection is held for
longer than a certain timeout. The notification includes the full stack trace of
the caller at the time the connection was requested. This can prove useful to
identify long-running queries that are locking connections, and possibly
impeding others from running. The feature can be turned on and off at will. A
small performance penalty will be paid if on (that of retrieving the caller's
stack), but none if the feature is off (the default).

Note that only functions specific to this package or with altered semantics are
documented. Please refer to the database/sql package documentation for more
information.
*/
package dbcontrol

import (
	"database/sql"
	"sync"
	"time"
)

// DB is the main type wrapping up sql.DB. You should use it just like you would
// sql.DB. If a connection is required and not available, the statement using
// the type will block until another connection is returned to the pool.
type DB struct {
	*sql.DB
	maxConns        int
	sem             chan bool
	blockCh         chan<- time.Duration
	blockChMux      sync.RWMutex
	usageTimeout    time.Duration
	usageTimeoutCh  chan<- string
	usageTimeoutMux sync.RWMutex
}

func Open(driver, dsn string) (*DB, error) {
	sqldb, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	// We wrap *sql.DB into our DB
	db := &DB{DB: sqldb}

	if c := Concurrency(); c > 0 {
		// Let's create a token channel and feed it with c tokens
		db.sem = make(chan bool, c)

		for i := 0; i < c; i++ {
			db.sem <- true
		}

		// This is actually required, otherwise connections are quickly
		// discarded, even if new ones have to be immediately opened.
		db.DB.SetMaxIdleConns(c)
		db.maxConns = c
	}

	return db, nil
}

// MaxConn returns the maximum number of connections for the DB.
func (db *DB) MaxConns() int {
	return db.maxConns
}
