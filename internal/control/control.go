package control

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func SocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "livesplit-overlay.sock")
}

func Listen(path string) (net.Listener, error) {
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		conn.Close()
		return nil, fmt.Errorf("another livesplit-overlay is already listening on %s", path)
	}
	os.Remove(path)
	return net.Listen("unix", path)
}

func Serve(ctx context.Context, ln net.Listener, handle func(cmd string) error) {
	stop := context.AfterFunc(ctx, func() { ln.Close() })
	defer stop()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serveConn(conn, handle)
	}
}

func serveConn(conn net.Conn, handle func(cmd string) error) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		cmd := strings.TrimSpace(sc.Text())
		if cmd == "" {
			continue
		}
		if err := handle(cmd); err != nil {
			fmt.Fprintf(conn, "error: %v\n", err)
		} else {
			fmt.Fprintln(conn, "ok")
		}
	}
}

func Send(path, cmd string) error {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return fmt.Errorf("livesplit-overlay is not running (%w)", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintln(conn, cmd); err != nil {
		return err
	}
	conn.(*net.UnixConn).CloseWrite()
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("no reply: %w", err)
	}
	reply = strings.TrimSpace(reply)
	if msg, ok := strings.CutPrefix(reply, "error: "); ok {
		return errors.New(msg)
	}
	return nil
}
