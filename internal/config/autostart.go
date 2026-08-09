package config

import "errors"

// ErrAutoStartUnsupported 当前平台不支持开机自启动时返回的错误。
var ErrAutoStartUnsupported = errors.New("当前平台不支持开机自启动")
