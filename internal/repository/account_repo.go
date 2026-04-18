package repository

import (
	"supply-chain/internal/model"

	"gorm.io/gorm"
)

type AccountRepo struct {
	db *gorm.DB
}

func NewAccountRepo(db *gorm.DB) *AccountRepo {
	return &AccountRepo{db: db}
}

func (r *AccountRepo) Create(account *model.Account) error {
	return r.db.Create(account).Error
}

func (r *AccountRepo) GetByID(id uint64) (*model.Account, error) {
	var account model.Account
	err := r.db.First(&account, id).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepo) GetByUsername(username string) (*model.Account, error) {
	var account model.Account
	err := r.db.Where("username = ?", username).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepo) CreatePermissions(permissions []model.AccountPermission) error {
	if len(permissions) == 0 {
		return nil
	}
	return r.db.Create(&permissions).Error
}

func (r *AccountRepo) GetPermissionsByAccountID(accountID uint64) ([]model.AccountPermission, error) {
	var perms []model.AccountPermission
	err := r.db.Where("account_id = ?", accountID).Find(&perms).Error
	return perms, err
}

func (r *AccountRepo) ReplacePermissions(accountID uint64, permissions []model.AccountPermission) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", accountID).Delete(&model.AccountPermission{}).Error; err != nil {
			return err
		}
		if len(permissions) == 0 {
			return nil
		}
		return tx.Create(&permissions).Error
	})
}

func (r *AccountRepo) GetAllModules() ([]model.Module, error) {
	var modules []model.Module
	err := r.db.Order("id ASC").Find(&modules).Error
	return modules, err
}

func (r *AccountRepo) GetModulesByIDs(ids []uint64) ([]model.Module, error) {
	var modules []model.Module
	err := r.db.Where("id IN ?", ids).Find(&modules).Error
	return modules, err
}

// ListAccounts returns paginated accounts.
func (r *AccountRepo) ListAccounts(page, pageSize int) ([]model.Account, int64, error) {
	var total int64
	if err := r.db.Model(&model.Account{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var accounts []model.Account
	err := r.db.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&accounts).Error
	if err != nil {
		return nil, 0, err
	}
	return accounts, total, nil
}

// UpdateAccount updates the given fields of an account.
func (r *AccountRepo) UpdateAccount(id uint64, updates map[string]interface{}) error {
	return r.db.Model(&model.Account{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteAccount deletes an account and all its permission/shop associations.
func (r *AccountRepo) DeleteAccount(id uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", id).Delete(&model.AccountPermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_id = ?", id).Delete(&model.AccountShop{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Account{}, id).Error
	})
}

// GetDirectSubordinateIDs returns account IDs whose parent_id equals the given accountID.
func (r *AccountRepo) GetDirectSubordinateIDs(parentID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.Model(&model.Account{}).Where("parent_id = ?", parentID).Pluck("id", &ids).Error
	return ids, err
}

// GetByIDs returns accounts by a list of IDs.
func (r *AccountRepo) GetByIDs(ids []uint64) ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.Where("id IN ?", ids).Find(&accounts).Error
	return accounts, err
}

// GetProductScope returns the product scope for an employee account.
// Returns nil (no error) if no scope is configured yet.
func (r *AccountRepo) GetProductScope(accountID uint64) (*model.AccountProductScope, error) {
	var scope model.AccountProductScope
	err := r.db.Where("account_id = ?", accountID).First(&scope).Error
	if err != nil {
		return nil, err
	}
	return &scope, nil
}

// SaveProductScope upserts the product scope for an account.
func (r *AccountRepo) SaveProductScope(accountID uint64, suppliers, tags, hiddenFields []string) error {
	s := model.StringSlice(suppliers)
	t := model.StringSlice(tags)
	h := model.StringSlice(hiddenFields)
	if s == nil {
		s = model.StringSlice{}
	}
	if t == nil {
		t = model.StringSlice{}
	}
	if h == nil {
		h = model.StringSlice{}
	}
	var existing model.AccountProductScope
	err := r.db.Where("account_id = ?", accountID).First(&existing).Error
	if err != nil {
		// Create new record
		return r.db.Create(&model.AccountProductScope{
			AccountID:    accountID,
			Suppliers:    s,
			Tags:         t,
			HiddenFields: h,
		}).Error
	}
	// Update existing
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"suppliers":     s,
		"tags":          t,
		"hidden_fields": h,
	}).Error
}

// ListEmployees returns all accounts with role = RoleEmployee.
// Every employee is an auto-review participant — there is no opt-in switch.
func (r *AccountRepo) ListEmployees() ([]model.Account, error) {
	var accounts []model.Account
	err := r.db.Where("role = ?", model.RoleEmployee).Find(&accounts).Error
	return accounts, err
}

func (r *AccountRepo) CreateAccountWithPermissions(account *model.Account, permissions []model.AccountPermission) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(account).Error; err != nil {
			return err
		}
		for i := range permissions {
			permissions[i].AccountID = account.ID
		}
		if len(permissions) > 0 {
			if err := tx.Create(&permissions).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetAllDescendantIDs returns all descendant account IDs of the given account
// (direct children, grandchildren, etc.) via iterative BFS.
func (r *AccountRepo) GetAllDescendantIDs(accountID uint64) ([]uint64, error) {
	var all []uint64
	queue := []uint64{accountID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		children, err := r.GetDirectSubordinateIDs(current)
		if err != nil {
			return nil, err
		}
		all = append(all, children...)
		queue = append(queue, children...)
	}
	return all, nil
}

// IsDescendantOf checks whether accountID is a descendant of ancestorID.
func (r *AccountRepo) IsDescendantOf(accountID, ancestorID uint64) (bool, error) {
	current := accountID
	for i := 0; i < 10; i++ { // max depth guard
		acc, err := r.GetByID(current)
		if err != nil {
			return false, err
		}
		if acc.ParentID == nil {
			return false, nil
		}
		if *acc.ParentID == ancestorID {
			return true, nil
		}
		current = *acc.ParentID
	}
	return false, nil
}

// ListSubordinateAccounts returns direct subordinate accounts (for non-super-admin listing).
func (r *AccountRepo) ListSubordinateAccounts(parentID uint64) ([]model.Account, error) {
	descendantIDs, err := r.GetAllDescendantIDs(parentID)
	if err != nil {
		return nil, err
	}
	if len(descendantIDs) == 0 {
		return []model.Account{}, nil
	}
	var accounts []model.Account
	err = r.db.Where("id IN ?", descendantIDs).Order("id ASC").Find(&accounts).Error
	return accounts, err
}
