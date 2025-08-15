// Package blessing_skin UUID生成和管理
package blessing_skin

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UUIDGenerator UUID生成器
type UUIDGenerator struct {
	storage *Storage
	cache   *UUIDCache
}

// NewUUIDGenerator 创建UUID生成器
func NewUUIDGenerator(storage *Storage) *UUIDGenerator {
	// 从配置中获取缓存大小，默认1000
	cacheSize := 1000

	return &UUIDGenerator{
		storage: storage,
		cache:   NewUUIDCache(cacheSize),
	}
}

// GenerateUUID 根据配置生成UUID
func (g *UUIDGenerator) GenerateUUID(playerName string) (string, error) {
	algorithm, err := g.storage.optionsMgr.GetOption("ygg_uuid_algorithm")
	if err != nil {
		algorithm = "v3" // 默认使用v3算法
	}

	switch algorithm {
	case "v3":
		return g.generateUUIDV3(playerName), nil
	case "v4":
		return g.generateUUIDV4(), nil
	default:
		return g.generateUUIDV3(playerName), nil
	}
}

// generateUUIDV3 生成v3 UUID（离线模式兼容）
func (g *UUIDGenerator) generateUUIDV3(name string) string {
	// 实现与PHP版本完全相同的算法
	// @see https://gist.github.com/games647/2b6a00a8fc21fd3b88375f03c9e2e603
	data := md5.Sum([]byte("OfflinePlayer:" + name))
	data[6] = (data[6] & 0x0F) | 0x30 // 设置版本号为3
	data[8] = (data[8] & 0x3F) | 0x80 // 设置变体
	return hex.EncodeToString(data[:])
}

// generateUUIDV4 生成v4 UUID（随机）
func (g *UUIDGenerator) generateUUIDV4() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// GetOrCreateUUID 获取或创建UUID映射（带缓存）
func (g *UUIDGenerator) GetOrCreateUUID(playerName string) (string, error) {
	// 先从缓存查找
	if uuid, found := g.cache.GetUUIDByName(playerName); found {
		return uuid, nil
	}

	// 缓存未命中，查询数据库
	var mapping UUIDMapping
	err := g.storage.db.Where("name = ?", playerName).First(&mapping).Error
	if err == nil {
		// 找到映射，添加到缓存
		g.cache.PutMapping(mapping.Name, mapping.UUID)
		return mapping.UUID, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	// 生成新UUID
	newUUID, err := g.GenerateUUID(playerName)
	if err != nil {
		return "", err
	}

	// 保存映射到数据库
	mapping = UUIDMapping{
		Name: playerName,
		UUID: newUUID,
	}

	if err := g.storage.db.Create(&mapping).Error; err != nil {
		return "", err
	}

	// 添加到缓存
	g.cache.PutMapping(playerName, newUUID)

	return newUUID, nil
}

// GetUUIDByName 根据角色名获取UUID（带缓存）
func (g *UUIDGenerator) GetUUIDByName(playerName string) (string, error) {
	// 先从缓存查找
	if uuid, found := g.cache.GetUUIDByName(playerName); found {
		return uuid, nil
	}

	// 缓存未命中，查询数据库
	var mapping UUIDMapping
	err := g.storage.db.Where("name = ?", playerName).First(&mapping).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("UUID not found for player: %s", playerName)
		}
		return "", err
	}

	// 添加到缓存
	g.cache.PutMapping(mapping.Name, mapping.UUID)
	return mapping.UUID, nil
}

// GetNameByUUID 根据UUID获取角色名（带缓存）
func (g *UUIDGenerator) GetNameByUUID(uuid string) (string, error) {
	// 先从缓存查找
	if name, found := g.cache.GetNameByUUID(uuid); found {
		return name, nil
	}

	// 缓存未命中，查询数据库
	var mapping UUIDMapping
	err := g.storage.db.Where("uuid = ?", uuid).First(&mapping).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("player not found for UUID: %s", uuid)
		}
		return "", err
	}

	// 添加到缓存
	g.cache.PutMapping(mapping.Name, mapping.UUID)
	return mapping.Name, nil
}

// UpdateUUIDMapping 更新UUID映射（仅在角色改名时使用）
func (g *UUIDGenerator) UpdateUUIDMapping(oldName, newName string) error {
	var mapping UUIDMapping
	err := g.storage.db.Where("name = ?", oldName).First(&mapping).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("UUID mapping not found for player: %s", oldName)
		}
		return err
	}

	// 检查新名称是否已被使用
	var existingMapping UUIDMapping
	err = g.storage.db.Where("name = ?", newName).First(&existingMapping).Error
	if err == nil {
		return fmt.Errorf("player name already exists: %s", newName)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 更新映射
	mapping.Name = newName
	return g.storage.db.Save(&mapping).Error
}

// GetUUIDsByNames 批量获取UUID映射（带缓存，自动创建缺失的UUID）
func (g *UUIDGenerator) GetUUIDsByNames(names []string) (map[string]string, error) {
	if len(names) == 0 {
		return make(map[string]string), nil
	}

	result := make(map[string]string)
	var missingNames []string

	// 先从缓存中查找
	for _, name := range names {
		if uuid, found := g.cache.GetUUIDByName(name); found {
			result[name] = uuid
		} else {
			missingNames = append(missingNames, name)
		}
	}

	// 如果所有都在缓存中找到了，直接返回
	if len(missingNames) == 0 {
		return result, nil
	}

	// 批量查询数据库中缺失的映射
	var mappings []UUIDMapping
	err := g.storage.db.Where("name IN ?", missingNames).Find(&mappings).Error
	if err != nil {
		return nil, err
	}

	// 将查询结果添加到结果和缓存中
	foundNames := make(map[string]bool)
	for _, mapping := range mappings {
		result[mapping.Name] = mapping.UUID
		g.cache.PutMapping(mapping.Name, mapping.UUID)
		foundNames[mapping.Name] = true
	}

	// 找出仍然缺失的UUID（需要创建）
	var needCreateNames []string
	for _, name := range missingNames {
		if !foundNames[name] {
			needCreateNames = append(needCreateNames, name)
		}
	}

	// 批量创建缺失的UUID映射
	if len(needCreateNames) > 0 {
		var newMappings []UUIDMapping
		for _, name := range needCreateNames {
			newUUID, err := g.GenerateUUID(name)
			if err != nil {
				continue // 跳过生成失败的UUID
			}

			newMappings = append(newMappings, UUIDMapping{
				Name: name,
				UUID: newUUID,
			})

			// 添加到结果和缓存
			result[name] = newUUID
			g.cache.PutMapping(name, newUUID)
		}

		// 批量插入到数据库
		if len(newMappings) > 0 {
			err = g.storage.db.Create(&newMappings).Error
			if err != nil {
				// 记录错误但不影响返回结果
				fmt.Printf("⚠️  Failed to batch create UUID mappings: %v\n", err)
			}
		}
	}

	return result, nil
}

// GetCacheStats 获取缓存统计信息
func (g *UUIDGenerator) GetCacheStats() map[string]any {
	return g.cache.GetStats()
}

// ClearCache 清空缓存
func (g *UUIDGenerator) ClearCache() {
	g.cache.Clear()
}
