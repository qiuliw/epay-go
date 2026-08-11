// internal/config/config.go
package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Worker   WorkerConfig   `mapstructure:"worker"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release, test
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireHour int    `mapstructure:"expire_hour"`
}

// WorkerConfig 异步通知 / 主动查单的固定并发与扫库间隔
type WorkerConfig struct {
	NotifyConcurrency     int `mapstructure:"notify_concurrency"`
	NotifyPollIntervalSec int `mapstructure:"notify_poll_interval_sec"`
	QueryConcurrency      int `mapstructure:"query_concurrency"`
	QueryPollIntervalSec  int `mapstructure:"query_poll_interval_sec"`
}

var Cfg *Config

func Load(path string) error {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// 环境变量覆盖
	viper.AutomaticEnv()

	// 绑定 Docker 环境变量
	viper.BindEnv("database.host", "DB_HOST")
	viper.BindEnv("database.port", "DB_PORT")
	viper.BindEnv("database.user", "DB_USER")
	viper.BindEnv("database.password", "DB_PASSWORD")
	viper.BindEnv("database.dbname", "DB_NAME")
	viper.BindEnv("redis.host", "REDIS_HOST")
	viper.BindEnv("redis.port", "REDIS_PORT")
	viper.BindEnv("redis.password", "REDIS_PASSWORD")
	viper.BindEnv("jwt.secret", "JWT_SECRET")
	viper.BindEnv("server.mode", "GIN_MODE")

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	Cfg = &Config{}
	if err := viper.Unmarshal(Cfg); err != nil {
		return err
	}
	Cfg.applyWorkerDefaults()

	return nil
}

func (c *Config) applyWorkerDefaults() {
	if c.Worker.NotifyConcurrency <= 0 {
		c.Worker.NotifyConcurrency = 16
	}
	if c.Worker.NotifyPollIntervalSec <= 0 {
		c.Worker.NotifyPollIntervalSec = 2
	}
	if c.Worker.QueryConcurrency <= 0 {
		c.Worker.QueryConcurrency = 16
	}
	if c.Worker.QueryPollIntervalSec <= 0 {
		c.Worker.QueryPollIntervalSec = 2
	}
}

func Get() *Config {
	return Cfg
}
