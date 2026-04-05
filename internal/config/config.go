package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Server        ServerConfig        `json:"server"`
	MySQL         MySQLConfig         `json:"mysql"`
	Elasticsearch ElasticsearchConfig `json:"elasticsearch"`
	COS           COSConfig           `json:"cos"`
	JWT           JWTConfig           `json:"jwt"`
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

type COSConfig struct {
	SecretID  string `json:"secret_id"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	BaseURL   string `json:"base_url"`
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
		COS: COSConfig{
			SecretID:  "",
			SecretKey: "",
			Region:    "ap-beijing",
			Bucket:    "",
			BaseURL:   "",
		},
		JWT: JWTConfig{
			Secret:     "supply-chain-jwt-secret-key-change-in-production",
			ExpireHour: 24,
		},
	}
	GlobalConfig = cfg
	return cfg
}
