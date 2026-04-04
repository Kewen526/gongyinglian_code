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
