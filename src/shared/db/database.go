package db

import (
	"context"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

type TxFunc func(tx pgx.Tx) error

func NewDB(connString string) (*DB, error) {
	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, err
	}

	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) Transaction(ctx context.Context, fn TxFunc) error {
	// Begin a transaction
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx) // recover from panic and rollback if necessary
			panic(p)             // re-throw panic after Rollback
		} else if err != nil {
			_ = tx.Rollback(ctx) // rollback if there was an error
		} else {
			err = tx.Commit(ctx) // commit if there was no error
		}
	}()

	err = fn(tx) // call the transaction function supplied by the caller
	return err
}
