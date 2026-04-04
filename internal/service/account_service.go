package service

import (
	"errors"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccountService struct {
	repo *repository.AccountRepo
}

func NewAccountService(repo *repository.AccountRepo) *AccountService {
	return &AccountService{repo: repo}
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

	return &model.AccountDetailResp{
		ID:          account.ID,
		Username:    account.Username,
		RealName:    account.RealName,
		Role:        account.Role,
		Permissions: permDetails,
		CreatedAt:   account.CreatedAt,
	}, nil
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
