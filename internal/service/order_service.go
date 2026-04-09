package service

import (
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
)

type OrderService struct {
	orderRepo *repository.OrderRepo
	shopRepo  *repository.ShopRepo
}

func NewOrderService(orderRepo *repository.OrderRepo, shopRepo *repository.ShopRepo) *OrderService {
	return &OrderService{orderRepo: orderRepo, shopRepo: shopRepo}
}

// ListOrders returns paginated orders with permission filtering.
// If role is super admin, shopIDs filter is nil (see all).
func (s *OrderService) ListOrders(req *model.OrderListReq, accountID uint64, role uint8) (*model.OrderListResp, error) {
	var shopIDs []uint64

	// Non-admin: filter by authorized shops
	if role != model.RoleSuperAdmin {
		ids, err := s.shopRepo.GetAccountShopIDs(accountID)
		if err != nil {
			return nil, err
		}
		shopIDs = ids // may be empty — will return 0 results
	}

	trades, total, err := s.orderRepo.ListTrades(req, shopIDs)
	if err != nil {
		return nil, err
	}

	// Batch load items for all trades
	if len(trades) > 0 {
		uids := make([]string, 0, len(trades))
		for _, t := range trades {
			uids = append(uids, t.UID)
		}
		itemsMap, err := s.orderRepo.BatchGetItemsByTradeUIDs(uids)
		if err != nil {
			return nil, err
		}
		for i := range trades {
			trades[i].Items = itemsMap[trades[i].UID]
		}
	}

	return &model.OrderListResp{
		Total: total,
		List:  trades,
	}, nil
}

// GetOrderDetail returns a single order with all items.
func (s *OrderService) GetOrderDetail(id uint64, accountID uint64, role uint8) (*model.OrderTrade, error) {
	trade, err := s.orderRepo.GetTradeByID(id)
	if err != nil {
		return nil, err
	}

	// Permission check for non-admin
	if role != model.RoleSuperAdmin {
		shopIDs, err := s.shopRepo.GetAccountShopIDs(accountID)
		if err != nil {
			return nil, err
		}
		// Check that the trade's shop is in the user's authorized shops
		if !s.isShopAuthorized(trade.SysShop, shopIDs) {
			return nil, nil // return nil to indicate not found / no access
		}
	}

	items, err := s.orderRepo.GetItemsByTradeUID(trade.UID)
	if err != nil {
		return nil, err
	}
	trade.Items = items

	return trade, nil
}

func (s *OrderService) isShopAuthorized(sysShop string, shopIDs []uint64) bool {
	if len(shopIDs) == 0 {
		return false
	}
	shops, err := s.shopRepo.GetByIDs(shopIDs)
	if err != nil {
		return false
	}
	for _, shop := range shops {
		if shop.SysShop == sysShop {
			return true
		}
	}
	return false
}

// ListShops returns all shops, optionally filtered by platform.
func (s *OrderService) ListShops(platform string) ([]model.Shop, error) {
	if platform != "" {
		return s.shopRepo.ListByPlatform(platform)
	}
	return s.shopRepo.ListAll()
}

// ListShopsByPlatform returns shops grouped by platform.
func (s *OrderService) ListShopsGrouped() ([]model.ShopsByPlatformResp, error) {
	shops, err := s.shopRepo.ListAll()
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]model.Shop)
	var platformOrder []string
	for _, shop := range shops {
		if _, exists := grouped[shop.SourcePlatform]; !exists {
			platformOrder = append(platformOrder, shop.SourcePlatform)
		}
		grouped[shop.SourcePlatform] = append(grouped[shop.SourcePlatform], shop)
	}

	result := make([]model.ShopsByPlatformResp, 0, len(grouped))
	for _, platform := range platformOrder {
		result = append(result, model.ShopsByPlatformResp{
			Platform: platform,
			Shops:    grouped[platform],
		})
	}
	return result, nil
}

// ListPlatforms returns distinct platform names.
func (s *OrderService) ListPlatforms() ([]string, error) {
	return s.shopRepo.ListPlatforms()
}

// GetAccountShops returns the shop IDs assigned to an account.
func (s *OrderService) GetAccountShops(accountID uint64) ([]uint64, error) {
	return s.shopRepo.GetAccountShopIDs(accountID)
}

// UpdateAccountShops replaces shop permissions for an account.
func (s *OrderService) UpdateAccountShops(accountID uint64, shopIDs []uint64) error {
	return s.shopRepo.ReplaceAccountShops(accountID, shopIDs)
}

// GetStatusOptions returns all available process status options for the frontend.
func (s *OrderService) GetStatusOptions() []model.StatusOption {
	return model.GetAllProcessStatusOptions()
}

// BatchUpdateOrders updates specified fields for multiple orders in the local database.
func (s *OrderService) BatchUpdateOrders(items []model.UpdateOrderItem) error {
	return s.orderRepo.BatchUpdateTradesByTradeNo(items)
}
