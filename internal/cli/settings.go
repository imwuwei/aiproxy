package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"aiproxy/internal/config"
	"aiproxy/internal/models"
)

// runSettings 配置管理命令分发。
func runSettings(dbPath string, jsonOut bool, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, `配置管理:
  settings show                 显示当前配置（API Key 脱敏）
  settings set                  修改配置（一次可设置多个项）
  settings gen-token            生成随机访问令牌（sk-...）

settings set 可用参数:
  --listen-addr <地址>       监听地址（需重启生效）
  --listen-port <端口>       监听端口（需重启生效）
  --access-token <令牌>      访问令牌（带 --auth-enabled 时生效）
  --auth-enabled <true|false> 启用代理鉴权
  --model-sync-interval <N>  模型刷新间隔（分钟）
  --proxy-timeout <N>        请求超时（秒）
  --breaker-threshold <N>    熔断阈值（连续失败次数）
  --breaker-cooldown <N>     熔断冷却（秒）
  --log-retention-days <N>   日志保留天数
  --debug <true|false>       调试日志

例子:
  aiproxy settings set --listen-port 8080 --auth-enabled true
  aiproxy settings set --access-token sk-new-token --model-sync-interval 30
`)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "show":
		return settingsShow(dbPath, jsonOut, subArgs)
	case "set":
		return settingsSet(dbPath, jsonOut, subArgs)
	case "gen-token":
		return settingsGenToken(dbPath, jsonOut, subArgs)
	default:
		return fmt.Errorf("未知配置命令: %s（可用: show/set/gen-token）", sub)
	}
}

// settingsShow 显示当前配置（API Key 脱敏）。
func settingsShow(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("show", "settings show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	cfg := comps.LoadConfig()

	if jsonOut {
		// 输出完整配置（含脱敏令牌）
		return printJSON(map[string]any{
			"listen_addr":         cfg.ListenAddr,
			"listen_port":         cfg.ListenPort,
			"access_token":        maskKey(cfg.AccessToken),
			"auth_enabled":        cfg.AuthEnabled,
			"model_sync_interval": int(cfg.ModelSyncInterval.Minutes()),
			"proxy_timeout":       int(cfg.ProxyTimeout.Seconds()),
			"breaker_threshold":   cfg.BreakerThreshold,
			"breaker_cooldown":    int(cfg.BreakerCooldown.Seconds()),
			"log_retention_days":  cfg.LogRetentionDays,
			"debug":               cfg.Debug,
			"proxy_addr":          cfg.ProxyAddr(),
			"base_url":            cfg.BaseURL(),
		})
	}

	fmt.Printf("监听地址:        %s\n", cfg.ListenAddr)
	fmt.Printf("监听端口:        %d\n", cfg.ListenPort)
	fmt.Printf("代理地址:        http://%s\n", cfg.ProxyAddr())
	fmt.Printf("OpenAI 接口:     %s\n", cfg.BaseURL())
	fmt.Printf("访问令牌:        %s\n", maskKey(cfg.AccessToken))
	fmt.Printf("启用鉴权:        %s\n", boolYesNo(cfg.AuthEnabled))
	fmt.Printf("模型刷新间隔:    %d 分钟\n", int(cfg.ModelSyncInterval.Minutes()))
	fmt.Printf("请求超时:        %d 秒\n", int(cfg.ProxyTimeout.Seconds()))
	fmt.Printf("熔断阈值:        %d 次\n", cfg.BreakerThreshold)
	fmt.Printf("熔断冷却:        %d 秒\n", int(cfg.BreakerCooldown.Seconds()))
	fmt.Printf("日志保留:        %d 天\n", cfg.LogRetentionDays)
	fmt.Printf("调试日志:        %s\n", boolYesNo(cfg.Debug))
	return nil
}

// settingsSet 修改配置。
func settingsSet(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("set", "settings set [参数...]")
	listenAddr := fs.String("listen-addr", "", "监听地址")
	listenPort := fs.Int("listen-port", 0, "监听端口")
	accessToken := fs.String("access-token", "", "访问令牌")
	authEnabled := fs.String("auth-enabled", "", "启用鉴权 true/false")
	syncInterval := fs.Int("model-sync-interval", 0, "模型刷新间隔（分钟）")
	proxyTimeout := fs.Int("proxy-timeout", 0, "请求超时（秒）")
	breakerThreshold := fs.Int("breaker-threshold", 0, "熔断阈值")
	breakerCooldown := fs.Int("breaker-cooldown", 0, "熔断冷却（秒）")
	logRetentionDays := fs.Int("log-retention-days", 0, "日志保留天数")
	debug := fs.String("debug", "", "调试日志 true/false")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// 校验
	if *listenPort < 0 || *listenPort > 65535 {
		return fmt.Errorf("参数 --listen-port 无效: %d（有效范围 1-65535）", *listenPort)
	}
	if *syncInterval < 0 {
		return fmt.Errorf("参数 --model-sync-interval 必须为正整数")
	}
	if *proxyTimeout < 0 {
		return fmt.Errorf("参数 --proxy-timeout 必须为正整数")
	}
	if *breakerThreshold < 0 {
		return fmt.Errorf("参数 --breaker-threshold 必须为正整数")
	}
	if *breakerCooldown < 0 {
		return fmt.Errorf("参数 --breaker-cooldown 必须为正整数")
	}
	if *logRetentionDays < 0 {
		return fmt.Errorf("参数 --log-retention-days 必须为正整数")
	}
	if *authEnabled != "" && *authEnabled != "true" && *authEnabled != "false" {
		return fmt.Errorf("参数 --auth-enabled 取值无效: %s（可选 true/false）", *authEnabled)
	}
	if *debug != "" && *debug != "true" && *debug != "false" {
		return fmt.Errorf("参数 --debug 取值无效: %s（可选 true/false）", *debug)
	}

	// 检查是否至少设置了一项
	anySet := flagProvided(args, "listen-addr") ||
		flagProvided(args, "listen-port") ||
		flagProvided(args, "access-token") ||
		flagProvided(args, "auth-enabled") ||
		flagProvided(args, "model-sync-interval") ||
		flagProvided(args, "proxy-timeout") ||
		flagProvided(args, "breaker-threshold") ||
		flagProvided(args, "breaker-cooldown") ||
		flagProvided(args, "log-retention-days") ||
		flagProvided(args, "debug")
	if !anySet {
		return errors.New("未指定任何要修改的配置项（运行 \"aiproxy settings set --help\" 查看可用参数）")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	// 启用鉴权时令牌不能为空
	if *authEnabled == "true" && *accessToken == "" {
		// 如果未显式传令牌但当前已有令牌，则允许保留
		cur := comps.LoadConfig()
		if cur.AccessToken == "" {
			return errors.New("启用代理鉴权时，访问令牌不能为空（--access-token）")
		}
	}

	// 逐项写入
	write := func(key, value string) error {
		if err := comps.SettingsStore.Set(key, value); err != nil {
			return fmt.Errorf("写入设置 %s 失败: %w", key, err)
		}
		return nil
	}

	if flagProvided(args, "listen-addr") {
		if err := write(models.SettingsListenAddr, *listenAddr); err != nil {
			return err
		}
	}
	if flagProvided(args, "listen-port") {
		if *listenPort <= 0 || *listenPort > 65535 {
			return fmt.Errorf("参数 --listen-port 无效: %d（有效范围 1-65535）", *listenPort)
		}
		if err := write(models.SettingsListenPort, strconv.Itoa(*listenPort)); err != nil {
			return err
		}
	}
	if flagProvided(args, "access-token") {
		if err := write(models.SettingsAccessToken, *accessToken); err != nil {
			return err
		}
	}
	if flagProvided(args, "auth-enabled") {
		if err := write(models.SettingsAuthEnabled, *authEnabled); err != nil {
			return err
		}
	}
	if flagProvided(args, "model-sync-interval") {
		if *syncInterval <= 0 {
			return fmt.Errorf("参数 --model-sync-interval 必须为正整数")
		}
		if err := write(models.SettingsModelSyncInterval, strconv.Itoa(*syncInterval)); err != nil {
			return err
		}
	}
	if flagProvided(args, "proxy-timeout") {
		if *proxyTimeout <= 0 {
			return fmt.Errorf("参数 --proxy-timeout 必须为正整数")
		}
		if err := write(models.SettingsProxyTimeoutSeconds, strconv.Itoa(*proxyTimeout)); err != nil {
			return err
		}
	}
	if flagProvided(args, "breaker-threshold") {
		if *breakerThreshold <= 0 {
			return fmt.Errorf("参数 --breaker-threshold 必须为正整数")
		}
		if err := write(models.SettingsBreakerThreshold, strconv.Itoa(*breakerThreshold)); err != nil {
			return err
		}
	}
	if flagProvided(args, "breaker-cooldown") {
		if *breakerCooldown <= 0 {
			return fmt.Errorf("参数 --breaker-cooldown 必须为正整数")
		}
		if err := write(models.SettingsBreakerCooldownSec, strconv.Itoa(*breakerCooldown)); err != nil {
			return err
		}
	}
	if flagProvided(args, "log-retention-days") {
		if *logRetentionDays <= 0 {
			return fmt.Errorf("参数 --log-retention-days 必须为正整数")
		}
		if err := write(models.SettingsLogRetentionDays, strconv.Itoa(*logRetentionDays)); err != nil {
			return err
		}
	}
	if flagProvided(args, "debug") {
		if err := write(models.SettingsDebug, *debug); err != nil {
			return err
		}
	}

	// 重新加载最新配置
	cfg := comps.LoadConfig()

	changedRestart := flagProvided(args, "listen-addr") || flagProvided(args, "listen-port")

	if jsonOut {
		return printJSON(map[string]any{
			"saved":         true,
			"restartNeeded": changedRestart,
		})
	}

	fmt.Println("设置已保存。")
	if changedRestart {
		fmt.Println("注意: 监听地址/端口修改需重启服务后才能生效。")
	}
	fmt.Printf("当前代理地址: http://%s\n", cfg.ProxyAddr())
	return nil
}

// settingsGenToken 生成随机访问令牌并提示（不自动写入，需配合 settings set 使用）。
func settingsGenToken(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("gen-token", "settings gen-token [--save]")
	save := fs.Bool("save", false, "同时写入配置（可选）")
	if err := fs.Parse(args); err != nil {
		return err
	}

	token := config.GenerateAccessToken()

	if *save {
		comps, err := openApp(dbPath)
		if err != nil {
			return err
		}
		defer comps.Close()
		if err := comps.SettingsStore.Set(models.SettingsAccessToken, token); err != nil {
			return err
		}
	}

	if jsonOut {
		return printJSON(map[string]any{
			"access_token": token,
			"saved":        *save,
		})
	}
	if *save {
		fmt.Printf("已生成并保存访问令牌: %s\n", token)
	} else {
		fmt.Printf("新访问令牌: %s\n", token)
		fmt.Println("提示: 使用 \"aiproxy settings set --access-token <令牌>\" 应用到配置。")
	}
	return nil
}
