package importer

import (
	"database/sql"
	"fmt"
)

// batcher accumulates rows and flushes them in batched transactions.
type batcher[T any] struct {
	db      *sql.DB
	size    int
	buf     []T
	flushFn func(tx *sql.Tx, rows []T) error
	total   int
}

func newBatcher[T any](db *sql.DB, size int, fn func(*sql.Tx, []T) error) *batcher[T] {
	return &batcher[T]{db: db, size: size, buf: make([]T, 0, size), flushFn: fn}
}

func (b *batcher[T]) add(row T) error {
	b.buf = append(b.buf, row)
	if len(b.buf) >= b.size {
		return b.flush()
	}
	return nil
}

func (b *batcher[T]) flush() error {
	if len(b.buf) == 0 {
		return nil
	}
	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := b.flushFn(tx, b.buf); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	b.total += len(b.buf)
	b.buf = b.buf[:0]
	return nil
}
