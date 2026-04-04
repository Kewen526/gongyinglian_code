package router

import (
	"supply-chain/internal/handler"
	"supply-chain/internal/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter(accountHandler *handler.AccountHandler, productHandler *handler.ProductHandler) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.Recovery())

	api := r.Group("/api/v1")

	// ---------- Account / Permission ----------
	api.POST("/accounts", accountHandler.CreateAccount)
	api.GET("/modules", accountHandler.GetAllModules)
	api.GET("/accounts/:id", accountHandler.GetAccountDetail)
	api.PUT("/accounts/:id/permissions", accountHandler.UpdatePermissions)

	// ---------- Product ----------
	api.GET("/products", productHandler.ListProducts)
	api.POST("/products", productHandler.CreateProduct)
	api.GET("/products/:id", productHandler.GetProductDetail)
	api.PUT("/products/:id", productHandler.UpdateProduct)
	api.DELETE("/products/:id", productHandler.DeleteProduct)

	// Spec
	api.POST("/products/:id/specs", productHandler.CreateSpec)
	api.PUT("/products/:id/specs/:specId", productHandler.UpdateSpec)
	api.DELETE("/products/:id/specs/:specId", productHandler.DeleteSpec)

	// Platform Price
	api.POST("/products/:id/platform-prices", productHandler.CreatePlatformPrice)
	api.PUT("/products/:id/platform-prices/:priceId", productHandler.UpdatePlatformPrice)
	api.DELETE("/products/:id/platform-prices/:priceId", productHandler.DeletePlatformPrice)

	// SKU
	api.POST("/products/:id/skus", productHandler.CreateSKU)
	api.PUT("/products/:id/skus/:skuId", productHandler.UpdateSKU)
	api.DELETE("/products/:id/skus/:skuId", productHandler.DeleteSKU)

	// Detail Images
	api.POST("/products/:id/detail-images", productHandler.BatchCreateDetailImages)
	api.DELETE("/products/:id/detail-images/:imageId", productHandler.DeleteDetailImage)

	// Videos
	api.POST("/products/:id/videos", productHandler.BatchCreateVideos)
	api.DELETE("/products/:id/videos/:videoId", productHandler.DeleteVideo)

	// ES Full Reindex
	api.POST("/products/reindex", productHandler.FullReindex)

	return r
}
