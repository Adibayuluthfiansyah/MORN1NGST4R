// Package config loads and validates the runtime configuration from the
// process environment.
//
// Rules (security.md section 8, API conventions):
//   - The listen address is an explicit loopback IP literal. A wildcard,
//     LAN or hostname address is refused. Remote access needs a separate
//     security review and is not configurable in the MVP.
//   - The Host allow-list contains only loopback names and addresses.
//   - A variable with the MORNINGSTAR_ prefix that this package does not know
//     is an error, so a misspelled setting is never silently ignored. The
//     exceptions are MORNINGSTAR_API_ORIGIN (apps/web) and MORNINGSTAR_PG_BASE
//     (scripts/db/pg-cluster.sh), which belong to other processes and are
//     accepted and ignored here.
//   - The database is reached over a unix socket in a directory (D-67). There is
//     no DSN, no host name, no port and no password setting, so none can be
//     supplied. The roles and the database name are fixed in the code.
//   - No secret is read from here. Secrets are delivered through the
//     credential store (security.md section 7), never through this package.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	envPrefix       = "MORNINGSTAR_"
	envHTTPListen   = "MORNINGSTAR_HTTP_LISTEN"
	envAllowedHosts = "MORNINGSTAR_HTTP_ALLOWED_HOSTS"
	envLogLevel     = "MORNINGSTAR_LOG_LEVEL"
	envLogFormat    = "MORNINGSTAR_LOG_FORMAT"
	envDBSocketDir  = "MORNINGSTAR_DB_SOCKET_DIR"
	envStateDir     = "MORNINGSTAR_STATE_DIR"

	// envWebAPIOrigin is a web-only setting and envScriptPGBase belongs to the
	// cluster script. See Load.
	envWebAPIOrigin = "MORNINGSTAR_API_ORIGIN"
	envScriptPGBase = "MORNINGSTAR_PG_BASE"

	defaultListen = "127.0.0.1:8080"
)

// Config is the validated configuration of the server process.
type Config struct {
	HTTP     HTTP
	Log      Log
	Database Database
	State    State
}

// maxSocketDirLen keeps the socket path inside the 107 bytes that a unix socket
// address allows. The socket file name ".s.PGSQL.5432" takes 14 more.
const maxSocketDirLen = 90

// Database locates the per-user PostgreSQL cluster (D-67).
type Database struct {
	// SocketDir is the directory of the unix socket. It is the only setting.
	SocketDir string
}

// State is the owner-only directory for runtime state outside PostgreSQL, such
// as the recovery marker (recovery-generation.md).
type State struct {
	Dir string
}

// HTTP configures the HTTP server.
type HTTP struct {
	// Listen is the explicit loopback address the server binds.
	Listen netip.AddrPort
	// AllowedHosts is the exact, lower-case "host:port" allow-list that the
	// Host header is checked against.
	AllowedHosts []string

	// Timeouts follow docs/design/api/conventions.md (read-header, read,
	// write, idle). They are not configurable in this phase.
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Log configures structured logging.
type Log struct {
	Level  slog.Level
	Format string // "json" or "text"
}

// Load builds a Config from environ, a list of "KEY=value" strings such as
// os.Environ(). It reports every problem it finds, joined into one error.
func Load(environ []string) (Config, error) {
	values := make(map[string]string)
	var errs []error
	for _, kv := range environ {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, envPrefix) {
			continue
		}
		switch key {
		case envHTTPListen, envAllowedHosts, envLogLevel, envLogFormat, envDBSocketDir, envStateDir:
			values[key] = value
		case envWebAPIOrigin, envScriptPGBase:
			// Owned and validated by apps/web (next.config.ts) and by
			// scripts/db/pg-cluster.sh. Recognised only so one shared
			// environment can serve every process. Never read as a Go setting.
		default:
			errs = append(errs, fmt.Errorf("unknown setting %s", key))
		}
	}

	cfg := Config{
		HTTP: HTTP{
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			ShutdownTimeout:   10 * time.Second,
		},
		Log: Log{Level: slog.LevelInfo, Format: "json"},
	}

	listen, ok := values[envHTTPListen]
	if !ok {
		listen = defaultListen
	}
	addr, err := parseLoopbackListen(listen)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", envHTTPListen, err))
	} else {
		cfg.HTTP.Listen = addr
	}

	if raw, ok := values[envAllowedHosts]; ok {
		hosts, err := parseAllowedHosts(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envAllowedHosts, err))
		} else {
			cfg.HTTP.AllowedHosts = hosts
		}
	} else if err == nil {
		port := strconv.Itoa(int(addr.Port()))
		cfg.HTTP.AllowedHosts = []string{hostPort(addr.Addr(), port), "localhost:" + port}
	}

	if raw, ok := values[envLogLevel]; ok {
		level, err := parseLevel(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envLogLevel, err))
		} else {
			cfg.Log.Level = level
		}
	}
	if raw, ok := values[envLogFormat]; ok {
		if raw != "json" && raw != "text" {
			errs = append(errs, fmt.Errorf("%s: %q is not one of json, text", envLogFormat, raw))
		} else {
			cfg.Log.Format = raw
		}
	}

	env := plainEnv(environ)
	if raw, ok := values[envDBSocketDir]; ok {
		dir, err := parseDir(raw, maxSocketDirLen)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envDBSocketDir, err))
		}
		cfg.Database.SocketDir = dir
	} else if base, err := xdgBase(env, "XDG_DATA_HOME", ".local/share"); err != nil {
		errs = append(errs, fmt.Errorf("%s is not set and %w", envDBSocketDir, err))
	} else {
		dir, err := parseDir(filepath.Join(base, "morningstar", "pg-run"), maxSocketDirLen)
		if err != nil {
			errs = append(errs, fmt.Errorf("default database socket directory: %w; set %s", err, envDBSocketDir))
		}
		cfg.Database.SocketDir = dir
	}
	if raw, ok := values[envStateDir]; ok {
		dir, err := parseDir(raw, 200)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", envStateDir, err))
		}
		cfg.State.Dir = dir
	} else if base, err := xdgBase(env, "XDG_STATE_HOME", ".local/state"); err != nil {
		errs = append(errs, fmt.Errorf("%s is not set and %w", envStateDir, err))
	} else {
		cfg.State.Dir = filepath.Join(base, "morningstar")
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

// plainEnv returns the variables that are not MORNINGSTAR_ settings.
func plainEnv(environ []string) map[string]string {
	out := make(map[string]string)
	for _, kv := range environ {
		if key, value, ok := strings.Cut(kv, "="); ok && !strings.HasPrefix(key, envPrefix) {
			out[key] = value
		}
	}
	return out
}

// xdgBase returns the XDG directory named by key, or $HOME/fallback. A value
// that is not an absolute path is ignored, as the XDG specification says.
func xdgBase(env map[string]string, key, fallback string) (string, error) {
	if v := env[key]; filepath.IsAbs(v) {
		return filepath.Clean(v), nil
	}
	if home := env["HOME"]; filepath.IsAbs(home) {
		return filepath.Join(filepath.Clean(home), filepath.FromSlash(fallback)), nil
	}
	return "", fmt.Errorf("neither %s nor HOME gives an absolute directory", key)
}

// parseDir accepts an absolute, clean directory path of at most max bytes
// without control characters. It does not touch the file system.
func parseDir(raw string, max int) (string, error) {
	switch {
	case raw == "":
		return "", errors.New("is empty")
	case len(raw) > max:
		return "", fmt.Errorf("is longer than %d bytes", max)
	case !filepath.IsAbs(raw):
		return "", errors.New("is not an absolute path")
	case filepath.Clean(raw) != raw:
		return "", errors.New("is not a clean path (no trailing slash, dot elements or doubled slashes)")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f {
			return "", errors.New("contains a control character")
		}
	}
	return raw, nil
}

func parseLoopbackListen(raw string) (netip.AddrPort, error) {
	ap, err := netip.ParseAddrPort(raw)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("%q is not an IP address with a port", raw)
	}
	if ap.Port() == 0 {
		return netip.AddrPort{}, fmt.Errorf("%q: port 0 is not allowed", raw)
	}
	if ap.Addr().Zone() != "" {
		return netip.AddrPort{}, fmt.Errorf("%q has an IPv6 zone. Zones are not allowed", raw)
	}
	if ap.Addr().Is4In6() || !ap.Addr().IsLoopback() {
		return netip.AddrPort{}, fmt.Errorf("%q is not a loopback address. Remote access is not supported", raw)
	}
	return ap, nil
}

func parseAllowedHosts(raw string) ([]string, error) {
	var hosts []string
	seen := make(map[string]bool)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			return nil, errors.New("empty entry")
		}
		host, port, err := net.SplitHostPort(entry)
		if err != nil {
			return nil, fmt.Errorf("%q is not host:port", entry)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return nil, fmt.Errorf("%q has an invalid port", entry)
		}
		if host != "localhost" {
			ip, err := netip.ParseAddr(host)
			if err != nil || ip.Zone() != "" || ip.Is4In6() || !ip.IsLoopback() {
				return nil, fmt.Errorf("%q is not localhost or a loopback address", entry)
			}
		}
		canonical := entry
		if ip, err := netip.ParseAddr(host); err == nil {
			canonical = hostPort(ip, port)
		}
		if !seen[canonical] {
			seen[canonical] = true
			hosts = append(hosts, canonical)
		}
	}
	return hosts, nil
}

func hostPort(ip netip.Addr, port string) string {
	return net.JoinHostPort(ip.String(), port)
}

func parseLevel(raw string) (slog.Level, error) {
	switch raw {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("%q is not one of debug, info, warn, error", raw)
}
