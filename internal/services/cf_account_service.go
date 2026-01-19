package services

import (
	"aws_cdn/internal/models"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type CFAccountService struct {
	db *gorm.DB
}

func NewCFAccountService(db *gorm.DB) *CFAccountService {
	return &CFAccountService{db: db}
}

// ListCFAccounts 列出所有 Cloudflare 账号
func (s *CFAccountService) ListCFAccounts() ([]models.CFAccount, error) {
	var accounts []models.CFAccount
	if err := s.db.Order("id DESC").Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("获取Cloudflare账号列表失败: %w", err)
	}
	return accounts, nil
}

// GetCFAccount 获取 Cloudflare 账号信息
func (s *CFAccountService) GetCFAccount(id uint) (*models.CFAccount, error) {
	var account models.CFAccount
	if err := s.db.First(&account, id).Error; err != nil {
		return nil, fmt.Errorf("Cloudflare账号不存在: %w", err)
	}
	return &account, nil
}

// CreateCFAccount 创建 Cloudflare 账号
func (s *CFAccountService) CreateCFAccount(email, password, apiToken, note string) (*models.CFAccount, error) {
	// 检查邮箱是否已存在
	var existingAccount models.CFAccount
	if err := s.db.Where("email = ?", email).First(&existingAccount).Error; err == nil {
		return nil, fmt.Errorf("该邮箱已存在")
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("检查邮箱失败: %w", err)
	}

	// 对密码进行 bcrypt 哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}

	account := &models.CFAccount{
		Email:    email,
		Password: string(hashedPassword),
		APIToken: apiToken, // 暂时明文存储，后续可以改进为加密存储
		Note:     note,
	}

	if err := s.db.Create(account).Error; err != nil {
		return nil, fmt.Errorf("创建Cloudflare账号失败: %w", err)
	}

	return account, nil
}

// UpdateCFAccount 更新 Cloudflare 账号
func (s *CFAccountService) UpdateCFAccount(id uint, email, password, apiToken, note *string) (*models.CFAccount, error) {
	account, err := s.GetCFAccount(id)
	if err != nil {
		return nil, err
	}

	// 如果更新邮箱，检查是否与其他账号冲突
	if email != nil && *email != account.Email {
		var existingAccount models.CFAccount
		if err := s.db.Where("email = ? AND id != ?", *email, id).First(&existingAccount).Error; err == nil {
			return nil, fmt.Errorf("该邮箱已被其他账号使用")
		} else if err != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("检查邮箱失败: %w", err)
		}
		account.Email = *email
	}

	// 如果更新密码，重新加密
	if password != nil && *password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("密码加密失败: %w", err)
		}
		account.Password = string(hashedPassword)
	}

	// 如果更新 API Token（只有非空字符串才更新）
	if apiToken != nil && *apiToken != "" {
		account.APIToken = *apiToken // 暂时明文存储，后续可以改进为加密存储
	}

	// 如果更新备注
	if note != nil {
		account.Note = *note
	}

	if err := s.db.Save(account).Error; err != nil {
		return nil, fmt.Errorf("更新Cloudflare账号失败: %w", err)
	}

	return account, nil
}

// DeleteCFAccount 删除 Cloudflare 账号
func (s *CFAccountService) DeleteCFAccount(id uint) error {
	account, err := s.GetCFAccount(id)
	if err != nil {
		return err
	}

	if err := s.db.Delete(account).Error; err != nil {
		return fmt.Errorf("删除Cloudflare账号失败: %w", err)
	}

	return nil
}

// VerifyPassword 验证密码
func (s *CFAccountService) VerifyPassword(account *models.CFAccount, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(password))
	return err == nil
}

// GetAPIToken 获取 API Token（解密后返回）
func (s *CFAccountService) GetAPIToken(account *models.CFAccount) string {
	// 暂时直接返回，后续可以改进为解密
	return account.APIToken
}
