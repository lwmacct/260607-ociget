package server

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
)

type TLSManager struct {
	certPath string
	keyPath  string

	reloadMu sync.Mutex
	cert     atomic.Pointer[tls.Certificate]
}

func NewTLSManager(certPath, keyPath string) (*TLSManager, error) {
	m := &TLSManager{
		certPath: certPath,
		keyPath:  keyPath,
	}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *TLSManager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := m.cert.Load()
	if cert == nil {
		return nil, fmt.Errorf("tls certificate not loaded")
	}

	return cert, nil
}

func (m *TLSManager) Reload() error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	certPEM, keyPEM, err := readTLSFiles(m.certPath, m.keyPath)
	if err != nil {
		return err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("load tls certificate: %w", err)
	}

	m.cert.Store(&cert)

	slog.Info("tls certificate loaded", "cert_file", m.certPath, "key_file", m.keyPath)
	return nil
}

func readTLSFiles(certPath, keyPath string) ([]byte, []byte, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read tls cert file: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read tls key file: %w", err)
	}

	return certPEM, keyPEM, nil
}
