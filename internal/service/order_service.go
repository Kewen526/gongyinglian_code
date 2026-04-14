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

// getEffectiveShopIDs returns the shop IDs visible to the given account.
// SuperAdmin → nil (all shops); all other roles → own account_shop records.
func (s *OrderService) getEffectiveShopIDs(accountID uint64, role uint8) ([]uint64, error) {
	if role == model.RoleSuperAdmin {
		return nil, nil
	}
	return s.shopRepo.GetAccountShopIDs(accountID)
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

// GetOccupiedShopsDetail returns detailed assignment info for shops visible to the caller.
// Super admin sees all; others see only shops within their own scope.
func (s *OrderService) GetOccupiedShopsDetail(callerID uint64, callerRole uint8) ([]repository.ShopAssignment, error) {
	var scopeShopIDs []uint64
	if callerRole != model.RoleSuperAdmin {
		ids, err := s.shopRepo.GetAccountShopIDs(callerID)
		if err != nil {
			return nil, err
		}
		scopeShopIDs = ids
	} else {
		// Super admin: get all shop IDs
		shops, err := s.shopRepo.ListAll()
		if err != nil {
			return nil, err
		}
		for _, shop := range shops {
			scopeShopIDs = append(scopeShopIDs, shop.ID)
		}
	}
	return s.shopRepo.GetOccupiedShopsDetail(scopeShopIDs)
}

// GetAccountShops returns the shop IDs assigned to an account.
func (s *OrderService) GetAccountShops(accountID uint64) ([]uint64, error) {
	return s.shopRepo.GetAccountShopIDs(accountID)
}

// UpdateAccountShops replaces shop assignments for an account.
// All roles (team lead / supervisor / employee) may have shops assigned.
// Per-layer mutual exclusion: among siblings (same parent + same role), shops
// cannot overlap. callerID=0 means super admin (no subset check).
func (s *OrderService) UpdateAccountShops(accountID uint64, shopIDs []uint64, callerID uint64) error {
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return errors.New("账号不存在")
	}

	// If caller is not super admin, shops must be a subset of caller's own shops.
	if callerID > 0 {
		callerShops, err := s.shopRepo.GetAccountShopIDs(callerID)
		if err != nil {
			return err
		}
		callerSet := make(map[uint64]bool, len(callerShops))
		for _, id := range callerShops {
			callerSet[id] = true
		}
		for _, sid := range shopIDs {
			if !callerSet[sid] {
				return fmt.Errorf("店铺ID %d 不在您的可分配范围内", sid)
			}
		}
	}

	// Per-layer mutual exclusion: check sibling accounts
	for _, shopID := range shopIDs {
		taken, ownerID, err := s.shopRepo.IsShopAssignedToSibling(shopID, accountID, account.Role, account.ParentID)
		if err != nil {
			return err
		}
		if taken {
			owner, _ := s.accountRepo.GetByID(ownerID)
			name := fmt.Sprintf("ID=%d", ownerID)
			if owner != nil {
				name = owner.RealName
				if name == "" {
					name = owner.Username
				}
			}
			return fmt.Errorf("店铺ID %d 已分配给 %s，同级不可重复分配", shopID, name)
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
