package server

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type controlPlane struct {
	socketPath string
	tlsManager *TLSManager

	mu       sync.Mutex
	listener net.Listener
}

func newControlPlane(socketPath string, tlsManager *TLSManager) *controlPlane {
	return &controlPlane{
		socketPath: socketPath,
		tlsManager: tlsManager,
	}
}

func (p *controlPlane) start(ctx context.Context) error {
	if p.socketPath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(p.socketPath), 0o700); err != nil {
		return fmt.Errorf("prepare control socket directory: %w", err)
	}
	_ = os.Remove(p.socketPath)

	ln, err := net.Listen("unix", p.socketPath)
	if err != nil {
		return fmt.Errorf("listen control socket: %w", err)
	}

	slog.Info("control socket listening", "socket", p.socketPath)

	p.mu.Lock()
	p.listener = ln
	p.mu.Unlock()

	go func() {
		<-ctx.Done()
		p.stop()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				p.stop()
				return
			}
			go p.handleConn(conn)
		}
	}()

	return nil
}

func (p *controlPlane) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener != nil {
		_ = p.listener.Close()
		_ = os.Remove(p.socketPath)
		p.listener = nil
	}
}

func (p *controlPlane) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		_, _ = conn.Write([]byte("ERR empty command\n"))
		return
	}

	cmd := strings.TrimSpace(scanner.Text())
	switch cmd {
	case "reload-cert":
		if p.tlsManager == nil {
			_, _ = conn.Write([]byte("ERR tls is disabled\n"))
			return
		}
		if err := p.tlsManager.Reload(); err != nil {
			_, _ = conn.Write([]byte("ERR " + err.Error() + "\n"))
			return
		}
		_, _ = conn.Write([]byte("OK reloaded\n"))
	default:
		_, _ = conn.Write([]byte("ERR unknown command\n"))
	}
}
