package sharedauth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestTransactionPublicationAndCommitFailure(t *testing.T) {
	sentinel := errors.New("synthetic commit acknowledgement lost")
	for _, outcome := range []string{"success", "operation-failure", "unknown-commit", "inactive", "read-only"} {
		t.Run(outcome, func(t *testing.T) {
			connector := &transactionConnector{phase: "active"}
			if outcome == "unknown-commit" {
				connector.commitErr = sentinel
			}
			if outcome == "inactive" {
				connector.phase = "staged"
			}
			connector.readOnly = outcome == "read-only"
			db := sql.OpenDB(connector)
			defer db.Close()
			s, err := New(db, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			result, err := transact(t.Context(), s, func(context.Context, *sql.Tx, uint64) (string, error) {
				calls++
				if outcome == "operation-failure" {
					return "must-not-publish", ErrIdentityConflict
				}
				return "committed-result", nil
			})
			switch outcome {
			case "success":
				if err != nil || result != "committed-result" || connector.commits != 1 {
					t.Fatal("committed result not published", err)
				}
			case "operation-failure":
				if !errors.Is(err, ErrIdentityConflict) || connector.rollbacks != 1 || connector.commits != 0 {
					t.Fatal("operation failure did not roll back", err)
				}
			case "unknown-commit":
				if !errors.Is(err, ErrCommitUnknown) || !errors.Is(err, sentinel) || connector.commits != 1 || calls != 1 {
					t.Fatal("unknown commit was hidden or retried", err)
				}
			case "inactive", "read-only":
				if !errors.Is(err, ErrNotReady) || calls != 0 || connector.rollbacks != 1 {
					t.Fatal("inactive authority ran business operations", err)
				}
			}
			if outcome != "success" && result != "" {
				t.Fatal("uncommitted result escaped the transaction")
			}
		})
	}
}

// The scripted driver exercises commit acknowledgement handling, not SQL isolation.
type transactionConnector struct {
	phase              string
	readOnly           bool
	commitErr          error
	commits, rollbacks int
}

func (c *transactionConnector) Connect(context.Context) (driver.Conn, error) {
	return &transactionConn{c}, nil
}
func (*transactionConnector) Driver() driver.Driver { return transactionDriver{} }

type transactionDriver struct{}

func (transactionDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use the isolated connector")
}

type transactionConn struct{ c *transactionConnector }

func (*transactionConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*transactionConn) Close() error              { return nil }
func (*transactionConn) Begin() (driver.Tx, error) { return nil, errors.New("expected BeginTx") }
func (c *transactionConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	if options.Isolation != driver.IsolationLevel(sql.LevelReadCommitted) {
		return nil, errors.New("wrong isolation")
	}
	return &transactionResult{c.c}, nil
}
func (c *transactionConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if query == "SELECT @@global.read_only OR @@global.super_read_only" {
		return &transactionRows{values: []driver.Value{c.c.readOnly}}, nil
	}
	if strings.Contains(query, "FROM ygg_go_state WHERE id=1 FOR SHARE") {
		return &transactionRows{values: []driver.Value{int64(1), c.c.phase, int64(10)}}, nil
	}
	return nil, errors.New("unexpected query")
}

type transactionResult struct{ c *transactionConnector }

func (tx *transactionResult) Commit() error   { tx.c.commits++; return tx.c.commitErr }
func (tx *transactionResult) Rollback() error { tx.c.rollbacks++; return nil }

type transactionRows struct {
	values []driver.Value
	done   bool
}

func (r *transactionRows) Columns() []string { return make([]string, len(r.values)) }
func (*transactionRows) Close() error        { return nil }
func (r *transactionRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(values, r.values)
	return nil
}
