package config

import (
	"errors"
	"time"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/tlsreload"
)

// Config is the root application config.
type Config struct {
	Server Server `json:"server" desc:"服务端配置"`
}

// Manager owns configuration loading, schema validation, and CLI integration.
var Manager = cfgm.New(DefaultConfig())

// Server contains backend runtime settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type Server struct {
	Database   string           `json:"database" desc:"SQLite 数据库文件路径"`
	HTTP       ServerHTTP       `json:"http" desc:"HTTP 服务配置"`
	ImageStore ServerImageStore `json:"image-store" desc:"镜像文件系统仓库配置"`
}

// ServerHTTP contains HTTP listener settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type ServerHTTP struct {
	Listen string           `json:"listen" desc:"HTTP 服务监听地址"`
	TLS    tlsreload.Config `json:"tls" desc:"HTTPS TLS 配置"`
}

// ServerImageStore contains local immutable image filesystem settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type ServerImageStore struct {
	Dir    string `json:"dir" desc:"镜像元数据目录"`
	RefTTL string `json:"ref-ttl" desc:"可变镜像引用重新解析间隔, 例如 5m"`
}

// Validate checks internal field consistency.
func (h ServerHTTP) Validate() error {
	if !h.TLS.Enabled {
		return nil
	}
	return h.TLS.Validate()
}

// UsesTLS reports whether HTTPS should be enabled.
func (h ServerHTTP) UsesTLS() bool {
	return h.TLS.Enabled
}

// Validate checks image store settings.
func (c ServerImageStore) Validate() error {
	if c.Dir == "" {
		return errors.New("image-store dir is required")
	}
	ttl, err := c.RefTTLDuration()
	if err != nil {
		return err
	}
	if ttl < 0 {
		return errors.New("image-store ref-ttl must not be negative")
	}
	return nil
}

// RefTTLDuration parses the mutable reference refresh interval.
func (c ServerImageStore) RefTTLDuration() (time.Duration, error) {
	if c.RefTTL == "" {
		return 0, nil
	}
	ttl, err := time.ParseDuration(c.RefTTL)
	if err != nil {
		return 0, errors.New("image-store ref-ttl must be a Go duration such as 5m")
	}
	return ttl, nil
}

// DefaultConfig returns a runnable baseline config.
func DefaultConfig() Config {
	return Config{
		Server: Server{
			Database: ".local/data/app.db",
			HTTP: ServerHTTP{
				Listen: ":40248",
				TLS: func() tlsreload.Config {
					config := tlsreload.DefaultConfig()
					config.DefaultCertificate = "default"
					config.Certificates = []tlsreload.CertificateSource{
						{
							ID:          "default",
							Certificate: "${APP_DATA:-.local/data}/ssl/fullchain.pem",
							PrivateKey:  "${APP_DATA:-.local/data}/ssl/privkey.pem",
						},
					}
					return config
				}(),
			},
			ImageStore: ServerImageStore{Dir: ".local/image-metadata", RefTTL: "5m"},
		},
	}
}
