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

	// --- Account management (super admin only) ---
	adminOnly := auth.Group("")
	adminOnly.Use(middleware.RequireSuperAdmin())
	{
		adminOnly.GET("/accounts", accountHandler.ListAccounts)
		adminOnly.POST("/accounts", accountHandler.CreateAccount)
		adminOnly.GET("/accounts/:id", accountHandler.GetAccountDetail)
		adminOnly.PUT("/accounts/:id", accountHandler.UpdateAccount)
		adminOnly.DELETE("/accounts/:id", accountHandler.DeleteAccount)
		adminOnly.PUT("/accounts/:id/permissions", accountHandler.UpdatePermissions)

		// Shop permissions for accounts (super admin only)
		adminOnly.GET("/accounts/:id/shops", orderHandler.GetAccountShops)
		adminOnly.PUT("/accounts/:id/shops", orderHandler.UpdateAccountShops)
	}

	// --- Modules (any logged-in user) ---
	auth.GET("/modules", accountHandler.GetAllModules)

	// --- Product: view permission ---
	productView := auth.Group("")
	productView.Use(middleware.RequireModulePermission(accountRepo, "product", false))
	{
		productView.GET("/products", productHandler.ListProducts)
		productView.GET("/products/:id", productHandler.GetProductDetail)
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
	}

	// --- Order: edit permission (sync, batch update, mark) ---
	orderEdit := auth.Group("")
	orderEdit.Use(middleware.RequireModulePermission(accountRepo, "order", true))
	{
		orderEdit.POST("/orders/sync", orderHandler.SyncOrders)
		orderEdit.PATCH("/orders/batch-update", orderHandler.BatchUpdateOrders)
		orderEdit.POST("/orders/mark", orderHandler.BatchMarkOrders)
	}

	// --- Shop/Platform queries (any logged-in user with order view) ---
	orderView.GET("/shops", orderHandler.ListShops)
	orderView.GET("/shops/grouped", orderHandler.ListShopsGrouped)
	orderView.GET("/shops/occupied", orderHandler.GetOccupiedShopIDs)
	orderView.GET("/platforms", orderHandler.ListPlatforms)

	return r
}
