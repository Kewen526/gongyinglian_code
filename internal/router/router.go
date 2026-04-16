package router

import (
	"supply-chain/internal/handler"
	"supply-chain/internal/middleware"
	"supply-chain/internal/repository"

	"github.com/gin-gonic/gin"
)

func SetupRouter(
	accountHandler *handler.AccountHandler,
	productHandler *handler.ProductHandler,
	uploadHandler *handler.UploadHandler,
	orderHandler *handler.OrderHandler,
	billingHandler *handler.BillingHandler,
	adminBillingHandler *handler.AdminBillingHandler,
	warehouseHandler *handler.WarehouseHandler,
	adminWarehouseHandler *handler.AdminWarehouseHandler,
	accountRepo *repository.AccountRepo,
) *gin.Engine {
	r := gin.Default()

	// Set max multipart memory to 200MB for file uploads
	r.MaxMultipartMemory = 200 << 20

	api := r.Group("/api/v1")

	// ========== Public (no auth) ==========
	api.POST("/login", accountHandler.Login)

	// ========== Authenticated routes ==========
	auth := api.Group("")
	auth.Use(middleware.JWTAuth())

	// --- Upload (any logged-in user) ---
	auth.POST("/upload/image", uploadHandler.UploadImage)
	auth.POST("/upload/video", uploadHandler.UploadVideo)
	auth.POST("/upload/file", uploadHandler.UploadFile)

	// --- Account management (super admin + team lead + supervisor) ---
	accountMgmt := auth.Group("")
	accountMgmt.Use(middleware.RequireAccountManager())
	{
		accountMgmt.GET("/accounts", accountHandler.ListAccounts)
		accountMgmt.POST("/accounts", accountHandler.CreateAccount)
		accountMgmt.GET("/accounts/:id", accountHandler.GetAccountDetail)
		accountMgmt.PUT("/accounts/:id", accountHandler.UpdateAccount)
		accountMgmt.DELETE("/accounts/:id", accountHandler.DeleteAccount)
		accountMgmt.PUT("/accounts/:id/permissions", accountHandler.UpdatePermissions)
		accountMgmt.GET("/accounts/:id/product-scope", accountHandler.GetProductScope)
		accountMgmt.PUT("/accounts/:id/product-scope", accountHandler.SaveProductScope)

		// Shop permissions for accounts
		accountMgmt.GET("/accounts/:id/shops", orderHandler.GetAccountShops)
		accountMgmt.PUT("/accounts/:id/shops", orderHandler.UpdateAccountShops)
	}

	// --- Modules (any logged-in user) ---
	auth.GET("/modules", accountHandler.GetAllModules)

	// --- Product: view permission ---
	productView := auth.Group("")
	productView.Use(middleware.RequireModulePermission(accountRepo, "product", false))
	{
		productView.GET("/products", productHandler.ListProducts)
		productView.GET("/products/:id", productHandler.GetProductDetail)
		productView.GET("/products/suppliers", productHandler.GetSuppliers)
		productView.GET("/products/field-options", productHandler.GetFieldOptions)
	}

	// --- Product: edit permission ---
	productEdit := auth.Group("")
	productEdit.Use(middleware.RequireModulePermission(accountRepo, "product", true))
	{
		productEdit.POST("/products", productHandler.CreateProduct)
		productEdit.PUT("/products/:id", productHandler.UpdateProduct)
		productEdit.DELETE("/products/:id", productHandler.DeleteProduct)

		// Spec
		productEdit.POST("/products/:id/specs", productHandler.CreateSpec)
		productEdit.PUT("/products/:id/specs/:specId", productHandler.UpdateSpec)
		productEdit.DELETE("/products/:id/specs/:specId", productHandler.DeleteSpec)

		// Platform Price
		productEdit.POST("/products/:id/platform-prices", productHandler.CreatePlatformPrice)
		productEdit.PUT("/products/:id/platform-prices/:priceId", productHandler.UpdatePlatformPrice)
		productEdit.DELETE("/products/:id/platform-prices/:priceId", productHandler.DeletePlatformPrice)

		// SKU
		productEdit.POST("/products/:id/skus", productHandler.CreateSKU)
		productEdit.PUT("/products/:id/skus/:skuId", productHandler.UpdateSKU)
		productEdit.DELETE("/products/:id/skus/:skuId", productHandler.DeleteSKU)

		// Detail Images
		productEdit.POST("/products/:id/detail-images", productHandler.BatchCreateDetailImages)
		productEdit.DELETE("/products/:id/detail-images/:imageId", productHandler.DeleteDetailImage)

		// Videos
		productEdit.POST("/products/:id/videos", productHandler.BatchCreateVideos)
		productEdit.DELETE("/products/:id/videos/:videoId", productHandler.DeleteVideo)

		// ES Full Reindex (edit permission required)
		productEdit.POST("/products/reindex", productHandler.FullReindex)
	}

	// --- Order: view permission ---
	orderView := auth.Group("")
	orderView.Use(middleware.RequireModulePermission(accountRepo, "order", false))
	{
		orderView.GET("/orders", orderHandler.ListOrders)
		orderView.GET("/orders/:id", orderHandler.GetOrderDetail)
		orderView.GET("/orders/status-options", orderHandler.GetStatusOptions)
		orderView.GET("/orders/auto-review", accountHandler.GetAutoReviewStatus)
	}

	// --- Order: edit permission (sync, batch update, mark) ---
	orderEdit := auth.Group("")
	orderEdit.Use(middleware.RequireModulePermission(accountRepo, "order", true))
	{
		orderEdit.POST("/orders/sync", orderHandler.SyncOrders)
		orderEdit.PATCH("/orders/batch-update", orderHandler.BatchUpdateOrders)
		orderEdit.POST("/orders/mark", orderHandler.BatchMarkOrders)
		orderEdit.PUT("/orders/auto-review", accountHandler.SetAutoReviewStatus)
	}

	// --- Shop/Platform queries (any logged-in user with order view) ---
	orderView.GET("/shops", orderHandler.ListShops)
	orderView.GET("/shops/grouped", orderHandler.ListShopsGrouped)
	orderView.GET("/shops/occupied", orderHandler.GetOccupiedShopIDs)
	orderView.GET("/platforms", orderHandler.ListPlatforms)

	// --- Billing (employees only, billing module permission) ---
	billingGroup := auth.Group("")
	billingGroup.Use(middleware.RequireModulePermission(accountRepo, "billing", false))
	{
		billingGroup.GET("/billing/wallet", billingHandler.GetWallet)
		billingGroup.GET("/billing", billingHandler.ListBillingRecords)
		billingGroup.POST("/billing/recharge", billingHandler.SubmitRecharge)
		billingGroup.GET("/billing/recharge-records", billingHandler.ListMyRechargeRecords)
	}

	// --- Warehouse billing (employees, warehouse module permission) ---
	warehouseGroup := auth.Group("")
	warehouseGroup.Use(middleware.RequireModulePermission(accountRepo, "warehouse", false))
	{
		warehouseGroup.GET("/warehouse/wallet", warehouseHandler.GetWallet)
		warehouseGroup.GET("/warehouse/billing", warehouseHandler.ListBillingRecords)
		warehouseGroup.POST("/warehouse/recharge", warehouseHandler.SubmitRecharge)
		warehouseGroup.GET("/warehouse/recharge-records", warehouseHandler.ListMyRechargeRecords)
	}

	// --- Admin Finance Center (super admin only) ---
	adminFinance := auth.Group("/admin/finance")
	adminFinance.Use(middleware.RequireSuperAdmin())
	{
		adminFinance.GET("/overview", adminBillingHandler.GetFinanceOverview)
		adminFinance.GET("/recharge-requests", adminBillingHandler.ListRechargeRequests)
		adminFinance.POST("/recharge-requests/:id/approve", adminBillingHandler.ApproveRecharge)
		adminFinance.POST("/recharge-requests/:id/reject", adminBillingHandler.RejectRecharge)
		adminFinance.GET("/billing-records", adminBillingHandler.ListAllBillingRecords)
		adminFinance.GET("/billing-records/export", adminBillingHandler.ExportBillingRecords)
	}

	// --- Admin Warehouse Finance (super admin only) ---
	adminWarehouse := auth.Group("/admin/warehouse")
	adminWarehouse.Use(middleware.RequireSuperAdmin())
	{
		adminWarehouse.GET("/overview", adminWarehouseHandler.GetOverview)
		adminWarehouse.GET("/recharge-requests", adminWarehouseHandler.ListRechargeRequests)
		adminWarehouse.POST("/recharge-requests/:id/approve", adminWarehouseHandler.ApproveRecharge)
		adminWarehouse.POST("/recharge-requests/:id/reject", adminWarehouseHandler.RejectRecharge)
		adminWarehouse.GET("/billing-records", adminWarehouseHandler.ListBillingRecords)
	}

	return r
}
