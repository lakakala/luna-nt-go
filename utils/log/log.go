package log

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

// 初始化项目全局zap Logger
func init() {
	// 步骤1：配置lumberjack（日志轮转）
	lumberjackConfig := &lumberjack.Logger{
		Filename:   "./logs/app.log", // 日志文件存储路径（目录需提前创建，否则会报错）
		MaxSize:    100,              // 单个日志文件的最大大小（单位：MB）
		MaxBackups: 10,               // 保留的旧日志文件最大数量
		MaxAge:     7,                // 保留的旧日志文件最大天数
		Compress:   true,             // 是否压缩旧日志文件（gzip）
		LocalTime:  true,             // 是否使用本地时间命名旧日志文件（默认是UTC时间）
	}

	// 步骤2：配置zap核心参数（编码格式、日志级别、输出目标）
	// 2.1 定义日志编码配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",                         // 日志中时间字段的key
		LevelKey:       "level",                        // 日志中级别字段的key
		NameKey:        "logger",                       // 日志中日志器名称字段的key
		CallerKey:      "caller",                       // 日志中调用方（文件+行号）字段的key
		MessageKey:     "msg",                          // 日志中消息内容字段的key
		StacktraceKey:  "stacktrace",                   // 日志中堆栈信息字段的key
		LineEnding:     zapcore.DefaultLineEnding,      // 日志行结尾符
		EncodeLevel:    zapcore.CapitalLevelEncoder,    // 级别编码格式（INFO、ERROR）
		EncodeTime:     customTimeEncoder,              // 自定义时间格式（默认是ISO8601，这里改为常规格式）
		EncodeDuration: zapcore.SecondsDurationEncoder, // 耗时编码格式（秒级）
		EncodeCaller:   zapcore.ShortCallerEncoder,     // 调用方编码格式（短路径：包/文件.go:行号）
	}

	// 2.2 选择编码器（生产环境用JSONEncoder，开发环境用ConsoleEncoder）
	// 这里通过环境变量区分，实际项目可配置在配置文件中
	var encoder zapcore.Encoder
	var writeSyncer zapcore.WriteSyncer
	env := os.Getenv("APP_ENV")
	if env == "production" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
		writeSyncer = zapcore.AddSync(lumberjackConfig)

	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 2.3 定义日志级别（最低级别，高于等于该级别的日志才会输出）
	// zap.DebugLevel < zap.InfoLevel < zap.WarnLevel < zap.ErrorLevel < zap.DPanicLevel < zap.PanicLevel < zap.FatalLevel
	level := zapcore.InfoLevel

	// 步骤3：创建zap核心对象，再生成Logger
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 步骤4：添加额外选项（开启调用方、开启堆栈跟踪）
	logger = zap.New(core,
		zap.AddCaller(),                   // 开启调用方信息（文件+行号）
		zap.AddStacktrace(zap.ErrorLevel), // 只有Error级别及以上才输出堆栈信息
	)
}

// 自定义时间格式：yyyy-MM-dd HH:mm:ss
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

func CtxDebugf(ctx context.Context, format string, args ...interface{}) {
	logger.Debug(fmt.Sprintf(format, args...))
}

func CtxInfof(ctx context.Context, format string, args ...interface{}) {
	logger.Info(fmt.Sprintf(format, args...))
}

func CtxWarnf(ctx context.Context, format string, args ...interface{}) {
	logger.Warn(fmt.Sprintf(format, args...))
}

func CtxErrorf(ctx context.Context, format string, args ...interface{}) {
	logger.Error(fmt.Sprintf(format, args...))
}
