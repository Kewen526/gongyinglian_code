package service

import (
	"errors"
	"fmt"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"
	"time"
)

type OrderService struct {
	orderRepo      *repository.OrderRepo
	shopRepo       *repository.ShopRepo
	accountRepo    *repository.AccountRepo
	billingService *BillingService
}

func NewOrderService(orderRepo *repository.OrderRepo, shopRepo *repository.ShopRepo, accountRepo *repository.AccountRepo, billingService *BillingService) *OrderService {
	return &OrderService{orderRepo: orderRepo, shopRepo: shopRepo, accountRepo: accountRepo, billingService: billingService}
}

// getEffectiveShopIDs returns the shop IDs visible to the given account based on hierarchy.
// SuperAdmin → nil (all shops); Employee → own shops;
// Supervisor → shops of direct employees; TeamLead → shops of employees under their supervisors.
func (s *OrderService) getEffectiveShopIDs(accountID uint64, role uint8) ([]uint64, error) {
	switch role {
	case model.RoleSuperAdmin:
		return nil, nil
	case model.RoleEmployee:
		return s.shopRepo.GetAccountShopIDs(accountID)
	case model.RoleSupervisor:
		empIDs, err := s.accountRepo.GetDirectSubordinateIDs(accountID)
		if err != nil {
			return nil, err
		}
		return s.shopRepo.GetShopIDsByAccountIDs(empIDs)
	case model.RoleTeamLead:
		supIDs, err := s.accountRepo.GetDirectSubordinateIDs(accountID)
		if err != nil {
			return nil, err
		}
		var allEmpIDs []uint64
		for _, supID := range supIDs {
			empIDs, err := s.accountRepo.GetDirectSubordinateIDs(supID)
			if err != nil {
				return nil, err
			}
			allEmpIDs = append(allEmpIDs, empIDs...)
		}
		return s.shopRepo.GetShopIDsByAccountIDs(allEmpIDs)
	}
	return []uint64{}, nil
}

// ListOrders returns paginated orders with permission filtering.
// If role is super admin, shopIDs filter is nil (see all).
func (s *OrderService) ListOrders(req *model.OrderListReq, accountID uint64, role uint8) (*model.OrderListResp, error) {
	shopIDs, err := s.getEffectiveShopIDs(accountID, role)
	if err != nil {
		return nil, err
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
		shopIDs, err := s.getEffectiveShopIDs(accountID, role)
		if err != nil {
			return nil, err
		}
		if !s.isShopAuthorized(trade.SysShop, shopIDs) {
			return nil, nil
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

// GetOccupiedShopIDs returns shop IDs already assigned to any employee.
// excludeAccountID > 0 means "ignore this account's own assignments" (for edit mode).
func (s *OrderService) GetOccupiedShopIDs(excludeAccountID uint64) ([]uint64, error) {
	ids, err := s.shopRepo.GetOccupiedShopIDs(excludeAccountID)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []uint64{}
	}
	return ids, nil
}

// GetAccountShops returns the shop IDs assigned to an account.
func (s *OrderService) GetAccountShops(accountID uint64) ([]uint64, error) {
	return s.shopRepo.GetAccountShopIDs(accountID)
}

// UpdateAccountShops replaces shop assignments for an account.
// Only employees (role=3) may have shops directly assigned.
// Each shop may only belong to one employee at a time.
func (s *OrderService) UpdateAccountShops(accountID uint64, shopIDs []uint64) error {
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return errors.New("账号不存在")
	}
	if account.Role != model.RoleEmployee {
		return errors.New("只能为员工分配店铺，主管和负责人通过下级员工继承可见范围")
	}
	for _, shopID := range shopIDs {
		occupied, err := s.shopRepo.IsShopAssignedToOther(shopID, accountID)
		if err != nil {
			return err
		}
		if occupied {
			return fmt.Errorf("店铺ID %d 已分配给其他员工，不可重复分配", shopID)
		}
	}
	return s.shopRepo.ReplaceAccountShops(accountID, shopIDs)
}

// GetStatusOptions returns all available process status options for the frontend.
func (s *OrderService) GetStatusOptions() []model.StatusOption {
	return model.GetAllProcessStatusOptions()
}

// BatchUpdateOrders updates specified fields for multiple orders in the local database.
// If any item sets mark to "已审核", deduction is triggered asynchronously.
func (s *OrderService) BatchUpdateOrders(items []model.UpdateOrderItem) error {
	if err := s.orderRepo.BatchUpdateTradesByTradeNo(items); err != nil {
		return err
	}
	if s.billingService == nil {
		return nil
	}
	for _, item := range items {
		if item.Mark == nil || *item.Mark != "已审核" {
			continue
		}
		trade, err := s.orderRepo.GetTradeByTradeNo(item.TradeNo)
		if err != nil || trade == nil {
			continue
		}
		_ = s.orderRepo.SetMarkApprovedAtIfNull(trade.UID, time.Now())
		s.billingService.TriggerDeductionAsync(trade)
	}
	return nil
}
