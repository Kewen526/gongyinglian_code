package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Server        ServerConfig        `json:"server"`
	MySQL         MySQLConfig         `json:"mysql"`
	Elasticsearch ElasticsearchConfig `json:"elasticsearch"`
	OSS           OSSConfig           `json:"oss"`
	JWT           JWTConfig           `json:"jwt"`
	WanLiNiu      WanLiNiuConfig      `json:"wanliniu"`
	Security      SecurityConfig      `json:"security"`
}

type SecurityConfig struct {
	AllowedOrigins []string `json:"allowed_origins"`
	AppToken       string   `json:"app_token"`
	TokenInterval  int      `json:"token_interval_seconds"`
	RateLimit      int      `json:"rate_limit_per_second"`
}

type WanLiNiuConfig struct {
	AppKey       string `json:"app_key"`
	Secret       string `json:"secret"`
	BaseURL      string `json:"base_url"`
	SyncInterval int    `json:"sync_interval_seconds"` // sync interval in seconds, default 60
}

type JWTConfig struct {
	Secret     string `json:"secret"`
	ExpireHour int    `json:"expire_hour"`
}

type ServerConfig struct {
	Port int `json:"port"`
}

type MySQLConfig struct {
	DSN                    string `json:"dsn"`
	MaxOpenConns           int    `json:"max_open_conns"`
	MaxIdleConns           int    `json:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `json:"conn_max_lifetime_minutes"`
}

type ElasticsearchConfig struct {
	Addresses    []string `json:"addresses"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	ProductIndex string   `json:"product_index"`
}

type OSSConfig struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
}

var GlobalConfig *Config

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	GlobalConfig = &cfg
	return &cfg, nil
}

// DefaultConfig returns a config with sensible defaults for development.
func DefaultConfig() *Config {
	cfg := &Config{
		Server: ServerConfig{Port: 8080},
		MySQL: MySQLConfig{
			DSN:                    "root:password@tcp(127.0.0.1:3306)/supply_chain?charset=utf8mb4&parseTime=True&loc=Local",
			MaxOpenConns:           100,
			MaxIdleConns:           20,
			ConnMaxLifetimeMinutes: 30,
		},
		Elasticsearch: ElasticsearchConfig{
			Addresses:    []string{"http://127.0.0.1:9200"},
			ProductIndex: "products",
		},
		OSS: OSSConfig{
			AccessKeyID:     "",
			AccessKeySecret: "",
			Endpoint:        "oss-cn-beijing.aliyuncs.com",
			Bucket:          "fasvio",
		},
		JWT: JWTConfig{
			Secret:     "",
			ExpireHour: 24,
		},
		WanLiNiu: WanLiNiuConfig{
			AppKey:       "",
			Secret:       "",
			BaseURL:      "https://open-api.hupun.com/api",
			SyncInterval: 60,
		},
		Security: SecurityConfig{
			AllowedOrigins: []string{"*"},
			AppToken:       "",
			TokenInterval:  300,
			RateLimit:      20,
		},
	}
	GlobalConfig = cfg
	return cfg
}
