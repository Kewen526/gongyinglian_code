package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
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
			log.Printf("[Auth] Login failed: user=%s reason=not_found\n", req.Username)
			return nil, errors.New("用户名或密码错误")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil {
		log.Printf("[Auth] Login failed: user=%s reason=wrong_password\n", req.Username)
		return nil, errors.New("用户名或密码错误")
	}

	log.Printf("[Auth] Login success: user=%s id=%d\n", req.Username, account.ID)
	return account, nil
}

// canManageTarget checks if the caller can manage the target account.
// Super admin can manage anyone. Others can only manage their descendants.
func (s *AccountService) canManageTarget(callerID uint64, callerRole uint8, targetID uint64) error {
	if callerRole == model.RoleSuperAdmin {
		return nil
	}
	isDesc, err := s.repo.IsDescendantOf(targetID, callerID)
	if err != nil {
		return errors.New("权限验证失败")
	}
	if !isDesc {
		return errors.New("无权操作该账号")
	}
	return nil
}

func (s *AccountService) CreateAccount(req *model.CreateAccountReq, callerID uint64, callerRole uint8) (*model.Account, error) {
	// Check if username already exists
	_, err := s.repo.GetByUsername(req.Username)
	if err == nil {
		return nil, errors.New("用户名已存在")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Hierarchy enforcement for non-super-admin callers
	if callerRole != model.RoleSuperAdmin {
		if err := s.validateCreateHierarchy(req, callerID, callerRole); err != nil {
			return nil, err
		}
	}

	// Validate parent based on role
	if err := s.validateParent(req.Role, req.ParentID); err != nil {
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
		ParentID: req.ParentID,
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

	// Permission subset validation for non-super-admin
	if callerRole != model.RoleSuperAdmin && len(perms) > 0 {
		if err := s.validatePermissionSubset(callerID, perms); err != nil {
			return nil, err
		}
	}

	if err := s.repo.CreateAccountWithPermissions(account, perms); err != nil {
		return nil, err
	}
	return account, nil
}

// validateCreateHierarchy ensures the caller can create an account with the given role/parent.
func (s *AccountService) validateCreateHierarchy(req *model.CreateAccountReq, callerID uint64, callerRole uint8) error {
	switch callerRole {
	case model.RoleTeamLead:
		// Team lead can create: supervisor (parent=self) or employee (parent=one of their supervisors)
		switch req.Role {
		case model.RoleSupervisor:
			if req.ParentID == nil || *req.ParentID != callerID {
				return errors.New("团队负责人只能创建自己下属的主管")
			}
		case model.RoleEmployee:
			if req.ParentID == nil {
				return errors.New("创建员工时必须指定所属主管")
			}
			// Parent must be a supervisor who is a descendant of this team lead
			isDesc, err := s.repo.IsDescendantOf(*req.ParentID, callerID)
			if err != nil {
				return errors.New("权限验证失败")
			}
			if !isDesc {
				return errors.New("只能为自己下属的主管创建员工")
			}
		default:
			return errors.New("团队负责人无权创建该角色的账号")
		}
	case model.RoleSupervisor:
		// Supervisor can only create employees under themselves
		if req.Role != model.RoleEmployee {
			return errors.New("主管只能创建员工账号")
		}
		if req.ParentID == nil || *req.ParentID != callerID {
			return errors.New("主管只能创建自己下属的员工")
		}
	default:
		return errors.New("无权创建账号")
	}
	return nil
}

// validatePermissionSubset ensures the permissions being assigned are a subset of the caller's own.
func (s *AccountService) validatePermissionSubset(callerID uint64, perms []model.AccountPermission) error {
	callerPerms, err := s.repo.GetPermissionsByAccountID(callerID)
	if err != nil {
		return errors.New("获取权限失败")
	}
	callerPermMap := make(map[uint64]model.AccountPermission, len(callerPerms))
	for _, p := range callerPerms {
		callerPermMap[p.ModuleID] = p
	}
	for _, p := range perms {
		cp, ok := callerPermMap[p.ModuleID]
		if !ok {
			return fmt.Errorf("无权分配模块ID=%d的权限", p.ModuleID)
		}
		if p.CanView == 1 && cp.CanView == 0 && cp.CanEdit == 0 {
			return fmt.Errorf("无权分配模块ID=%d的查看权限", p.ModuleID)
		}
		if p.CanEdit == 1 && cp.CanEdit == 0 {
			return fmt.Errorf("无权分配模块ID=%d的编辑权限", p.ModuleID)
		}
	}
	return nil
}

// validateParent checks that the parent_id matches the required role hierarchy.
// Supervisor (2) must have a TeamLead parent; Employee (3) must have a Supervisor parent.
func (s *AccountService) validateParent(role uint8, parentID *uint64) error {
	switch role {
	case model.RoleSupervisor:
		if parentID == nil {
			return errors.New("新增主管时必须选择所属团队负责人")
		}
		parent, err := s.repo.GetByID(*parentID)
		if err != nil {
			return errors.New("所选团队负责人不存在")
		}
		if parent.Role != model.RoleTeamLead {
			return errors.New("主管的上级必须是团队负责人")
		}
	case model.RoleEmployee:
		if parentID == nil {
			return errors.New("新增员工时必须选择所属主管")
		}
		parent, err := s.repo.GetByID(*parentID)
		if err != nil {
			return errors.New("所选主管不存在")
		}
		if parent.Role != model.RoleSupervisor {
			return errors.New("员工的上级必须是主管")
		}
	}
	return nil
}

func (s *AccountService) GetAllModules() ([]model.Module, error) {
	return s.repo.GetAllModules()
}

// ListAccounts returns accounts visible to the caller.
// Super admin sees all (paginated); others see only their descendants.
func (s *AccountService) ListAccounts(page, pageSize int, callerID uint64, callerRole uint8) (*model.AccountListResp, error) {
	if callerRole == model.RoleSuperAdmin {
		return s.listAllAccounts(page, pageSize)
	}
	return s.listSubordinateAccounts(callerID)
}

func (s *AccountService) listAllAccounts(page, pageSize int) (*model.AccountListResp, error) {
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

func (s *AccountService) listSubordinateAccounts(callerID uint64) (*model.AccountListResp, error) {
	accounts, err := s.repo.ListSubordinateAccounts(callerID)
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
		Total: int64(len(list)),
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

	// Resolve parent name
	var parentName string
	if account.ParentID != nil {
		if parent, err := s.repo.GetByID(*account.ParentID); err == nil {
			parentName = parent.RealName
		}
	}

	return &model.AccountDetailResp{
		ID:          account.ID,
		Username:    account.Username,
		RealName:    account.RealName,
		Role:        account.Role,
		ParentID:    account.ParentID,
		ParentName:  parentName,
		Permissions: permDetails,
		ShopIDs:     shopIDs,
		CreatedAt:   account.CreatedAt,
	}, nil
}

// UpdateAccount updates account basic info with hierarchy enforcement.
func (s *AccountService) UpdateAccount(id uint64, req *model.UpdateAccountReq, callerID uint64, callerRole uint8) error {
	// Hierarchy check
	if err := s.canManageTarget(callerID, callerRole, id); err != nil {
		return err
	}

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

	if req.ParentID != nil {
		targetRole := existing.Role
		if req.Role != nil {
			targetRole = *req.Role
		}
		if err := s.validateParent(targetRole, req.ParentID); err != nil {
			return err
		}
		updates["parent_id"] = *req.ParentID
	}

	if len(updates) == 0 {
		return nil
	}

	return s.repo.UpdateAccount(id, updates)
}

// DeleteAccount deletes an account with hierarchy enforcement.
func (s *AccountService) DeleteAccount(id uint64, callerID uint64, callerRole uint8) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if account.Role == model.RoleSuperAdmin {
		return errors.New("超级管理员账号不可删除")
	}
	// Hierarchy check
	if err := s.canManageTarget(callerID, callerRole, id); err != nil {
		return err
	}
	return s.repo.DeleteAccount(id)
}

// GetProductScope returns the product scope for an account.
func (s *AccountService) GetProductScope(accountID uint64) (*model.ProductScopeResp, error) {
	scope, err := s.repo.GetProductScope(accountID)
	if err != nil {
		return &model.ProductScopeResp{Suppliers: []string{}, Tags: []string{}, HiddenFields: []string{}}, nil
	}
	suppliers := []string(scope.Suppliers)
	tags := []string(scope.Tags)
	hiddenFields := []string(scope.HiddenFields)
	if suppliers == nil {
		suppliers = []string{}
	}
	if tags == nil {
		tags = []string{}
	}
	if hiddenFields == nil {
		hiddenFields = []string{}
	}
	return &model.ProductScopeResp{Suppliers: suppliers, Tags: tags, HiddenFields: hiddenFields}, nil
}

// SaveProductScope upserts the product scope with hierarchy and subset validation.
func (s *AccountService) SaveProductScope(accountID uint64, req *model.ProductScopeReq, callerID uint64, callerRole uint8) error {
	// Hierarchy check
	if err := s.canManageTarget(callerID, callerRole, accountID); err != nil {
		return err
	}

	// Validate hidden_fields entries are in whitelist
	for _, f := range req.HiddenFields {
		if !model.IsValidHiddenField(f) {
			return fmt.Errorf("无效的隐藏字段: %s", f)
		}
	}

	// For non-super-admin, validate scope is subset of caller's own scope
	if callerRole != model.RoleSuperAdmin {
		if err := s.validateScopeSubset(callerID, req.Suppliers, req.Tags); err != nil {
			return err
		}
	}

	return s.repo.SaveProductScope(accountID, req.Suppliers, req.Tags, req.HiddenFields)
}

// validateScopeSubset ensures the suppliers/tags are a subset of the caller's own scope.
func (s *AccountService) validateScopeSubset(callerID uint64, suppliers, tags []string) error {
	callerScope, err := s.repo.GetProductScope(callerID)
	if err != nil || callerScope == nil {
		return errors.New("您尚未配置商品可视范围，无法为下属分配")
	}

	callerSupplierSet := make(map[string]bool, len(callerScope.Suppliers))
	for _, sup := range callerScope.Suppliers {
		callerSupplierSet[sup] = true
	}
	for _, sup := range suppliers {
		if !callerSupplierSet[sup] {
			return fmt.Errorf("供应商 %s 不在您的可视范围内", sup)
		}
	}

	callerTagSet := make(map[string]bool, len(callerScope.Tags))
	for _, tag := range callerScope.Tags {
		callerTagSet[tag] = true
	}
	for _, tag := range tags {
		if !callerTagSet[tag] {
			return fmt.Errorf("标签 %s 不在您的可视范围内", tag)
		}
	}

	return nil
}

// UpdatePermissions replaces permissions with hierarchy and subset validation.
func (s *AccountService) UpdatePermissions(accountID uint64, req *model.UpdatePermissionsReq, callerID uint64, callerRole uint8) error {
	// Hierarchy check
	if err := s.canManageTarget(callerID, callerRole, accountID); err != nil {
		return err
	}

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

	// Permission subset validation for non-super-admin
	if callerRole != model.RoleSuperAdmin && len(perms) > 0 {
		if err := s.validatePermissionSubset(callerID, perms); err != nil {
			return err
		}
	}

	return s.repo.ReplacePermissions(accountID, perms)
}

// ---------- TeamLeaderPaymentInfo ----------

// GetMyPaymentInfo returns the payment info for the calling team leader.
// Returns a zero-value struct (not an error) when not configured yet.
func (s *AccountService) GetMyPaymentInfo(accountID uint64) (*model.TeamLeaderPaymentInfo, error) {
	info, err := s.repo.GetPaymentInfoByAccountID(accountID)
	if err != nil {
		// Not found → return empty struct so the frontend can pre-fill the form
		return &model.TeamLeaderPaymentInfo{AccountID: accountID}, nil
	}
	return info, nil
}

// SavePaymentInfo validates and persists payment info for a team leader.
func (s *AccountService) SavePaymentInfo(accountID uint64, req *model.SavePaymentInfoReq) error {
	if err := validatePaymentInfoReq(req); err != nil {
		return err
	}
	info := &model.TeamLeaderPaymentInfo{
		AccountID:           accountID,
		CorpBankName:        strings.TrimSpace(req.CorpBankName),
		CorpAccountName:     strings.TrimSpace(req.CorpAccountName),
		CorpAccountNo:       strings.TrimSpace(req.CorpAccountNo),
		PersonalBankName:    strings.TrimSpace(req.PersonalBankName),
		PersonalAccountName: strings.TrimSpace(req.PersonalAccountName),
		PersonalAccountNo:   strings.TrimSpace(req.PersonalAccountNo),
		AlipayQR:            strings.TrimSpace(req.AlipayQR),
		WechatQR:            strings.TrimSpace(req.WechatQR),
	}
	return s.repo.UpsertPaymentInfo(info)
}

// GetLeaderPaymentInfo returns the team leader's payment info for an employee.
func (s *AccountService) GetLeaderPaymentInfo(employeeID uint64) (*model.TeamLeaderPaymentInfo, error) {
	leader, err := s.repo.FindTeamLeaderByDescendantID(employeeID)
	if err != nil {
		return nil, errors.New("查询团队负责人失败")
	}
	if leader == nil {
		return nil, errors.New("未找到所属团队负责人")
	}
	info, err := s.repo.GetPaymentInfoByAccountID(leader.ID)
	if err != nil {
		return nil, errors.New("团队负责人尚未配置收款信息")
	}
	return info, nil
}

// validatePaymentInfoReq checks that each bank channel is either fully filled or empty,
// and that at least one channel is complete.
func validatePaymentInfoReq(req *model.SavePaymentInfoReq) error {
	corpFields := []string{req.CorpBankName, req.CorpAccountName, req.CorpAccountNo}
	corpFilled := countNonEmpty(corpFields)
	if corpFilled > 0 && corpFilled < 3 {
		return errors.New("对公银行信息需要完整填写（银行名称、账户名称、账号）")
	}

	personalFields := []string{req.PersonalBankName, req.PersonalAccountName, req.PersonalAccountNo}
	personalFilled := countNonEmpty(personalFields)
	if personalFilled > 0 && personalFilled < 3 {
		return errors.New("对私银行信息需要完整填写（银行名称、账户名称、账号）")
	}

	hasCorp     := corpFilled == 3
	hasPersonal := personalFilled == 3
	hasAlipay   := strings.TrimSpace(req.AlipayQR) != ""
	hasWechat   := strings.TrimSpace(req.WechatQR) != ""

	if !hasCorp && !hasPersonal && !hasAlipay && !hasWechat {
		return errors.New("请至少填写一种完整的收款方式")
	}
	return nil
}

func countNonEmpty(fields []string) int {
	n := 0
	for _, f := range fields {
		if strings.TrimSpace(f) != "" {
			n++
		}
	}
	return n
}
