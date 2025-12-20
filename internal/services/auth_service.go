package services

import (
	"aws_cdn/internal/auth"
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
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
	log := logger.GetLogger()
	log.WithField("username", username).Info("用户尝试登录")

	var user models.User
	if err := s.db.Where("username = ? AND is_active = ?", username, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.WithField("username", username).Warn("登录失败：用户不存在或未激活")
			return "", errors.New("用户名或密码错误")
		}
		log.WithError(err).WithField("username", username).Error("查询用户失败")
		return "", err
	}

	// 校验密码（假定 Password 字段保存的是 bcrypt 哈希）
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		log.WithFields(map[string]interface{}{
			"user_id":  user.ID,
			"username": username,
		}).Warn("登录失败：密码错误")
		return "", errors.New("用户名或密码错误")
	}

	// 如果开启了谷歌验证码，则校验 TOTP
	if user.IsTwoFactorEnabled {
		if user.TwoFactorSecret == "" {
			log.WithFields(map[string]interface{}{
				"user_id":  user.ID,
				"username": username,
			}).Error("用户启用了二步验证但未配置密钥")
			return "", errors.New("未配置谷歌验证码密钥")
		}
		if ok := totp.Validate(otpCode, user.TwoFactorSecret); !ok {
			log.WithFields(map[string]interface{}{
				"user_id":  user.ID,
				"username": username,
			}).Warn("登录失败：谷歌验证码错误")
			return "", errors.New("谷歌验证码错误")
		}
		log.WithFields(map[string]interface{}{
			"user_id":  user.ID,
			"username": username,
		}).Debug("谷歌验证码验证成功")
	} else {
		// 如果未开启二步验证，但前端传了验证码，可以忽略；如需强制要求，可在此报错
	}

	// 生成 JWT
	token, err := auth.GenerateToken(user.ID, user.Username, s.jwtSecret, s.expireHours)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"user_id":  user.ID,
			"username": username,
		}).Error("生成JWT令牌失败")
		return "", err
	}

	log.WithFields(map[string]interface{}{
		"user_id":  user.ID,
		"username": username,
		"two_factor_enabled": user.IsTwoFactorEnabled,
	}).Info("用户登录成功")

	return token, nil
}


