package main

import (
	"fmt"
	"log"
	"os"
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
		&model.Module{},
		&model.AccountPermission{},
		&model.Wallet{},
		&model.RechargeRequest{},
		&model.BillingRecord{},
		&model.Product{},
		&model.ProductSpec{},
		&model.ProductPlatformPrice{},
		&model.ProductSKU{},
		&model.ProductDetailImage{},
		&model.ProductVideo{},
		&model.AccountProductScope{},
		&model.WarehouseWallet{},
		&model.WarehouseRechargeRequest{},
		&model.WarehouseBillingRecord{},
		&model.WarehouseFlowCounter{},
		&model.WlnGoodsSpecCache{},
		&model.TeamLeaderPaymentInfo{},
	); err != nil {
		log.Fatalf("Failed to auto-migrate tables: %v", err)
	}
	log.Println("[MySQL] Tables migrated")

	// Migrate group_name → tags for existing products
	db.Exec("UPDATE product SET tags = JSON_ARRAY(group_name) WHERE group_name IS NOT NULL AND group_name != '' AND (tags IS NULL OR JSON_LENGTH(tags) = 0)")

	// ---------- Auto-create super admin & seed modules ----------
	initSuperAdmin(db)
	initModules(db)

	// ---------- Alibaba Cloud OSS ----------
	oss.InitOSS(&cfg.OSS)
	log.Println("[OSS] Initialized")

	// ---------- Elasticsearch ----------
	if err := es.InitES(&cfg.Elasticsearch); err != nil {
		log.Printf("[ES] Warning: failed to connect to ES: %v (search features will be unavailable)\n", err)
	}

	// ---------- Repository Layer ----------
	accountRepo := repository.NewAccountRepo(db)
	productRepo := repository.NewProductRepo(db)
	orderRepo := repository.NewOrderRepo(db)
	shopRepo := repository.NewShopRepo(db)
	billingRepo := repository.NewBillingRepo(db)
	wlnGoodsRepo := repository.NewWlnGoodsRepo(db)

	// ---------- Service Layer ----------
	accountService := service.NewAccountService(accountRepo, shopRepo)
	productService := service.NewProductService(productRepo, accountRepo)
	billingService := service.NewBillingService(billingRepo, orderRepo, productRepo)
	orderService := service.NewOrderService(orderRepo, shopRepo, accountRepo, billingService)
	syncService := service.NewSyncService(orderRepo, shopRepo, accountRepo, &cfg.WanLiNiu, billingService, wlnGoodsRepo)

	// Wire the ERP mark-push hook so BillingService can restore "已审核" on
	// WanLiNiu after an insufficient-balance order recovers via recharge.
	// SmartMarkPush routes to the correct API (domestic batch-mark vs foreign remark)
	// based on each order's trade_source.
	billingService.SetMarkPusher(syncService.SmartMarkPush)

	// ---------- Start scheduled tasks ----------
	syncService.StartAutoSync()
	syncService.StartAfterSaleSync()
	syncService.StartAutoReview()
	syncService.StartForeignSync()
	syncService.StartForeignAfterSaleSync()
	syncService.StartImageRefresh()
	defer syncService.Stop()
	log.Println("[Sync] Order sync service started")
	log.Println("[Sync] After-sale sync service started")
	log.Println("[Sync] Auto-review task started")
	log.Println("[Sync] Foreign order sync service started")
	log.Println("[Sync] Foreign after-sale sync service started")
	log.Println("[GoodsCache] Image refresh service started")

	if os.Getenv("DISABLE_AUTO_DEDUCT") != "true" {
		billingService.StartAutoDeduct()
	} else {
		log.Println("[Billing] Auto-deduct DISABLED via DISABLE_AUTO_DEDUCT=true")
	}
	billingService.StartAutoRefund()
	billingService.StartMonthlyDiscountRefresh()
	defer billingService.Stop()
	log.Println("[Billing] Billing service started")

	warehouseRepo := repository.NewWarehouseRepo(db)
	warehouseService := service.NewWarehouseService(warehouseRepo)
	if os.Getenv("DISABLE_AUTO_DEDUCT") != "true" {
		warehouseService.StartAutoDeduct()
	} else {
		log.Println("[Warehouse] Auto-deduct DISABLED via DISABLE_AUTO_DEDUCT=true")
	}
	defer warehouseService.Stop()
	log.Println("[Warehouse] Warehouse billing service started")

	// ---------- Handler Layer ----------
	accountHandler := handler.NewAccountHandler(accountService)
	productHandler := handler.NewProductHandler(productService)
	uploadHandler := handler.NewUploadHandler()
	orderHandler := handler.NewOrderHandler(orderService, syncService)
	billingHandler := handler.NewBillingHandler(billingService)
	adminBillingHandler := handler.NewAdminBillingHandler(billingService)
	warehouseHandler := handler.NewWarehouseHandler(warehouseService)
	adminWarehouseHandler := handler.NewAdminWarehouseHandler(warehouseService)

	// ---------- Router ----------
	r := router.SetupRouter(accountHandler, productHandler, uploadHandler, orderHandler, billingHandler, adminBillingHandler, warehouseHandler, adminWarehouseHandler, accountRepo)

	// ---------- Start Server ----------
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("[Server] Starting on %s\n", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// initModules ensures required modules exist in the database.
func initModules(db *gorm.DB) {
	modules := []model.Module{
		{Name: "产品管理", Code: "product"},
		{Name: "订单管理", Code: "order"},
		{Name: "财务流水", Code: "billing"},
		{Name: "仓储发货", Code: "warehouse"},
	}
	for _, m := range modules {
		var count int64
		db.Model(&model.Module{}).Where("code = ?", m.Code).Count(&count)
		if count == 0 {
			if err := db.Create(&m).Error; err != nil {
				log.Printf("[Init] Failed to create module %s: %v\n", m.Code, err)
			} else {
				log.Printf("[Init] Module created: %s\n", m.Code)
			}
		}
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
	log.Println("[Init] Super admin created: username=admin")
}
