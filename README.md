dbcontrol
=========

This package is a wrapper on Go's standard database/sql, providing a general
mechanism to limit the number of active connections to the database, no matter
the driver you use. Provided that you don't explicitly declare sql.Row, sql.Rows
or sql.Stmt variables, you can use dbcontrol just by adding a couple of calls
(setting the actual limit and wrapping up the DB) and using dbcontrol.DB instead
of sql.DB. All operations from database/sql are then transparently supported.

We use this package at [VividCortes](https://vividcortex.com/) to cope with
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
you can get the default non-limited behavior by setting concurrency to zero.

To create a DB you open it as usual using the database/sql package and then
proceed to wrap it up with this package. You no longer have to keep track of the
original sql.DB object; you should always use the dbcontrol.DB object returned
by the NewDB() call, like so:

	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}

	db := dbcontrol.NewDB(sqldb)

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
pool size or otherwise work around concurrency problems.

Contributing
============

Pull requests (with tests, ideally) are welcome!

License
=======

Copyright (c) 2013 VividCortex, licensed under the MIT license.
Please see the LICENSE file for details.
