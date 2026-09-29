// Package postgres holds the pgx helpers Postgres lessons repeat. It lives
// beside lab rather than in it so lab stays standard-library only and a
// lesson that never talks to Postgres does not compile pgx.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/nickstrad/systems-trinkets/internal/lab"
)

// DefaultURL is the local dev database from software/software.md. The
// credentials are not secret; every lesson prints and uses them as-is.
const DefaultURL = "postgres://trinkets:trinkets@localhost:5432/trinkets"

// URL is DATABASE_URL, or DefaultURL.
func URL() string { return lab.Env("DATABASE_URL", DefaultURL) }

// Connect opens one connection to URL() and fails fast on error. Set
// DATABASE_URL to point a lesson at another database. Call it once per
// connection a lesson needs: two transactions can only contend for a row
// lock from two separate connections.
func Connect(ctx context.Context) *pgx.Conn {
	conn, err := pgx.Connect(ctx, URL())
	lab.Check(err)
	return conn
}
