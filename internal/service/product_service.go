package service

import (
	"context"
	"fmt"
	"log"
	"supply-chain/internal/config"
	"supply-chain/internal/es"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
)

type ProductService struct {
	repo        *repository.ProductRepo
	accountRepo *repository.AccountRepo
}

func NewProductService(repo *repository.ProductRepo, accountRepo *repository.AccountRepo) *ProductService {
	return &ProductService{repo: repo, accountRepo: accountRepo}
}

// ---------- Product ----------

func (s *ProductService) CreateProduct(req *model.CreateProductReq) (*model.Product, error) {
	p := &model.Product{
		ImageURL:     req.ImageURL,
		Name:         req.Name,
		ProductCode:  req.ProductCode,
		Supplier:     req.Supplier,
		Status:       req.Status,
		Brand:        req.Brand,
		Category:     req.Category,
		Tags:         model.StringSlice(req.Tags),
		Material:     req.Material,
		PatentStatus: req.PatentStatus,
		FactoryPrice: req.FactoryPrice,
	}

	specs := make([]model.ProductSpec, 0, len(req.Specs))
	for _, sp := range req.Specs {
		specs = append(specs, model.ProductSpec{
			SizeModel:  sp.SizeModel,
			Dimension:  sp.Dimension,
			Weight:     sp.Weight,
			BoxSpec:    sp.BoxSpec,
			PackingQty: sp.PackingQty,
		})
	}
	prices := make([]model.ProductPlatformPrice, 0, len(req.PlatformPrices))
	for _, pp := range req.PlatformPrices {
		currency := pp.Currency
		if currency == "" {
			currency = "CNY"
		}
		prices = append(prices, model.ProductPlatformPrice{
			PlatformName: pp.PlatformName,
			ControlPrice: pp.ControlPrice,
			Currency:     currency,
		})
	}
	skus := make([]model.ProductSKU, 0, len(req.SKUs))
	for _, sk := range req.SKUs {
		skus = append(skus, model.ProductSKU{
			Model:   sk.Model,
			Size:    sk.Size,
			SKUCode: sk.SKUCode,
		})
	}
	images := make([]model.ProductDetailImage, 0, len(req.DetailImages))
	for i, img := range req.DetailImages {
		sort := img.SortOrder
		if sort == 0 {
			sort = uint(i)
		}
		images = append(images, model.ProductDetailImage{
			ImageURL:  img.ImageURL,
			SortOrder: sort,
		})
	}
	videos := make([]model.ProductVideo, 0, len(req.Videos))
	for _, v := range req.Videos {
		videos = append(videos, model.ProductVideo{
			VideoURL: v.VideoURL,
			CoverURL: v.CoverURL,
		})
	}

	if err := s.repo.CreateWithDetails(p, specs, prices, skus, images, videos); err != nil {
		return nil, err
	}

	// Async sync to ES
	go func() {
		if err := es.IndexProduct(context.Background(), config.GlobalConfig.Elasticsearch.ProductIndex, p); err != nil {
			log.Printf("[ES] Failed to index product %d: %v\n", p.ID, err)
		}
	}()

	return p, nil
}

func (s *ProductService) GetProductDetail(id uint64) (*model.ProductDetailResp, error) {
	p, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	specs, _ := s.repo.GetSpecsByProductID(id)
	prices, _ := s.repo.GetPlatformPricesByProductID(id)
	skus, _ := s.repo.GetSKUsByProductID(id)
	images, _ := s.repo.GetDetailImagesByProductID(id)
	videos, _ := s.repo.GetVideosByProductID(id)

	return &model.ProductDetailResp{
		Product:        *p,
		Specs:          specs,
		PlatformPrices: prices,
		SKUs:           skus,
		DetailImages:   images,
		Videos:         videos,
	}, nil
}

func (s *ProductService) UpdateProduct(id uint64, req *model.UpdateProductReq) error {
	updates := map[string]interface{}{}
	if req.ImageURL != nil {
		updates["image_url"] = *req.ImageURL
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.ProductCode != nil {
		updates["product_code"] = *req.ProductCode
	}
	if req.Supplier != nil {
		updates["supplier"] = *req.Supplier
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Brand != nil {
		updates["brand"] = *req.Brand
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Tags != nil {
		updates["tags"] = model.StringSlice(*req.Tags)
	}
	if req.Material != nil {
		updates["material"] = *req.Material
	}
	if req.PatentStatus != nil {
		updates["patent_status"] = *req.PatentStatus
	}
	if req.FactoryPrice != nil {
		updates["factory_price"] = *req.FactoryPrice
	}

	// Convert sub-resource pointers to model pointers for UpdateWithDetails.
	var specsPtr *[]model.ProductSpec
	if req.Specs != nil {
		specs := make([]model.ProductSpec, 0, len(*req.Specs))
		for _, sp := range *req.Specs {
			specs = append(specs, model.ProductSpec{
				SizeModel:  sp.SizeModel,
				Dimension:  sp.Dimension,
				Weight:     sp.Weight,
				BoxSpec:    sp.BoxSpec,
				PackingQty: sp.PackingQty,
			})
		}
		specsPtr = &specs
	}
	var pricesPtr *[]model.ProductPlatformPrice
	if req.PlatformPrices != nil {
		prices := make([]model.ProductPlatformPrice, 0, len(*req.PlatformPrices))
		for _, pp := range *req.PlatformPrices {
			currency := pp.Currency
			if currency == "" {
				currency = "CNY"
			}
			prices = append(prices, model.ProductPlatformPrice{
				PlatformName: pp.PlatformName,
				ControlPrice: pp.ControlPrice,
				Currency:     currency,
			})
		}
		pricesPtr = &prices
	}
	var skusPtr *[]model.ProductSKU
	if req.SKUs != nil {
		skus := make([]model.ProductSKU, 0, len(*req.SKUs))
		for _, sk := range *req.SKUs {
			skus = append(skus, model.ProductSKU{
				Model:   sk.Model,
				Size:    sk.Size,
				SKUCode: sk.SKUCode,
			})
		}
		skusPtr = &skus
	}
	var imagesPtr *[]model.ProductDetailImage
	if req.DetailImages != nil {
		images := make([]model.ProductDetailImage, 0, len(*req.DetailImages))
		for i, img := range *req.DetailImages {
			sort := img.SortOrder
			if sort == 0 {
				sort = uint(i)
			}
			images = append(images, model.ProductDetailImage{
				ImageURL:  img.ImageURL,
				SortOrder: sort,
			})
		}
		imagesPtr = &images
	}
	var videosPtr *[]model.ProductVideo
	if req.Videos != nil {
		videos := make([]model.ProductVideo, 0, len(*req.Videos))
		for _, v := range *req.Videos {
			videos = append(videos, model.ProductVideo{
				VideoURL: v.VideoURL,
				CoverURL: v.CoverURL,
			})
		}
		videosPtr = &videos
	}

	// Nothing to do?
	if len(updates) == 0 && specsPtr == nil && pricesPtr == nil && skusPtr == nil && imagesPtr == nil && videosPtr == nil {
		return nil
	}

	if err := s.repo.UpdateWithDetails(id, updates, specsPtr, pricesPtr, skusPtr, imagesPtr, videosPtr); err != nil {
		return err
	}

	// Async sync to ES
	go func() {
		p, err := s.repo.GetByID(id)
		if err != nil {
			log.Printf("[ES] Failed to get product %d for sync: %v\n", id, err)
			return
		}
		if err := es.IndexProduct(context.Background(), config.GlobalConfig.Elasticsearch.ProductIndex, p); err != nil {
			log.Printf("[ES] Failed to sync product %d: %v\n", id, err)
		}
	}()

	return nil
}

func (s *ProductService) DeleteProduct(id uint64) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}

	// Sync delete from ES (synchronous for consistency)
	if err := es.DeleteProduct(context.Background(), config.GlobalConfig.Elasticsearch.ProductIndex, id); err != nil {
		log.Printf("[ES] Failed to delete product %d from ES: %v\n", id, err)
	}
	return nil
}

// ListProducts searches products via ES, optionally applying employee scope.
// accountID=0 or role != RoleEmployee means no scope restriction.
func (s *ProductService) ListProducts(req *model.ProductListReq, accountID uint64, role uint8) (*model.ProductListResp, error) {
	// Validate page size
	switch req.PageSize {
	case 20, 50, 100:
	default:
		req.PageSize = 20
	}

	// Inject employee scope if applicable
	if role == model.RoleEmployee && accountID > 0 && s.accountRepo != nil {
		scope, err := s.accountRepo.GetProductScope(accountID)
		if err == nil && scope != nil {
			req.ScopeSuppliers = []string(scope.Suppliers)
			req.ScopeTags = []string(scope.Tags)
		}
	}

	searchResult, err := es.SearchProducts(context.Background(), config.GlobalConfig.Elasticsearch.ProductIndex, req)
	if err != nil {
		return nil, fmt.Errorf("ES search failed: %w", err)
	}

	products, err := s.repo.GetByIDs(searchResult.IDs)
	if err != nil {
		return nil, err
	}

	resp := &model.ProductListResp{
		List:  products,
		Total: searchResult.Total,
	}

	if len(searchResult.SearchAfterSort) == 2 {
		resp.SearchAfterCode = fmt.Sprintf("%v", searchResult.SearchAfterSort[0])
		resp.SearchAfterID = fmt.Sprintf("%v", searchResult.SearchAfterSort[1])
	}

	return resp, nil
}

// GetDistinctSuppliers returns all unique supplier values from the product table.
func (s *ProductService) GetDistinctSuppliers() ([]string, error) {
	return s.repo.GetDistinctSuppliers()
}

// ---------- Spec ----------

func (s *ProductService) CreateSpec(productID uint64, req *model.CreateSpecReq) (*model.ProductSpec, error) {
	spec := &model.ProductSpec{
		ProductID:  productID,
		SizeModel:  req.SizeModel,
		Dimension:  req.Dimension,
		Weight:     req.Weight,
		BoxSpec:    req.BoxSpec,
		PackingQty: req.PackingQty,
	}
	if err := s.repo.CreateSpec(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func (s *ProductService) UpdateSpec(id uint64, req *model.UpdateSpecReq) error {
	updates := map[string]interface{}{}
	if req.SizeModel != nil {
		updates["size_model"] = *req.SizeModel
	}
	if req.Dimension != nil {
		updates["dimension"] = *req.Dimension
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.BoxSpec != nil {
		updates["box_spec"] = *req.BoxSpec
	}
	if req.PackingQty != nil {
		updates["packing_qty"] = *req.PackingQty
	}
	if len(updates) == 0 {
		return nil
	}
	return s.repo.UpdateSpec(id, updates)
}

func (s *ProductService) DeleteSpec(id uint64) error {
	return s.repo.DeleteSpec(id)
}

// ---------- Platform Price ----------

func (s *ProductService) CreatePlatformPrice(productID uint64, req *model.CreatePlatformPriceReq) (*model.ProductPlatformPrice, error) {
	price := &model.ProductPlatformPrice{
		ProductID:    productID,
		PlatformName: req.PlatformName,
		ControlPrice: req.ControlPrice,
		Currency:     req.Currency,
	}
	if err := s.repo.CreatePlatformPrice(price); err != nil {
		return nil, err
	}
	return price, nil
}

func (s *ProductService) UpdatePlatformPrice(id uint64, req *model.UpdatePlatformPriceReq) error {
	updates := map[string]interface{}{}
	if req.PlatformName != nil {
		updates["platform_name"] = *req.PlatformName
	}
	if req.ControlPrice != nil {
		updates["control_price"] = *req.ControlPrice
	}
	if req.Currency != nil {
		updates["currency"] = *req.Currency
	}
	if len(updates) == 0 {
		return nil
	}
	return s.repo.UpdatePlatformPrice(id, updates)
}

func (s *ProductService) DeletePlatformPrice(id uint64) error {
	return s.repo.DeletePlatformPrice(id)
}

// ---------- SKU ----------

func (s *ProductService) CreateSKU(productID uint64, req *model.CreateSKUReq) (*model.ProductSKU, error) {
	sku := &model.ProductSKU{
		ProductID: productID,
		Model:     req.Model,
		Size:      req.Size,
		SKUCode:   req.SKUCode,
	}
	if err := s.repo.CreateSKU(sku); err != nil {
		return nil, err
	}
	return sku, nil
}

func (s *ProductService) UpdateSKU(id uint64, req *model.UpdateSKUReq) error {
	updates := map[string]interface{}{}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.Size != nil {
		updates["size"] = *req.Size
	}
	if req.SKUCode != nil {
		updates["sku_code"] = *req.SKUCode
	}
	if len(updates) == 0 {
		return nil
	}
	return s.repo.UpdateSKU(id, updates)
}

func (s *ProductService) DeleteSKU(id uint64) error {
	return s.repo.DeleteSKU(id)
}

// ---------- Detail Images ----------

func (s *ProductService) BatchCreateDetailImages(productID uint64, req *model.BatchCreateDetailImageReq) ([]model.ProductDetailImage, error) {
	images := make([]model.ProductDetailImage, 0, len(req.Images))
	for _, img := range req.Images {
		images = append(images, model.ProductDetailImage{
			ProductID: productID,
			ImageURL:  img.ImageURL,
			SortOrder: img.SortOrder,
		})
	}
	if err := s.repo.BatchCreateDetailImages(images); err != nil {
		return nil, err
	}
	return images, nil
}

func (s *ProductService) DeleteDetailImage(id uint64) error {
	return s.repo.DeleteDetailImage(id)
}

// ---------- Videos ----------

func (s *ProductService) BatchCreateVideos(productID uint64, req *model.BatchCreateVideoReq) ([]model.ProductVideo, error) {
	videos := make([]model.ProductVideo, 0, len(req.Videos))
	for _, v := range req.Videos {
		videos = append(videos, model.ProductVideo{
			ProductID: productID,
			VideoURL:  v.VideoURL,
			CoverURL:  v.CoverURL,
		})
	}
	if err := s.repo.BatchCreateVideos(videos); err != nil {
		return nil, err
	}
	return videos, nil
}

func (s *ProductService) DeleteVideo(id uint64) error {
	return s.repo.DeleteVideo(id)
}

// ---------- Full ES Reindex ----------

func (s *ProductService) FullReindex() error {
	index := config.GlobalConfig.Elasticsearch.ProductIndex
	ctx := context.Background()

	// Delete and recreate index
	_ = es.DeleteIndex(ctx, index)
	if err := es.CreateProductIndex(ctx, index); err != nil {
		return fmt.Errorf("failed to create ES index: %w", err)
	}

	batchSize := 1000
	var lastID uint64 = 0
	totalIndexed := 0

	for {
		products, err := s.repo.BatchGetForReindex(lastID, batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch products from DB: %w", err)
		}
		if len(products) == 0 {
			break
		}

		if err := es.BulkIndex(ctx, index, products); err != nil {
			return fmt.Errorf("failed to bulk index to ES: %w", err)
		}

		lastID = products[len(products)-1].ID
		totalIndexed += len(products)
		log.Printf("[Reindex] Indexed %d products so far (lastID=%d)\n", totalIndexed, lastID)
	}

	log.Printf("[Reindex] Complete. Total indexed: %d\n", totalIndexed)
	return nil
}
