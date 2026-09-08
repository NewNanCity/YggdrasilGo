package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"yggdrasil-api-go/src/config"
	"yggdrasil-api-go/src/sharedauth/migrationplan"
	"yggdrasil-api-go/src/sharedauth/migrations"
)

type options struct {
	config          string
	plan            string
	confirmDatabase string
	confirmPlan     string
	maxRows         int
	timeout         time.Duration
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "shared-auth-migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("command required: dry-run, schema-upgrade, verify-hooks, apply, verify, activate, or deactivate")
	}
	command := args[0]
	validCommands := map[string]struct{}{
		"dry-run": {}, "schema-upgrade": {}, "verify-hooks": {}, "apply": {},
		"verify": {}, "activate": {}, "deactivate": {},
	}
	if _, valid := validCommands[command]; !valid {
		return fmt.Errorf("unknown command %q", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var opts options
	flags.StringVar(&opts.config, "config", "conf/config.yml", "service config containing the BlessingSkin DSN")
	flags.StringVar(&opts.plan, "plan", "", "private migration plan path")
	flags.StringVar(&opts.confirmDatabase, "confirm-database", "", "exact database name required for write commands")
	flags.StringVar(&opts.confirmPlan, "confirm-plan-sha256", "", "exact plan SHA-256 required for plan write commands")
	flags.IntVar(&opts.maxRows, "max-rows", 100000, "maximum rows allowed in each source table")
	flags.DurationVar(&opts.timeout, "timeout", 2*time.Minute, "database operation timeout")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || opts.maxRows <= 0 || opts.timeout <= 0 {
		return errors.New("unexpected arguments or non-positive limits")
	}
	if command == "dry-run" && opts.plan == "" {
		return errors.New("dry-run requires -plan in a private, ignored directory")
	}
	if command != "dry-run" && command != "schema-upgrade" && command != "verify-hooks" && opts.plan == "" {
		return errors.New("plan path is required")
	}
	if opts.plan != "" {
		privatePath, err := privatePlanPath(opts.plan)
		if err != nil {
			return err
		}
		opts.plan = privatePath
	}

	dsn, err := loadDSN(opts.config, os.Stdin)
	if err != nil {
		return err
	}
	db, err := openDatabase(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to configured database: %w", err)
	}
	var database string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil || database == "" {
		return errors.New("configured DSN must select a database")
	}

	switch command {
	case "dry-run":
		snapshot, err := migrationplan.ReadSnapshot(ctx, db, opts.maxRows)
		if err != nil {
			return err
		}
		plan, err := migrationplan.Build(snapshot, uuid.New(), time.Now())
		if err != nil {
			return err
		}
		if err := migrationplan.Save(opts.plan, plan); err != nil {
			return err
		}
		digest, _ := plan.Digest()
		printPlan("dry-run", database, plan, digest, "staged plan not applied")
		return nil
	case "schema-upgrade":
		if err := requireDatabaseConfirmation(database, opts.confirmDatabase); err != nil {
			return err
		}
		if err := migrations.Upgrade(ctx, db); err != nil {
			return err
		}
		fmt.Printf("schema-upgrade database=%q hooks=verified state=not-created\n", database)
		return nil
	case "verify-hooks":
		if err := migrations.VerifyHooks(ctx, db); err != nil {
			return err
		}
		fmt.Printf("verify-hooks database=%q status=ok\n", database)
		return nil
	}

	plan, err := migrationplan.Load(opts.plan)
	if err != nil {
		return err
	}
	digest, err := plan.Digest()
	if err != nil {
		return err
	}
	switch command {
	case "apply", "activate", "deactivate":
		if err := requireDatabaseConfirmation(database, opts.confirmDatabase); err != nil {
			return err
		}
		if opts.confirmPlan != digest {
			return errors.New("plan SHA-256 confirmation does not match")
		}
	}
	switch command {
	case "apply":
		err = migrationplan.Apply(ctx, db, plan, opts.maxRows)
	case "verify":
		var phase string
		phase, err = migrationplan.Verify(ctx, db, plan, opts.maxRows)
		if err == nil {
			printPlan("verify", database, plan, digest, phase)
		}
		return err
	case "activate":
		err = migrationplan.Activate(ctx, db, plan, opts.maxRows)
	case "deactivate":
		err = migrationplan.Deactivate(ctx, db, plan)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		return err
	}
	printPlan(command, database, plan, digest, "ok")
	return nil
}

func loadDSN(path string, stdin io.Reader) (*mysql.Config, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(io.LimitReader(stdin, 4*1024*1024+1))
		if len(data) > 4*1024*1024 {
			return nil, errors.New("configuration from stdin exceeds 4 MiB")
		}
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Storage.Type != "blessing_skin" || cfg.Storage.BlessingSkinOptions.DatabaseDSN == "" {
		return nil, errors.New("migration requires blessing_skin storage and its configured DSN")
	}
	dsn, err := mysql.ParseDSN(cfg.Storage.BlessingSkinOptions.DatabaseDSN)
	if err != nil {
		return nil, errors.New("invalid configured database DSN")
	}
	if !dsn.ParseTime || dsn.DBName == "" {
		return nil, errors.New("migration DSN requires parseTime=true and an explicit database")
	}
	if err := cfg.PrepareBlessingSkinDSN(dsn); err != nil {
		return nil, fmt.Errorf("invalid migration database transport: %w", err)
	}
	return dsn, nil
}

func openDatabase(cfg *mysql.Config) (*sql.DB, error) {
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(3 * time.Minute)
	return db, nil
}

func requireDatabaseConfirmation(actual, confirmed string) error {
	if confirmed == "" || confirmed != actual {
		return fmt.Errorf("write command requires -confirm-database=%q", actual)
	}
	return nil
}

func privatePlanPath(path string) (string, error) {
	workdir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(filepath.Join(workdir, ".local", "shared-auth"))
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("plan must be a file under .local/shared-auth")
	}
	return candidate, nil
}

func printPlan(command, database string, plan migrationplan.Plan, digest, status string) {
	fmt.Printf("command=%s database=%q migration_id=%s plan_sha256=%s status=%q players=%d mappings=%d active=%d blocked=%d reserved=%d invalid_mappings=%d watermark=%d\n",
		command, database, plan.MigrationID, digest, status, plan.Summary.Players, plan.Summary.Mappings,
		plan.Summary.Active, plan.Summary.Blocked, plan.Summary.Reserved, plan.Summary.InvalidMappings, plan.PlayerHighWatermark)
}
