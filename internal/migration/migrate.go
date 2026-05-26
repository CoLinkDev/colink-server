package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	appmigrations "colink-server/migrations"
)

func Up(db *sql.DB, databaseName string) error {
	sourceDriver, err := iofs.New(appmigrations.Files, ".")
	if err != nil {
		return fmt.Errorf("open migration source: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}

	databaseDriver, err := postgres.WithConnection(context.Background(), conn, &postgres.Config{
		DatabaseName:          databaseName,
		MigrationsTable:       "schema_migrations",
		MultiStatementEnabled: true,
	})
	if err != nil {
		return fmt.Errorf("open migration database driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, databaseName, databaseDriver)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
