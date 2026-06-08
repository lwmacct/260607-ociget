package config

import (
	"errors"
	"time"
)

// Config is the root application config.
type Config struct {
	Server Server `json:"server" desc:"服务端配置"`
}

// Server contains backend runtime settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type Server struct {
	Database      string              `json:"database" desc:"SQLite 数据库文件路径"`
	HTTP          ServerHTTP          `json:"http" desc:"HTTP 服务配置"`
	Control       ServerControl       `json:"control" desc:"本地控制面配置"`
	DownloadCache ServerDownloadCache `json:"download-cache" desc:"下载文件缓存配置"`
}

// ServerHTTP contains HTTP listener settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type ServerHTTP struct {
	Listen      string `json:"listen" desc:"HTTP 服务监听地址"`
	SSLCertFile string `json:"ssl-cert-file" desc:"SSL 证书文件路径"`
	SSLKeyFile  string `json:"ssl-key-file" desc:"SSL 密钥文件路径"`
}

// ServerControl contains local control socket settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type ServerControl struct {
	Listen string `json:"listen" desc:"Unix socket path for local control commands"`
}

// ServerDownloadCache contains extracted download cache settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type ServerDownloadCache struct {
	Enabled bool   `json:"enabled" desc:"是否启用下载文件缓存"`
	Dir     string `json:"dir" desc:"下载文件缓存目录"`
	TTL     string `json:"ttl" desc:"下载文件缓存有效期, 例如 168h"`
}

// Validate checks internal field consistency.
func (h ServerHTTP) Validate() error {
	if (h.SSLCertFile == "") != (h.SSLKeyFile == "") {
		return errors.New("http ssl-cert-file and ssl-key-file must be configured together")
	}
	return nil
}

// UsesTLS reports whether HTTPS should be enabled.
func (h ServerHTTP) UsesTLS() bool {
	return h.SSLCertFile != "" && h.SSLKeyFile != ""
}

// Validate checks download cache settings.
func (c ServerDownloadCache) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Dir == "" {
		return errors.New("download-cache dir is required when enabled")
	}
	ttl, err := c.TTLDuration()
	if err != nil {
		return err
	}
	if ttl < 0 {
		return errors.New("download-cache ttl must not be negative")
	}
	return nil
}

// TTLDuration parses the cache TTL.
func (c ServerDownloadCache) TTLDuration() (time.Duration, error) {
	if c.TTL == "" {
		return 0, nil
	}
	ttl, err := time.ParseDuration(c.TTL)
	if err != nil {
		return 0, errors.New("download-cache ttl must be a Go duration such as 168h")
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
			},
			Control: ServerControl{
				Listen: ".local/run/control.sock",
			},
			DownloadCache: ServerDownloadCache{
				Enabled: true,
				Dir:     ".local/cache/downloads",
				TTL:     "168h",
			},
		},
	}
}
