package services

import (
	"aws_cdn/internal/auth"
	"aws_cdn/internal/config"
	"aws_cdn/internal/models"
	"errors"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthService 认证服务
type AuthService struct {
	db          *gorm.DB
	jwtSecret   string
	expireHours int
}

func NewAuthService(db *gorm.DB, cfg *config.JWTConfig) *AuthService {
	return &AuthService{
		db:          db,
		jwtSecret:   cfg.Secret,
		expireHours: cfg.ExpireHours,
	}
}

// Authenticate 使用用户名 + 密码 + Google 验证码登录
func (s *AuthService) Authenticate(username, password, otpCode string) (string, error) {
	var user models.User
	if err := s.db.Where("username = ? AND is_active = ?", username, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("用户名或密码错误")
		}
		return "", err
	}

	// 校验密码（假定 Password 字段保存的是 bcrypt 哈希）
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", errors.New("用户名或密码错误")
	}

	// 如果开启了谷歌验证码，则校验 TOTP
	if user.IsTwoFactorEnabled {
		if user.TwoFactorSecret == "" {
			return "", errors.New("未配置谷歌验证码密钥")
		}
		if ok := totp.Validate(otpCode, user.TwoFactorSecret); !ok {
			return "", errors.New("谷歌验证码错误")
		}
	} else {
		// 如果未开启二步验证，但前端传了验证码，可以忽略；如需强制要求，可在此报错
	}

	// 生成 JWT
	token, err := auth.GenerateToken(user.ID, user.Username, s.jwtSecret, s.expireHours)
	if err != nil {
		return "", err
	}

	return token, nil
}


