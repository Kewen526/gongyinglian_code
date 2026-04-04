package main

import (
	"fmt"
	"log"
	"time"

	"supply-chain/internal/config"
	"supply-chain/internal/es"
	"supply-chain/internal/handler"
	"supply-chain/internal/repository"
	"supply-chain/internal/router"
	"supply-chain/internal/service"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Load config (use default if config file not found)
	cfg := config.DefaultConfig()
	log.Println("[Config] Loaded configuration")

	// ---------- MySQL ----------
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	// Connection pool settings
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MySQL.ConnMaxLifetimeMinutes) * time.Minute)

	log.Println("[MySQL] Connected with pool settings:",
		"MaxOpen=", cfg.MySQL.MaxOpenConns,
		"MaxIdle=", cfg.MySQL.MaxIdleConns,
		"MaxLifetime=", cfg.MySQL.ConnMaxLifetimeMinutes, "min")

	// ---------- Elasticsearch ----------
	if err := es.InitES(&cfg.Elasticsearch); err != nil {
		log.Printf("[ES] Warning: failed to connect to ES: %v (search features will be unavailable)\n", err)
	}

	// ---------- Repository Layer ----------
	accountRepo := repository.NewAccountRepo(db)
	productRepo := repository.NewProductRepo(db)

	// ---------- Service Layer ----------
	accountService := service.NewAccountService(accountRepo)
	productService := service.NewProductService(productRepo)

	// ---------- Handler Layer ----------
	accountHandler := handler.NewAccountHandler(accountService)
	productHandler := handler.NewProductHandler(productService)

	// ---------- Router ----------
	r := router.SetupRouter(accountHandler, productHandler)

	// ---------- Start Server ----------
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[Server] Starting on %s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
