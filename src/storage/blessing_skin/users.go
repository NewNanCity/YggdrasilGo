// Package blessing_skin BlessingSkin用户管理
package blessing_skin

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"yggdrasil-api-go/src/yggdrasil"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// GetUserByID 根据用户ID获取用户（单查询优化版）
func (s *Storage) GetUserByID(userID string) (*yggdrasil.User, error) {
	// 一次性查询用户信息、角色列表和UUID映射
	var results []struct {
		UID        uint   `gorm:"column:uid"`
		Email      string `gorm:"column:email"`
		PlayerName string `gorm:"column:player_name"`
		UUID       string `gorm:"column:uuid"`
	}

	err := s.db.Table("users u").
		Select("u.uid, u.email, p.name as player_name, uuid.uuid").
		Joins("LEFT JOIN players p ON u.uid = p.uid").
		Joins("LEFT JOIN uuid ON p.name = uuid.name").
		Where("u.uid = ?", userID).
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	// 构建用户信息
	userInfo := results[0]
	var profiles []yggdrasil.Profile
	var missingUUIDs []string

	for _, result := range results {
		if result.PlayerName != "" { // 有角色
			if result.UUID != "" {
				// UUID已存在
				profiles = append(profiles, yggdrasil.Profile{
					ID:         result.UUID,
					Name:       result.PlayerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			} else {
				// UUID缺失，需要创建
				missingUUIDs = append(missingUUIDs, result.PlayerName)
			}
		}
	}

	// 批量创建缺失的UUID
	if len(missingUUIDs) > 0 {
		uuidMap, err := s.uuidGen.GetUUIDsByNames(missingUUIDs)
		if err != nil {
			return nil, err
		}

		for _, playerName := range missingUUIDs {
			if uuid, exists := uuidMap[playerName]; exists {
				profiles = append(profiles, yggdrasil.Profile{
					ID:         uuid,
					Name:       playerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			}
		}
	}

	return &yggdrasil.User{
		ID:       fmt.Sprintf("%d", userInfo.UID),
		Email:    userInfo.Email,
		Password: "", // 不返回密码
		Profiles: profiles,
	}, nil
}

// GetUserByEmail 根据邮箱获取用户（单查询优化版）
func (s *Storage) GetUserByEmail(email string) (*yggdrasil.User, error) {
	// 一次性查询用户信息、角色列表和UUID映射
	var results []struct {
		UID        uint   `gorm:"column:uid"`
		Email      string `gorm:"column:email"`
		PlayerName string `gorm:"column:player_name"`
		UUID       string `gorm:"column:uuid"`
	}

	err := s.db.Table("users u").
		Select("u.uid, u.email, p.name as player_name, uuid.uuid").
		Joins("LEFT JOIN players p ON u.uid = p.uid").
		Joins("LEFT JOIN uuid ON p.name = uuid.name").
		Where("u.email = ?", email).
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	// 构建用户信息
	userInfo := results[0]
	var profiles []yggdrasil.Profile
	var missingUUIDs []string

	for _, result := range results {
		if result.PlayerName != "" { // 有角色
			if result.UUID != "" {
				// UUID已存在
				profiles = append(profiles, yggdrasil.Profile{
					ID:         result.UUID,
					Name:       result.PlayerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			} else {
				// UUID缺失，需要创建
				missingUUIDs = append(missingUUIDs, result.PlayerName)
			}
		}
	}

	// 批量创建缺失的UUID
	if len(missingUUIDs) > 0 {
		uuidMap, err := s.uuidGen.GetUUIDsByNames(missingUUIDs)
		if err != nil {
			return nil, err
		}

		for _, playerName := range missingUUIDs {
			if uuid, exists := uuidMap[playerName]; exists {
				profiles = append(profiles, yggdrasil.Profile{
					ID:         uuid,
					Name:       playerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			}
		}
	}

	return &yggdrasil.User{
		ID:       fmt.Sprintf("%d", userInfo.UID),
		Email:    userInfo.Email,
		Password: "", // 不返回密码
		Profiles: profiles,
	}, nil
}

// GetUserByPlayerName 根据角色名获取用户（单查询优化版）
func (s *Storage) GetUserByPlayerName(playerName string) (*yggdrasil.User, error) {
	// 一次性查询用户信息、所有角色和UUID映射
	var results []struct {
		UID        uint   `gorm:"column:uid"`
		Email      string `gorm:"column:email"`
		PlayerName string `gorm:"column:player_name"`
		UUID       string `gorm:"column:uuid"`
	}

	err := s.db.Table("players p1").
		Select("u.uid, u.email, p2.name as player_name, uuid.uuid").
		Joins("JOIN users u ON p1.uid = u.uid").
		Joins("LEFT JOIN players p2 ON u.uid = p2.uid").
		Joins("LEFT JOIN uuid ON p2.name = uuid.name").
		Where("p1.name = ?", playerName).
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("player not found")
	}

	// 构建用户信息
	userInfo := results[0]
	var profiles []yggdrasil.Profile
	var missingUUIDs []string

	for _, result := range results {
		if result.PlayerName != "" { // 有角色
			if result.UUID != "" {
				// UUID已存在
				profiles = append(profiles, yggdrasil.Profile{
					ID:         result.UUID,
					Name:       result.PlayerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			} else {
				// UUID缺失，需要创建
				missingUUIDs = append(missingUUIDs, result.PlayerName)
			}
		}
	}

	// 批量创建缺失的UUID
	if len(missingUUIDs) > 0 {
		uuidMap, err := s.uuidGen.GetUUIDsByNames(missingUUIDs)
		if err != nil {
			return nil, err
		}

		for _, playerName := range missingUUIDs {
			if uuid, exists := uuidMap[playerName]; exists {
				profiles = append(profiles, yggdrasil.Profile{
					ID:         uuid,
					Name:       playerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			}
		}
	}

	return &yggdrasil.User{
		ID:       fmt.Sprintf("%d", userInfo.UID),
		Email:    userInfo.Email,
		Password: "", // 不返回密码
		Profiles: profiles,
	}, nil
}

// GetUserByUUID 根据UUID获取用户（单查询优化版）
func (s *Storage) GetUserByUUID(uuid string) (*yggdrasil.User, error) {
	// 一次性查询用户信息、角色列表和UUID映射
	var results []struct {
		UID        uint   `gorm:"column:uid"`
		Email      string `gorm:"column:email"`
		PlayerName string `gorm:"column:player_name"`
		UUID       string `gorm:"column:uuid"`
	}

	err := s.db.Table("uuid u1").
		Select("users.uid, users.email, p.name as player_name, u2.uuid").
		Joins("JOIN players p1 ON u1.name = p1.name").
		Joins("JOIN users ON p1.uid = users.uid").
		Joins("LEFT JOIN players p ON users.uid = p.uid").
		Joins("LEFT JOIN uuid u2 ON p.name = u2.name").
		Where("u1.uuid = ?", uuid).
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	// 构建用户信息
	userInfo := results[0]
	var profiles []yggdrasil.Profile
	var missingUUIDs []string

	for _, result := range results {
		if result.PlayerName != "" { // 有角色
			if result.UUID != "" {
				// UUID已存在
				profiles = append(profiles, yggdrasil.Profile{
					ID:         result.UUID,
					Name:       result.PlayerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			} else {
				// UUID缺失，需要创建
				missingUUIDs = append(missingUUIDs, result.PlayerName)
			}
		}
	}

	// 批量创建缺失的UUID
	if len(missingUUIDs) > 0 {
		uuidMap, err := s.uuidGen.GetUUIDsByNames(missingUUIDs)
		if err != nil {
			return nil, err
		}

		for _, playerName := range missingUUIDs {
			if uuid, exists := uuidMap[playerName]; exists {
				profiles = append(profiles, yggdrasil.Profile{
					ID:         uuid,
					Name:       playerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			}
		}
	}

	return &yggdrasil.User{
		ID:       fmt.Sprintf("%d", userInfo.UID),
		Email:    userInfo.Email,
		Password: "", // 不返回密码
		Profiles: profiles,
	}, nil
}

// AuthenticateUser 用户认证（单查询优化版）
func (s *Storage) AuthenticateUser(username, password string) (*yggdrasil.User, error) {
	// 一次性查询用户信息、角色列表和UUID映射
	var results []struct {
		UID        uint   `gorm:"column:uid"`
		Email      string `gorm:"column:email"`
		Password   string `gorm:"column:password"`
		Permission int    `gorm:"column:permission"`
		Verified   bool   `gorm:"column:verified"`
		PlayerName string `gorm:"column:player_name"`
		UUID       string `gorm:"column:uuid"`
	}

	var err error
	if strings.Contains(username, "@") {
		// 邮箱登录
		err = s.db.Table("users u").
			Select("u.uid, u.email, u.password, u.permission, u.verified, p.name as player_name, uuid.uuid").
			Joins("LEFT JOIN players p ON u.uid = p.uid").
			Joins("LEFT JOIN uuid ON p.name = uuid.name").
			Where("u.email = ?", username).
			Find(&results).Error
	} else {
		// 角色名登录
		err = s.db.Table("players p1").
			Select("u.uid, u.email, u.password, u.permission, u.verified, p2.name as player_name, uuid.uuid").
			Joins("JOIN users u ON p1.uid = u.uid").
			Joins("LEFT JOIN players p2 ON u.uid = p2.uid").
			Joins("LEFT JOIN uuid ON p2.name = uuid.name").
			Where("p1.name = ?", username).
			Find(&results).Error
	}

	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	// 验证密码
	userInfo := results[0]
	if !s.verifyPassword(password, userInfo.Password) {
		return nil, fmt.Errorf("invalid password")
	}

	// 检查用户状态
	if userInfo.Permission == -1 { // BANNED = -1 in BlessingSkin
		return nil, fmt.Errorf("user is banned")
	}

	// 检查邮箱验证（如果启用）
	if !userInfo.Verified {
		// 这里可以根据配置决定是否要求邮箱验证
		// 暂时允许未验证用户登录
	}

	// 构建角色列表
	var profiles []yggdrasil.Profile
	var missingUUIDs []string

	for _, result := range results {
		if result.PlayerName != "" { // 有角色
			if result.UUID != "" {
				// UUID已存在
				profiles = append(profiles, yggdrasil.Profile{
					ID:         result.UUID,
					Name:       result.PlayerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			} else {
				// UUID缺失，需要创建
				missingUUIDs = append(missingUUIDs, result.PlayerName)
			}
		}
	}

	// 批量创建缺失的UUID
	if len(missingUUIDs) > 0 {
		uuidMap, err := s.uuidGen.GetUUIDsByNames(missingUUIDs)
		if err != nil {
			return nil, err
		}

		for _, playerName := range missingUUIDs {
			if uuid, exists := uuidMap[playerName]; exists {
				profiles = append(profiles, yggdrasil.Profile{
					ID:         uuid,
					Name:       playerName,
					Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
				})
			}
		}
	}

	return &yggdrasil.User{
		ID:       fmt.Sprintf("%d", userInfo.UID),
		Email:    userInfo.Email,
		Password: "", // 认证后不返回密码
		Profiles: profiles,
	}, nil
}

// verifyPassword 验证密码（BlessingSkin官方兼容密码验证）
func (s *Storage) verifyPassword(rawPassword, hashedPassword string) bool {
	// 根据BlessingSkin的PWD_METHOD配置进行验证
	switch strings.ToUpper(s.config.PwdMethod) {
	case "BCRYPT":
		return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(rawPassword)) == nil

	case "ARGON2I":
		// 验证Argon2i哈希
		return s.verifyArgon2i(rawPassword, hashedPassword)

	case "PHP_PASSWORD_HASH":
		// PHP的password_hash函数，通常是bcrypt
		return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(rawPassword)) == nil

	case "MD5":
		return hashedPassword == fmt.Sprintf("%x", md5.Sum([]byte(rawPassword)))

	case "SALTED2MD5":
		// BlessingSkin的SALTED2MD5: md5(md5(password) + salt)
		firstHash := fmt.Sprintf("%x", md5.Sum([]byte(rawPassword)))
		saltedHash := fmt.Sprintf("%x", md5.Sum([]byte(firstHash+s.config.Salt)))
		return hashedPassword == saltedHash

	case "SHA256":
		return hashedPassword == fmt.Sprintf("%x", sha256.Sum256([]byte(rawPassword)))

	case "SALTED2SHA256":
		// sha256(sha256(password) + salt)
		firstHash := fmt.Sprintf("%x", sha256.Sum256([]byte(rawPassword)))
		saltedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(firstHash+s.config.Salt)))
		return hashedPassword == saltedHash

	case "SHA512":
		hash := sha512.Sum512([]byte(rawPassword))
		return hashedPassword == fmt.Sprintf("%x", hash)

	case "SALTED2SHA512":
		// sha512(sha512(password) + salt)
		firstHash := sha512.Sum512([]byte(rawPassword))
		firstHashStr := fmt.Sprintf("%x", firstHash)
		saltedHash := sha512.Sum512([]byte(firstHashStr + s.config.Salt))
		return hashedPassword == fmt.Sprintf("%x", saltedHash)

	default:
		// 默认使用BCRYPT（BlessingSkin默认）
		return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(rawPassword)) == nil
	}
}

// VerifyPasswordTest 测试用的密码验证方法（导出）
func (s *Storage) VerifyPasswordTest(rawPassword, hashedPassword string) bool {
	return s.verifyPassword(rawPassword, hashedPassword)
}

// SetPwdMethod 设置密码加密方法（测试用）
func (s *Storage) SetPwdMethod(method string) {
	s.config.PwdMethod = method
}

// verifyArgon2i 验证Argon2i哈希
func (s *Storage) verifyArgon2i(password, hash string) bool {
	// 解析Argon2i哈希格式: $argon2i$v=19$m=1024,t=2,p=2$salt$hash
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2i" {
		return false
	}

	// 解析版本
	version := parts[2]
	if version != "v=19" {
		return false // 只支持版本19
	}

	// 解析参数 m=memory,t=time,p=threads
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false
	}

	var memory, time uint32
	var threads uint8
	for _, param := range params {
		kv := strings.Split(param, "=")
		if len(kv) != 2 {
			return false
		}

		val, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return false
		}

		switch kv[0] {
		case "m":
			memory = uint32(val)
		case "t":
			time = uint32(val)
		case "p":
			threads = uint8(val)
		}
	}

	// 解析盐值
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	// 解析期望的哈希值
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	// 计算Argon2i哈希
	computedHash := argon2.Key([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	// 使用constant time比较防止时序攻击
	return subtle.ConstantTimeCompare(expectedHash, computedHash) == 1
}
