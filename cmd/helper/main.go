package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"goapache-manager/internal/helper"
)

const socketPath = "/run/goapache/helper.sock"

type safeListener struct {
	net.Listener
	expectedUID uint32
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

	output, err := execute(ctx, req.Action)

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
