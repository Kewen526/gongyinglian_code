package service

import (
	"errors"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccountService struct {
	repo     *repository.AccountRepo
	shopRepo *repository.ShopRepo
}

func NewAccountService(repo *repository.AccountRepo, shopRepo *repository.ShopRepo) *AccountService {
	return &AccountService{repo: repo, shopRepo: shopRepo}
}

func (s *AccountService) Login(req *model.LoginReq) (*model.Account, error) {
	account, err := s.repo.GetByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户名或密码错误")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	return account, nil
}

func (s *AccountService) CreateAccount(req *model.CreateAccountReq) (*model.Account, error) {
	// Check if username already exists
	_, err := s.repo.GetByUsername(req.Username)
	if err == nil {
		return nil, errors.New("用户名已存在")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	account := &model.Account{
		Username: req.Username,
		Password: string(hashed),
		RealName: req.RealName,
		Role:     req.Role,
	}

	// Build permissions
	perms := make([]model.AccountPermission, 0, len(req.Permissions))
	for _, p := range req.Permissions {
		perms = append(perms, model.AccountPermission{
			ModuleID: p.ModuleID,
			CanView:  p.CanView,
			CanEdit:  p.CanEdit,
		})
	}

	if err := s.repo.CreateAccountWithPermissions(account, perms); err != nil {
		return nil, err
	}
	return account, nil
}

func (s *AccountService) GetAllModules() ([]model.Module, error) {
	return s.repo.GetAllModules()
}

// ListAccounts returns a paginated list of accounts with their permissions and shop_ids.
func (s *AccountService) ListAccounts(page, pageSize int) (*model.AccountListResp, error) {
	accounts, total, err := s.repo.ListAccounts(page, pageSize)
	if err != nil {
		return nil, err
	}

	list := make([]model.AccountDetailResp, 0, len(accounts))
	for _, acc := range accounts {
		detail, err := s.GetAccountDetail(acc.ID)
		if err != nil {
			continue
		}
		list = append(list, *detail)
	}

	return &model.AccountListResp{
		Total: total,
		List:  list,
	}, nil
}

func (s *AccountService) GetAccountDetail(id uint64) (*model.AccountDetailResp, error) {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	perms, err := s.repo.GetPermissionsByAccountID(id)
	if err != nil {
		return nil, err
	}

	// Collect module IDs and fetch modules
	moduleIDs := make([]uint64, 0, len(perms))
	for _, p := range perms {
		moduleIDs = append(moduleIDs, p.ModuleID)
	}

	moduleMap := make(map[uint64]model.Module)
	if len(moduleIDs) > 0 {
		modules, err := s.repo.GetModulesByIDs(moduleIDs)
		if err != nil {
			return nil, err
		}
		for _, m := range modules {
			moduleMap[m.ID] = m
		}
	}

	permDetails := make([]model.PermissionDetail, 0, len(perms))
	for _, p := range perms {
		m := moduleMap[p.ModuleID]
		permDetails = append(permDetails, model.PermissionDetail{
			ModuleID:   p.ModuleID,
			ModuleName: m.Name,
			ModuleCode: m.Code,
			CanView:    p.CanView,
			CanEdit:    p.CanEdit,
		})
	}

	// Get shop permissions
	var shopIDs []uint64
	if s.shopRepo != nil {
		ids, err := s.shopRepo.GetAccountShopIDs(id)
		if err == nil {
			shopIDs = ids
		}
	}
	if shopIDs == nil {
		shopIDs = []uint64{}
	}

	return &model.AccountDetailResp{
		ID:          account.ID,
		Username:    account.Username,
		RealName:    account.RealName,
		Role:        account.Role,
		Permissions: permDetails,
		ShopIDs:     shopIDs,
		CreatedAt:   account.CreatedAt,
	}, nil
}

// UpdateAccount updates account basic info (username/password/real_name/role).
func (s *AccountService) UpdateAccount(id uint64, req *model.UpdateAccountReq) error {
	// Verify account exists
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	updates := make(map[string]interface{})

	if req.Username != nil && *req.Username != existing.Username {
		// Check if new username is taken
		other, err := s.repo.GetByUsername(*req.Username)
		if err == nil && other.ID != id {
			return errors.New("用户名已存在")
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		updates["username"] = *req.Username
	}

	if req.Password != nil && *req.Password != "" {
		if len(*req.Password) < 6 {
			return errors.New("密码至少6位")
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		updates["password"] = string(hashed)
	}

	if req.RealName != nil {
		updates["real_name"] = *req.RealName
	}

	if req.Role != nil {
		updates["role"] = *req.Role
	}

	if len(updates) == 0 {
		return nil
	}

	return s.repo.UpdateAccount(id, updates)
}

// DeleteAccount deletes an account. Super admin cannot be deleted.
func (s *AccountService) DeleteAccount(id uint64) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if account.Role == model.RoleSuperAdmin {
		return errors.New("超级管理员账号不可删除")
	}
	return s.repo.DeleteAccount(id)
}

func (s *AccountService) UpdatePermissions(accountID uint64, req *model.UpdatePermissionsReq) error {
	// Verify account exists
	_, err := s.repo.GetByID(accountID)
	if err != nil {
		return err
	}

	perms := make([]model.AccountPermission, 0, len(req.Permissions))
	for _, p := range req.Permissions {
		perms = append(perms, model.AccountPermission{
			AccountID: accountID,
			ModuleID:  p.ModuleID,
			CanView:   p.CanView,
			CanEdit:   p.CanEdit,
		})
	}

	return s.repo.ReplacePermissions(accountID, perms)
}
