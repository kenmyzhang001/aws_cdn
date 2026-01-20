package main

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/database"
	"aws_cdn/internal/models"
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"gorm.io/gorm"
)

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量")
	}

	// 初始化配置
	cfg := config.Load()

	// 初始化数据库
	db, err := database.Initialize(database.DatabaseConfig{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 检查是否已有用户
	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		fmt.Println("数据库中已存在用户，是否继续创建新用户？(y/n): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("已取消创建用户")
			return
		}
	}

	// 读取用户输入
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("请输入用户名: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)
	if username == "" {
		log.Fatal("用户名不能为空")
	}

	fmt.Print("请输入邮箱: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" {
		log.Fatal("邮箱不能为空")
	}

	fmt.Print("请输入密码: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatalf("读取密码失败: %v", err)
	}
	password := strings.TrimSpace(string(passwordBytes))
	fmt.Println() // 换行
	if password == "" {
		log.Fatal("密码不能为空")
	}

	fmt.Print("请再次输入密码确认: ")
	passwordBytes2, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatalf("读取密码失败: %v", err)
	}
	password2 := strings.TrimSpace(string(passwordBytes2))
	fmt.Println() // 换行
	if password != password2 {
		log.Fatal("两次输入的密码不一致")
	}

	// 询问是否启用二步验证
	fmt.Print("是否启用谷歌验证码二步验证？(y/n): ")
	enable2FA, _ := reader.ReadString('\n')
	enable2FA = strings.TrimSpace(strings.ToLower(enable2FA))

	var twoFactorSecret string
	var isTwoFactorEnabled bool

	if enable2FA == "y" || enable2FA == "yes" {
		isTwoFactorEnabled = true
		// 生成 TOTP 密钥
		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      "CDN 管理平台",
			AccountName: username,
		})
		if err != nil {
			log.Fatalf("生成 TOTP 密钥失败: %v", err)
		}
		twoFactorSecret = key.Secret()

		fmt.Println("\n=== 谷歌验证码配置信息 ===")
		fmt.Printf("密钥: %s\n", twoFactorSecret)
		fmt.Printf("二维码 URL: %s\n", key.URL())
		fmt.Println("\n请使用 Google Authenticator 或其他 TOTP 应用扫描二维码或手动输入密钥")
		fmt.Println("================================\n")
	}

	// 生成密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("生成密码哈希失败: %v", err)
	}

	// 检查用户名和邮箱是否已存在
	var existingUser models.User
	if err := db.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		log.Fatalf("用户名或邮箱已存在")
	} else if err != gorm.ErrRecordNotFound {
		log.Fatalf("查询用户失败: %v", err)
	}

	// 创建用户
	user := models.User{
		Username:           username,
		Email:              email,
		Password:           string(hashedPassword),
		IsActive:           true,
		TwoFactorSecret:    twoFactorSecret,
		IsTwoFactorEnabled: isTwoFactorEnabled,
	}

	if err := db.Create(&user).Error; err != nil {
		log.Fatalf("创建用户失败: %v", err)
	}

	fmt.Println("✅ 用户创建成功！")
	fmt.Printf("用户名: %s\n", username)
	fmt.Printf("邮箱: %s\n", email)
	if isTwoFactorEnabled {
		fmt.Printf("谷歌验证码密钥: %s\n", twoFactorSecret)
		fmt.Println("⚠️  请妥善保管谷歌验证码密钥，登录时需要输入验证码")
	} else {
		fmt.Println("⚠️  未启用二步验证，建议在生产环境中启用以提高安全性")
	}
}
