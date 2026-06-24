package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	TCP      TCPConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Server   ServerConfig
}

type AppConfig struct {
	Name        string        `mapstructure:"name"`
	Environment string        `mapstructure:"environment"`
	LogLevel    string        `mapstructure:"log_level"`
	Debug       bool          `mapstructure:"debug"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type TCPConfig struct {
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port"`
	MaxConnections    int           `mapstructure:"max_connections"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	KeepAlive         time.Duration `mapstructure:"keepalive"`
	ReadBufferSize    int           `mapstructure:"read_buffer_size"`
	WriteBufferSize   int           `mapstructure:"write_buffer_size"`
}

type PostgresConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdle     time.Duration `mapstructure:"conn_max_idle"`
}

type RedisConfig struct {
	Addrs    []string `mapstructure:"addrs"`
	Password string   `mapstructure:"password"`
	DB       int      `mapstructure:"db"`
	PoolSize int      `mapstructure:"pool_size"`
}

type JWTConfig struct {
	AccessSecret  string        `mapstructure:"access_secret"`
	RefreshSecret string        `mapstructure:"refresh_secret"`
	AccessTTL     time.Duration `mapstructure:"access_ttl"`
	RefreshTTL    time.Duration `mapstructure:"refresh_ttl"`
}

type ServerConfig struct {
	APIHost string `mapstructure:"api_host"`
	APIPort int    `mapstructure:"api_port"`
	WSHost  string `mapstructure:"ws_host"`
	WSPort  int    `mapstructure:"ws_port"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("/etc/h2o")

	viper.AutomaticEnv()

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("app.name", "h2o-gps-platform")
	viper.SetDefault("app.environment", "development")
	viper.SetDefault("app.log_level", "info")
	viper.SetDefault("app.shutdown_timeout", "30s")

	viper.SetDefault("tcp.host", "0.0.0.0")
	viper.SetDefault("tcp.port", 8080)
	viper.SetDefault("tcp.max_connections", 100000)
	viper.SetDefault("tcp.read_timeout", "60s")
	viper.SetDefault("tcp.write_timeout", "10s")
	viper.SetDefault("tcp.idle_timeout", "180s")
	viper.SetDefault("tcp.keepalive", "30s")
	viper.SetDefault("tcp.read_buffer_size", 4096)
	viper.SetDefault("tcp.write_buffer_size", 1024)

	viper.SetDefault("postgres.max_open_conns", 50)
	viper.SetDefault("postgres.max_idle_conns", 10)
	viper.SetDefault("postgres.conn_max_lifetime", "1h")
	viper.SetDefault("postgres.conn_max_idle", "10m")

	viper.SetDefault("redis.addrs", []string{"localhost:6379"})
	viper.SetDefault("redis.pool_size", 20)

	viper.SetDefault("jwt.access_ttl", "15m")
	viper.SetDefault("jwt.refresh_ttl", "168h")

	viper.SetDefault("server.api_host", "0.0.0.0")
	viper.SetDefault("server.api_port", 8081)
	viper.SetDefault("server.ws_host", "0.0.0.0")
	viper.SetDefault("server.ws_port", 8082)
}
