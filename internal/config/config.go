package config

import "errors"

// Config is the root application config.
type Config struct {
	Server Server `json:"server" desc:"服务端配置"`
}

// Server contains backend runtime settings.
//
//nolint:tagliatelle // public config keys are kebab-case.
type Server struct {
	Database string        `json:"database" desc:"SQLite 数据库文件路径"`
	HTTP     ServerHTTP    `json:"http" desc:"HTTP 服务配置"`
	Control  ServerControl `json:"control" desc:"本地控制面配置"`
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

// DefaultConfig returns a runnable baseline config.
func DefaultConfig() Config {
	return Config{
		Server: Server{
			Database: ".local/data/app.db",
			HTTP: ServerHTTP{
				Listen: ":40238",
			},
			Control: ServerControl{
				Listen: ".local/run/control.sock",
			},
		},
	}
}
