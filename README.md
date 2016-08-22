dbcontrol
=========

This package is a wrapper on Go's standard database/sql, providing a general
mechanism to limit the number of active connections to the database, no matter
the driver you use. Provided that you don't explicitly declare sql.Row, sql.Rows
or sql.Stmt variables, you can use dbcontrol just by adding a couple of calls
(setting the actual limit and wrapping up the DB) and using dbcontrol.DB instead
of sql.DB. All operations from database/sql are then transparently supported.

We use this package at [VividCortex](https://vividcortex.com/) to cope with
concurrent access to our HTTP servers, while a native solution to limit the
number of connections is not included in Go's standard library itself.

Documentation
=============

Please read the generated [package documentation](http://godoc.org/github.com/VividCortex/dbcontrol).

Getting Started
===============

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

Contributing
============

We only accept pull requests for minor fixes or improvements. This includes:

* Small bug fixes
* Typos
* Documentation or comments

Please open issues to discuss new features. Pull requests for new features will be rejected,
so we recommend forking the repository and making changes in your fork for your use case.

License
=======

Copyright (c) 2013 VividCortex, licensed under the MIT license.
Please see the LICENSE file for details.
