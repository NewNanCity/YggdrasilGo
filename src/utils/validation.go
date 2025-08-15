// Package utils 验证工具函数
package utils

import (
	"regexp"
	"strings"
)

// 预编译的正则表达式，避免运行时重复编译
var (
	// 邮箱格式验证
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	// UUID格式验证（32位十六进制字符）
	uuidRegex = regexp.MustCompile(`^[0-9a-f]{32}$`)

	// 用户名格式验证（3-16位字母数字下划线）
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,16}$`)

	// 角色名格式验证（Minecraft官方规则）
	playerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,16}$`)

	// 材质类型验证
	textureTypeRegex = regexp.MustCompile(`^(skin|cape)$`)
)

// IsValidEmail 验证邮箱格式
func IsValidEmail(email string) bool {
	if len(email) > 254 { // RFC 5321 限制
		return false
	}
	return emailRegex.MatchString(email)
}

// IsValidUUIDFormat 验证UUID格式（32位十六进制）
func IsValidUUIDFormat(uuid string) bool {
	return len(uuid) == 32 && uuidRegex.MatchString(uuid)
}

// IsValidUsername 验证用户名格式
func IsValidUsername(username string) bool {
	return usernameRegex.MatchString(username)
}

// IsValidPlayerName 验证角色名格式
func IsValidPlayerName(name string) bool {
	return playerNameRegex.MatchString(name)
}

// IsValidTextureType 验证材质类型
func IsValidTextureType(textureType string) bool {
	return textureTypeRegex.MatchString(textureType)
}

// IsEmailFormat 检查字符串是否为邮箱格式（简单检查）
func IsEmailFormat(input string) bool {
	return strings.Contains(input, "@")
}

// SanitizeInput 清理输入字符串
func SanitizeInput(input string) string {
	// 移除前后空白字符
	input = strings.TrimSpace(input)

	// 限制长度
	if len(input) > 255 {
		input = input[:255]
	}

	return input
}

// ValidateLoginInput 验证登录输入（简化版）
func ValidateLoginInput(username, password string) bool {
	username = SanitizeInput(username)
	password = SanitizeInput(password)

	if username == "" || password == "" {
		return false
	}

	if len(password) > 255 {
		return false
	}

	// 验证用户名格式（邮箱或角色名）
	if IsEmailFormat(username) {
		return IsValidEmail(username)
	} else {
		return IsValidPlayerName(username)
	}
}

// ValidateUUIDInput 验证UUID输入（简化版）
func ValidateUUIDInput(uuid string) bool {
	uuid = SanitizeInput(uuid)
	return uuid != "" && IsValidUUIDFormat(uuid)
}

// ValidatePlayerNameInput 验证角色名输入（简化版）
func ValidatePlayerNameInput(name string) bool {
	name = SanitizeInput(name)
	return name != "" && IsValidPlayerName(name)
}

// BatchValidatePlayerNames 批量验证角色名（简化版）
func BatchValidatePlayerNames(names []string) bool {
	if len(names) == 0 || len(names) > 100 {
		return false
	}

	for _, name := range names {
		if !ValidatePlayerNameInput(name) {
			return false
		}
	}

	return true
}
