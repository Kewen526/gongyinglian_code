package main

import (
	"fmt"
	"log"
	"time"

	"supply-chain/internal/config"
	"supply-chain/internal/es"
	"supply-chain/internal/handler"
	"supply-chain/internal/model"
	"supply-chain/internal/oss"
	"supply-chain/internal/repository"
	"supply-chain/internal/router"
	"supply-chain/internal/service"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Load config: try config.json first, fallback to defaults
	cfg, err := config.LoadConfig("configs/config.json")
	if err != nil {
		log.Printf("[Config] config.json not found, using defaults: %v\n", err)
		cfg = config.DefaultConfig()
	}
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

	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MySQL.ConnMaxLifetimeMinutes) * time.Minute)

	log.Println("[MySQL] Connected with pool settings:",
		"MaxOpen=", cfg.MySQL.MaxOpenConns,
		"MaxIdle=", cfg.MySQL.MaxIdleConns,
		"MaxLifetime=", cfg.MySQL.ConnMaxLifetimeMinutes, "min")

	// ---------- Auto-migrate tables ----------
	if err := db.AutoMigrate(
		&model.Account{},
		&model.OrderTrade{},
		&model.OrderItem{},
		&model.Shop{},
		&model.AccountShop{},
		&model.SyncState{},
	); err != nil {
		log.Fatalf("Failed to auto-migrate tables: %v", err)
	}
	log.Println("[MySQL] Tables migrated")

	// ---------- Auto-create super admin ----------
	initSuperAdmin(db)

	// ---------- Tencent Cloud COS ----------
	oss.InitCOS(&cfg.COS)
	log.Println("[COS] Initialized")

	// ---------- Elasticsearch ----------
	if err := es.InitES(&cfg.Elasticsearch); err != nil {
		log.Printf("[ES] Warning: failed to connect to ES: %v (search features will be unavailable)\n", err)
	}

	// ---------- Repository Layer ----------
	accountRepo := repository.NewAccountRepo(db)
	productRepo := repository.NewProductRepo(db)
	orderRepo := repository.NewOrderRepo(db)
	shopRepo := repository.NewShopRepo(db)

	// ---------- Service Layer ----------
	accountService := service.NewAccountService(accountRepo, shopRepo)
	productService := service.NewProductService(productRepo)
	orderService := service.NewOrderService(orderRepo, shopRepo, accountRepo)
	syncService := service.NewSyncService(orderRepo, shopRepo, &cfg.WanLiNiu)

	// ---------- Start auto sync ----------
	syncService.StartAutoSync()
	defer syncService.Stop()
	log.Println("[Sync] Order sync service started")

	// ---------- Handler Layer ----------
	accountHandler := handler.NewAccountHandler(accountService)
	productHandler := handler.NewProductHandler(productService)
	uploadHandler := handler.NewUploadHandler()
	orderHandler := handler.NewOrderHandler(orderService, syncService)

	// ---------- Router ----------
	r := router.SetupRouter(accountHandler, productHandler, uploadHandler, orderHandler, accountRepo)

	// ---------- Start Server ----------
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[Server] Starting on %s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// initSuperAdmin creates the default super admin account if it doesn't exist.
// Default credentials: admin / admin123
func initSuperAdmin(db *gorm.DB) {
	var count int64
	db.Model(&model.Account{}).Where("role = ?", model.RoleSuperAdmin).Count(&count)
	if count > 0 {
		log.Println("[Init] Super admin already exists, skipping")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash super admin password: %v", err)
	}

	admin := model.Account{
		Username: "admin",
		Password: string(hashed),
		RealName: "超级管理员",
		Role:     model.RoleSuperAdmin,
	}

	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("Failed to create super admin: %v", err)
	}
	log.Println("[Init] Super admin created: username=admin, password=admin123")
}
