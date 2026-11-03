package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"goapache-manager/internal/helper"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const socketPath = "/run/goapache/helper.sock"

type safeListener struct {
	net.Listener
	expectedUID uint32
}

var domainPattern = regexp.MustCompile(
	`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`,
)

func createVHost(
	ctx context.Context,
	rawDomain string,
) (string, error) {
	// First implementation: Debian/Ubuntu only.
	if _, err := os.Stat("/etc/apache2"); err != nil {
		return "", fmt.Errorf("this operation currently supports Debian/Ubuntu Apache only")
	}

	domain := strings.ToLower(strings.TrimSpace(rawDomain))

	if len(domain) == 0 || len(domain) > 253 ||
		!domainPattern.MatchString(domain) {
		return "", fmt.Errorf("invalid domain name")
	}

	if domain == "localhost" {
		return "", fmt.Errorf("localhost cannot be used as a managed virtual host")
	}

	sitesAvailable := "/etc/apache2/sites-available"
	sitesEnabled := "/etc/apache2/sites-enabled"
	webRoot := "/var/www"

	// Verify expected directories and reject symlinks.
	for _, dir := range []string{
		sitesAvailable,
		sitesEnabled,
		webRoot,
	} {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", fmt.Errorf("required directory %s: %w", dir, err)
		}

		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe directory: %s", dir)
		}
	}

	filename := domain + ".conf"
	configPath := filepath.Join(sitesAvailable, filename)
	enabledPath := filepath.Join(sitesEnabled, filename)
	documentRoot := filepath.Join(webRoot, domain)

	// Never overwrite an existing configuration or symlink.
	if _, err := os.Lstat(configPath); err == nil {
		return "", fmt.Errorf("virtual host configuration already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if _, err := os.Lstat(enabledPath); err == nil {
		return "", fmt.Errorf("virtual host is already enabled")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	documentCreated := false

	if info, err := os.Lstat(documentRoot); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("document root exists but is not a safe directory")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(documentRoot, 0755); err != nil {
			return "", fmt.Errorf("create document root: %w", err)
		}
		documentCreated = true
	} else {
		return "", err
	}

	configCreated := false
	linkCreated := false

	cleanup := func() {
		if linkCreated {
			_ = os.Remove(enabledPath)
		}

		if configCreated {
			_ = os.Remove(configPath)
		}

		if documentCreated {
			_ = os.Remove(documentRoot)
		}
	}

	config := fmt.Sprintf(`# Managed by GoApache Manager: %s
	<VirtualHost *:80>
    ServerName %s

    DocumentRoot %s

    <Directory %s>
        Options FollowSymLinks
        AllowOverride None
        Require all granted
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/%s-error.log
    CustomLog ${APACHE_LOG_DIR}/%s-access.log combined
</VirtualHost>
`, domain, domain, documentRoot, documentRoot, domain, domain)

	file, err := os.OpenFile(
		configPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0644,
	)
	if err != nil {
		if documentCreated {
			_ = os.Remove(documentRoot)
		}

		return "", fmt.Errorf("create config: %w", err)
	}

	configCreated = true

	_, writeErr := file.WriteString(config)
	syncErr := file.Sync()
	closeErr := file.Close()

	if writeErr != nil || syncErr != nil || closeErr != nil {
		cleanup()

		return "", fmt.Errorf(
			"write apache configuration: %w",
			errors.Join(writeErr, syncErr, closeErr),
		)
	}

	// Enable using a fixed relative symlink.
	if err := os.Symlink(
		"../sites-available/"+domain+".conf",
		enabledPath,
	); err != nil {
		cleanup()
		return "", fmt.Errorf("enable vhost: %w", err)
	}

	linkCreated = true

	// Validate before reloading Apache.
	testOutput, err := runApacheCommand(ctx, "-t")
	if err != nil {
		cleanup()
		return testOutput, fmt.Errorf("apache configtest failed: %w", err)
	}

	reloadOutput, err := runSystemctl(ctx, "reload", "apache2")
	if err != nil {
		cleanup()

		// Best-effort validation and reload of the previous configuration.
		rollbackCtx, cancel := context.WithTimeout(
			context.Background(),
			20*time.Second,
		)
		defer cancel()

		if _, rollbackErr := runApacheCommand(rollbackCtx, "-t"); rollbackErr == nil {
			_, _ = runSystemctl(rollbackCtx, "reload", "apache2")
		}

		return reloadOutput, fmt.Errorf("apache reload failed: %w", err)
	}

	return fmt.Sprintf(
		"VirtualHost created successfully\nDomain: %s\nDocumentRoot: %s\nConfig: %s\n%s",
		domain,
		documentRoot,
		configPath,
		testOutput,
	), nil
}
func setVHostEnabled(
	ctx context.Context,
	rawDomain string,
	enabled bool,
) (string, error) {
	if _, err := os.Stat("/etc/apache2"); err != nil {
		return "", fmt.Errorf("this operation currently supports Debian/Ubuntu only")
	}

	domain := strings.ToLower(strings.TrimSpace(rawDomain))

	if len(domain) == 0 ||
		len(domain) > 253 ||
		!domainPattern.MatchString(domain) ||
		domain == "localhost" {
		return "", fmt.Errorf("invalid domain name")
	}

	sitesAvailable := "/etc/apache2/sites-available"
	sitesEnabled := "/etc/apache2/sites-enabled"

	for _, dir := range []string{sitesAvailable, sitesEnabled} {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}

		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe directory: %s", dir)
		}
	}

	filename := domain + ".conf"
	configPath := filepath.Join(sitesAvailable, filename)
	enabledPath := filepath.Join(sitesEnabled, filename)

	// Only modify configurations created by GoApache Manager.
	configInfo, err := os.Lstat(configPath)
	if err != nil {
		return "", fmt.Errorf("configuration not found: %w", err)
	}

	if !configInfo.Mode().IsRegular() {
		return "", fmt.Errorf("configuration is not a regular file")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	marker := "# Managed by GoApache Manager: " + domain

	firstLine := strings.SplitN(string(content), "\n", 2)[0]
	if firstLine != marker {
		return "", fmt.Errorf("configuration is not managed by GoApache Manager")
	}

	expectedTarget := "../sites-available/" + filename

	// Determine the current enabled state.
	linkTarget, linkErr := os.Readlink(enabledPath)
	isEnabled := linkErr == nil

	if linkErr != nil && !errors.Is(linkErr, os.ErrNotExist) {
		return "", fmt.Errorf("cannot inspect enabled configuration: %w", linkErr)
	}

	if isEnabled && linkTarget != expectedTarget {
		return "", fmt.Errorf("enabled path is not the expected GoApache symlink")
	}

	if enabled && isEnabled {
		return "VirtualHost is already enabled", nil
	}

	if !enabled && !isEnabled {
		return "VirtualHost is already disabled", nil
	}

	// Apply the requested change.
	if enabled {
		if err := os.Symlink(expectedTarget, enabledPath); err != nil {
			return "", fmt.Errorf("enable VirtualHost: %w", err)
		}
	} else {
		if err := os.Remove(enabledPath); err != nil {
			return "", fmt.Errorf("disable VirtualHost: %w", err)
		}
	}

	// Restore the original state if validation or reload fails.
	rollback := func() {
		if enabled {
			_ = os.Remove(enabledPath)
		} else {
			_ = os.Symlink(expectedTarget, enabledPath)
		}

		rollbackCtx, cancel := context.WithTimeout(
			context.Background(),
			20*time.Second,
		)
		defer cancel()

		if _, err := runApacheCommand(rollbackCtx, "-t"); err == nil {
			_, _ = runSystemctl(
				rollbackCtx,
				"reload",
				apacheService(),
			)
		}
	}

	testOutput, err := runApacheCommand(ctx, "-t")
	if err != nil {
		rollback()
		return testOutput, fmt.Errorf("apache configtest failed: %w", err)
	}

	reloadOutput, err := runSystemctl(
		ctx,
		"reload",
		apacheService(),
	)
	if err != nil {
		rollback()
		return reloadOutput, fmt.Errorf("apache reload failed: %w", err)
	}

	operation := "disabled"
	if enabled {
		operation = "enabled"
	}

	return fmt.Sprintf(
		"VirtualHost %s successfully\nDomain: %s\n%s",
		operation,
		domain,
		testOutput,
	), nil
}

func editVHost(
	ctx context.Context,
	rawDomain string,
	documentRoot string,
	serverAliases []string,
) (string, error) {
	if _, err := os.Stat("/etc/apache2"); err != nil {
		return "", fmt.Errorf("this operation currently supports Debian/Ubuntu only")
	}

	domain := strings.ToLower(strings.TrimSpace(rawDomain))
	if len(domain) == 0 || len(domain) > 253 ||
		!domainPattern.MatchString(domain) || domain == "localhost" {
		return "", fmt.Errorf("invalid domain name")
	}

	documentRoot = strings.TrimSpace(documentRoot)
	if !filepath.IsAbs(documentRoot) || filepath.Clean(documentRoot) != documentRoot {
		return "", fmt.Errorf("document root must be an absolute clean path")
	}

	// DocumentRoot must be a real directory under /var/www.
	const webRoot = "/var/www"
	rel, err := filepath.Rel(webRoot, documentRoot)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("document root must be inside /var/www")
	}

	for _, dir := range []string{webRoot, documentRoot} {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", fmt.Errorf("inspect directory %s: %w", dir, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe directory: %s", dir)
		}
	}

	// Reject symlinks in every intermediate DocumentRoot component.
	current := webRoot
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("inspect document root component: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe document root component: %s", current)
		}
	}

	sitesAvailable := "/etc/apache2/sites-available"
	sitesEnabled := "/etc/apache2/sites-enabled"

	for _, dir := range []string{sitesAvailable, sitesEnabled} {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe directory: %s", dir)
		}
	}

	configPath := filepath.Join(sitesAvailable, domain+".conf")

	info, err := os.Lstat(configPath)
	if err != nil {
		return "", fmt.Errorf("configuration not found: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("configuration is not a regular file")
	}

	oldContent, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	marker := "# Managed by GoApache Manager: " + domain
	firstLine := strings.SplitN(string(oldContent), "\n", 2)[0]
	if firstLine != marker {
		return "", fmt.Errorf("configuration is not managed by GoApache Manager")
	}

	// Validate aliases and reject duplicates.
	seen := map[string]bool{domain: true}
	aliases := make([]string, 0, len(serverAliases))

	for _, rawAlias := range serverAliases {
		alias := strings.ToLower(strings.TrimSpace(rawAlias))
		if alias == "" || len(alias) > 253 ||
			!domainPattern.MatchString(alias) || alias == "localhost" {
			return "", fmt.Errorf("invalid ServerAlias: %q", rawAlias)
		}
		if seen[alias] {
			return "", fmt.Errorf("duplicate ServerAlias: %s", alias)
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}

	aliasConfig := ""
	if len(aliases) > 0 {
		aliasConfig = "\tServerAlias " + strings.Join(aliases, " ") + "\n"
	}

	newConfig := fmt.Sprintf(`# Managed by GoApache Manager: %s
<VirtualHost *:80>
	ServerName %s
%s
	DocumentRoot %s

	<Directory %s>
		Options FollowSymLinks
		AllowOverride None
		Require all granted
	</Directory>

	ErrorLog ${APACHE_LOG_DIR}/%s-error.log
	CustomLog ${APACHE_LOG_DIR}/%s-access.log combined
</VirtualHost>
`, domain, domain, aliasConfig, documentRoot, documentRoot, domain, domain)

	// Create a unique backup before modifying the configuration.
	backupPath := configPath + ".bak-" + time.Now().UTC().Format("20060102T150405.000000000")
	backup, err := os.OpenFile(
		backupPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		return "", fmt.Errorf("create configuration backup: %w", err)
	}

	_, writeErr := backup.Write(oldContent)
	syncErr := backup.Sync()
	closeErr := backup.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("write configuration backup: %w", err)
	}

	// Atomic replacement using a temporary file in the same directory.
	writeAtomic := func(content []byte) error {
		tmp, err := os.CreateTemp(sitesAvailable, ".goapache-*.tmp")
		if err != nil {
			return err
		}

		tmpName := tmp.Name()
		defer os.Remove(tmpName)

		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			_ = tmp.Close()
			return err
		}

		_, writeErr := tmp.Write(content)
		syncErr := tmp.Sync()
		closeErr := tmp.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return err
		}

		return os.Rename(tmpName, configPath)
	}

	if err := writeAtomic([]byte(newConfig)); err != nil {
		return "", fmt.Errorf("write new configuration: %w", err)
	}

	rollback := func() {
		_ = writeAtomic(oldContent)

		rollbackCtx, cancel := context.WithTimeout(
			context.Background(),
			20*time.Second,
		)
		defer cancel()

		if _, err := runApacheCommand(rollbackCtx, "-t"); err == nil {
			_, _ = runSystemctl(rollbackCtx, "reload", apacheService())
		}
	}

	testOutput, err := runApacheCommand(ctx, "-t")
	if err != nil {
		rollback()
		return testOutput, fmt.Errorf("apache configtest failed: %w", err)
	}

	reloadOutput, err := runSystemctl(ctx, "reload", apacheService())
	if err != nil {
		rollback()
		return reloadOutput, fmt.Errorf("apache reload failed: %w", err)
	}

	return fmt.Sprintf(
		"VirtualHost updated successfully\nDomain: %s\nDocumentRoot: %s\nBackup: %s\n%s",
		domain,
		documentRoot,
		backupPath,
		testOutput,
	), nil
}

func runApacheCommand(
	ctx context.Context,
	args ...string,
) (string, error) {
	cmd := exec.CommandContext(ctx, apacheControl(), args...)
	output, err := cmd.CombinedOutput()

	return strings.TrimSpace(string(output)), err
}

func runSystemctl(
	ctx context.Context,
	args ...string,
) (string, error) {
	cmd := exec.CommandContext(
		ctx,
		"/usr/bin/systemctl",
		append([]string{"--no-ask-password"}, args...)...,
	)

	output, err := cmd.CombinedOutput()

	return strings.TrimSpace(string(output)), err
}
func (l *safeListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		unixConn, ok := conn.(*net.UnixConn)
		if !ok {
			conn.Close()
			continue
		}

		raw, err := unixConn.SyscallConn()
		if err != nil {
			conn.Close()
			continue
		}

		var cred *syscall.Ucred
		var controlErr error

		err = raw.Control(func(fd uintptr) {
			cred, controlErr = syscall.GetsockoptUcred(
				int(fd),
				syscall.SOL_SOCKET,
				syscall.SO_PEERCRED,
			)
		})

		if err != nil || controlErr != nil ||
			cred == nil || cred.Uid != l.expectedUID {
			conn.Close()
			continue
		}

		return conn, nil
	}
}

func main() {
	if runtime.GOOS != "linux" {
		log.Fatal("Helper only supports Linux")
	}

	if os.Geteuid() != 0 {
		log.Fatal("Helper must run as root")
	}

	clientUser, err := user.Lookup("goapache")
	if err != nil {
		log.Fatal("goapache system user not found")
	}

	uid, err := strconv.ParseUint(clientUser.Uid, 10, 32)
	if err != nil {
		log.Fatal(err)
	}
	// Create and secure socket directory
	socketDir := filepath.Dir(socketPath)

	if err := os.MkdirAll(socketDir, 0750); err != nil {
		log.Fatalf("create socket directory: %v", err)
	}

	gid := mustGroupID("goapache")

	if err := os.Chown(socketDir, 0, gid); err != nil {
		log.Fatalf("chown socket directory: %v", err)
	}

	if err := os.Chmod(socketDir, 0750); err != nil {
		log.Fatalf("chmod socket directory: %v", err)
	}

	// Remove stale socket
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("remove old socket: %v", err)
	}

	// Create Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listen unix socket: %v", err)
	}
	defer os.Remove(socketPath)
	defer listener.Close()

	if err := os.Chown(socketPath, 0, gid); err != nil {
		log.Fatalf("chown socket: %v", err)
	}

	if err := os.Chmod(socketPath, 0660); err != nil {
		log.Fatal(err)
	}

	sl := &safeListener{
		Listener:    listener,
		expectedUID: uint32(uid),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/execute", executeHandler)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      35 * time.Second,
	}

	log.Println("GoApache privileged helper started")

	if err := server.Serve(sl); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func mustGroupID(name string) int {
	g, err := user.LookupGroup(name)
	if err != nil {
		log.Fatal(err)
	}

	id, err := strconv.Atoi(g.Gid)
	if err != nil {
		log.Fatal(err)
	}

	return id
}

func executeHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var req helper.Request

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		30*time.Second,
	)
	defer cancel()

	output, err := executeRequest(ctx, req)

	result := helper.Response{
		Success: err == nil,
		Output:  output,
	}

	if err != nil {
		result.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	_ = json.NewEncoder(w).Encode(result)
}
func executeRequest(
	ctx context.Context,
	req helper.Request,
) (string, error) {
	switch req.Action {
	case "apache.vhost.create":
		return createVHost(ctx, req.Domain)

	case "apache.vhost.enable":
		return setVHostEnabled(ctx, req.Domain, true)

	case "apache.vhost.disable":
		return setVHostEnabled(ctx, req.Domain, false)
	case "apache.vhost.edit":
		return editVHost(
			ctx,
			req.Domain,
			req.DocumentRoot,
			req.ServerAliases,
		)
	default:
		return execute(ctx, req.Action)
	}
}
func execute(
	ctx context.Context,
	action string,
) (string, error) {

	commands := map[string]struct {
		path string
		args []string
	}{
		"apache.status": {
			path: "/usr/bin/systemctl",
			args: []string{"is-active", apacheService()},
		},
		"apache.configtest": {
			path: apacheControl(),
			args: []string{"-t"},
		},
		"apache.reload": {
			path: "/usr/bin/systemctl",
			args: []string{"reload", apacheService()},
		},
		"apache.restart": {
			path: "/usr/bin/systemctl",
			args: []string{"restart", apacheService()},
		},
		"apache.vhosts.list": {
			path: apacheControl(),
			args: []string{"-S"},
		},
	}

	cmdSpec, ok := commands[action]
	if !ok {
		return "", fmt.Errorf("operation not allowed")
	}

	cmd := exec.CommandContext(
		ctx,
		cmdSpec.path,
		cmdSpec.args...,
	)

	output, err := cmd.CombinedOutput()

	if err != nil {
		return strings.TrimSpace(string(output)), err
	}

	return strings.TrimSpace(string(output)), nil
}

func apacheService() string {
	if _, err := os.Stat("/etc/apache2"); err == nil {
		return "apache2"
	}

	return "httpd"
}

func apacheControl() string {
	if _, err := os.Stat("/etc/apache2"); err == nil {
		return "/usr/sbin/apache2ctl"
	}

	return "/usr/sbin/apachectl"
}
