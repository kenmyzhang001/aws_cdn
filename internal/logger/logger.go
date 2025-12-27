package logger

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *logrus.Logger

// InitLogger 初始化日志系统
// logDir: 日志文件目录
// logFileName: 日志文件名（不含扩展名）
// maxSize: 单个日志文件最大大小（MB），默认500MB，5G=5120MB
// maxBackups: 保留的备份文件数量，默认10个
// maxAge: 保留天数，0表示不删除
func InitLogger(logDir, logFileName string, maxSize, maxBackups int, maxAge int) {
	Log = logrus.New()

	// 设置日志格式为JSON格式，方便解析
	Log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// 设置日志级别
	Log.SetLevel(logrus.InfoLevel)

	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		Log.WithError(err).Error("创建日志目录失败，将使用标准输出")
		Log.SetOutput(os.Stdout)
		return
	}

	// 配置日志轮转
	logPath := filepath.Join(logDir, logFileName+".log")
	writer := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSize,    // 单个文件最大大小（MB）
		MaxBackups: maxBackups, // 保留的备份文件数量
		MaxAge:     maxAge,     // 保留天数（0表示不删除）
		Compress:   true,       // 压缩旧日志文件
		LocalTime:  true,       // 使用本地时间
	}

	// 只输出到文件，不输出到控制台
	Log.SetOutput(writer)
}

// GetLogger 获取日志实例
func GetLogger() *logrus.Logger {
	if Log == nil {
		// 如果没有初始化，使用默认配置
		InitLogger("./logs", "app", 20, 10, 0)
	}
	return Log
}

