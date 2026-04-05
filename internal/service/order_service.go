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

// ListOrders returns orders visible to the account (filtered by shop permissions).
// Super admin (role=0) can see all orders.
func (s *OrderService) ListOrders(req *model.OrderListReq, accountID uint64, role uint8) (*model.OrderListResp, error) {
	var shopNames []string

	// Non-super-admin: filter by assigned shops
	if role != model.RoleSuperAdmin {
		names, err := s.shopRepo.GetAccountShopNames(accountID)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			// No shop assigned, return empty
			return &model.OrderListResp{
				List:     []model.OrderTrade{},
				Total:    0,
				Page:     req.Page,
				PageSize: req.PageSize,
			}, nil
		}
		shopNames = names
	}

	list, total, err := s.orderRepo.ListOrders(req, shopNames)
	if err != nil {
		return nil, err
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	return &model.OrderListResp{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetOrderDetail returns an order with its items.
// Non-super-admin must have the order's shop assigned.
func (s *OrderService) GetOrderDetail(id uint64, accountID uint64, role uint8) (*model.OrderDetailResp, error) {
	trade, err := s.orderRepo.GetTradeByID(id)
	if err != nil {
		return nil, err
	}

	// Permission check for non-super-admin
	if role != model.RoleSuperAdmin {
		names, err := s.shopRepo.GetAccountShopNames(accountID)
		if err != nil {
			return nil, err
		}
		hasAccess := false
		for _, n := range names {
			if n == trade.ShopName {
				hasAccess = true
				break
			}
		}
		if !hasAccess {
			return nil, ErrNoShopPermission
		}
	}

	items, err := s.orderRepo.GetItemsByTradeUID(trade.UID)
	if err != nil {
		return nil, err
	}

	return &model.OrderDetailResp{
		Trade: *trade,
		Items: items,
	}, nil
}

// GetAllShops returns all shops
func (s *OrderService) GetAllShops() ([]model.Shop, error) {
	return s.shopRepo.GetAllShops()
}

// GetShopsGrouped returns shops grouped by platform
func (s *OrderService) GetShopsGrouped() ([]model.ShopGroupedResp, error) {
	return s.shopRepo.GetShopsGroupedByPlatform()
}

// GetPlatforms returns all distinct platforms
func (s *OrderService) GetPlatforms() ([]string, error) {
	return s.orderRepo.GetDistinctPlatforms()
}

// GetAccountShops returns shops assigned to an account
func (s *OrderService) GetAccountShops(accountID uint64) (*model.AccountShopResp, error) {
	shops, err := s.shopRepo.GetAccountShops(accountID)
	if err != nil {
		return nil, err
	}
	return &model.AccountShopResp{
		AccountID: accountID,
		Shops:     shops,
	}, nil
}

// UpdateAccountShops replaces shop assignments for an account
func (s *OrderService) UpdateAccountShops(accountID uint64, shopIDs []uint64) error {
	return s.shopRepo.ReplaceAccountShops(accountID, shopIDs)
}
