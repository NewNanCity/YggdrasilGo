// Package mysqltest creates disposable, local-only MySQL fixtures without external DSNs.
package mysqltest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

const Image = "mysql:8.0.46@sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b"

func localEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" || strings.ContainsAny(endpoint, `\%?#`) {
		return false
	}
	switch parsed.Scheme {
	case "npipe":
		name, found := strings.CutPrefix(parsed.Path, "//./pipe/")
		return found && name != "" && name != "." && name != ".." && !strings.Contains(name, "/")
	case "unix":
		return strings.HasPrefix(endpoint, "unix:///") && parsed.Path != "/" && path.Clean(parsed.Path) == parsed.Path
	default:
		return false
	}
}

type Server struct {
	config *mysql.Config
}

func randomHex(t *testing.T, size int) string {
	t.Helper()
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value)
}

// Start requires explicit opt-in and only accepts a local Docker socket.
func Start(t *testing.T) *Server {
	t.Helper()
	if os.Getenv("YGG_TEST_MYSQL") != "1" {
		t.Skip("set YGG_TEST_MYSQL=1 to create a disposable local MySQL container")
	}
	dockerContext := "default"
	if runtime.GOOS == "windows" {
		dockerContext = "desktop-linux"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	endpoint, err := exec.CommandContext(ctx, "docker", "context", "inspect", dockerContext, "--format", "{{.Endpoints.docker.Host}}").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect local Docker context: %v", err)
	}
	host := strings.TrimSpace(string(endpoint))
	if !localEndpoint(host) {
		t.Fatal("refusing non-local Docker endpoint")
	}
	// Pin the validated endpoint so later context edits cannot redirect this fixture.
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "docker", append([]string{"--host", host}, args...)...).CombinedOutput()
	}
	password := randomHex(t, 32)
	args := []string{"--host", host, "create", "--pull=never",
		"--label", "ygg-go.test=shared-auth", "--name", "ygg-go-test-" + randomHex(t, 8),
		"--publish", "127.0.0.1::3306", "--memory", "1g", "--cpus", "2",
		"--tmpfs", "/var/lib/mysql:rw,nosuid,size=768m",
		"--env", "MYSQL_ROOT_PASSWORD", "--env", "MYSQL_ROOT_HOST=%",
		Image, "--innodb-buffer-pool-size=64M", "--innodb-redo-log-capacity=16M"}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Env = append(os.Environ(), "MYSQL_ROOT_PASSWORD="+password)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("create disposable MySQL: %v: %s", err, output)
	}
	id := strings.TrimSpace(string(output))
	if decoded, err := hex.DecodeString(id); err != nil || len(decoded) != 32 {
		t.Fatal("Docker did not return a valid container ID")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		// The exact ID was created by this test; no user containers or host files are touched.
		if output, err := run(cleanup, "rm", "--force", "--volumes", id); err != nil {
			t.Errorf("remove disposable MySQL %s: %v: %s", id, err, output)
		}
	})
	if output, err := run(ctx, "start", id); err != nil {
		t.Fatalf("start disposable MySQL: %v: %s", err, output)
	}
	output, err = run(ctx, "inspect", "--format", `{{json (index .NetworkSettings.Ports "3306/tcp")}}`, id)
	if err != nil {
		t.Fatalf("inspect test port: %v", err)
	}
	var ports []struct{ HostIP, HostPort string }
	if err := json.Unmarshal(output, &ports); err != nil || len(ports) != 1 || ports[0].HostIP != "127.0.0.1" {
		t.Fatal("test database must expose exactly one loopback port")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = "root", password
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort("127.0.0.1", ports[0].HostPort)
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = time.Second, 5*time.Second, 5*time.Second
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	startupLogger := cfg.Logger
	// Connection resets are expected while the image's entrypoint initializes MySQL.
	cfg.Logger = log.New(io.Discard, "", 0)
	db := open(t, cfg)
	defer db.Close()
	for {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("disposable MySQL did not become ready within 60 seconds")
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Log("isolated MySQL 8.0.46 ready; container data will be removed by test cleanup")
	cfg.Logger = startupLogger
	return &Server{config: cfg}
}

// Restricted opens a second pool using only the proposed runtime table privileges.
func (s *Server) Restricted(t *testing.T, fixture *sql.DB) *sql.DB {
	t.Helper()
	var database string
	if err := fixture.QueryRow("SELECT DATABASE()").Scan(&database); err != nil {
		t.Fatal(err)
	}
	suffix := strings.TrimPrefix(database, "ygg_test_")
	if decoded, err := hex.DecodeString(suffix); !strings.HasPrefix(database, "ygg_test_") || err != nil || len(decoded) != 8 {
		t.Fatal("restricted fixture requires a database created by this helper")
	}
	cfg := s.config.Clone()
	cfg.InterpolateParams = true
	admin := open(t, cfg)
	defer admin.Close()
	username, password := "ygg_"+randomHex(t, 8), randomHex(t, 32)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE USER ? IDENTIFIED BY ?", username, password); err != nil {
		t.Fatal("create restricted test principal failed")
	}
	grants := []struct{ table, privileges string }{
		{"users", "SELECT"}, {"players", "SELECT"}, {"ygg_go_state", "SELECT"},
		{"ygg_go_identities", "SELECT, INSERT, UPDATE (state, updated_at)"},
		{"ygg_go_auth_subjects", "SELECT, INSERT, UPDATE"},
		{"ygg_go_tokens", "SELECT, INSERT, UPDATE, DELETE"},
		{"ygg_go_join_sessions", "SELECT, INSERT, UPDATE, DELETE"},
	}
	for _, grant := range grants {
		if _, err := admin.ExecContext(ctx, "GRANT "+grant.privileges+" ON "+database+"."+grant.table+" TO ?", username); err != nil {
			t.Fatalf("grant restricted test privilege on %s failed", grant.table)
		}
	}
	cfg.User, cfg.Passwd, cfg.DBName = username, password, database
	cfg.InterpolateParams = false
	db := open(t, cfg)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Database creates a fresh schema inside the container owned by Start.
func (s *Server) Database(t *testing.T) *sql.DB {
	t.Helper()
	cfg := s.config.Clone()
	admin := open(t, cfg)
	defer admin.Close()
	name := "ygg_test_" + randomHex(t, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create synthetic schema: %v", err)
	}
	cfg.DBName = name
	db := open(t, cfg)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(fmt.Errorf("close synthetic schema: %w", err))
		}
	})
	return db
}

func open(t *testing.T, cfg *mysql.Config) *sql.DB {
	t.Helper()
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal("invalid isolated MySQL connection settings")
	}
	return sql.OpenDB(connector)
}
