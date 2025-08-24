// Package file 文件存储实现
package file

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"yggdrasil-api-go/src/config"
	storage "yggdrasil-api-go/src/storage/interface"
	"yggdrasil-api-go/src/yggdrasil"

	"github.com/bytedance/sonic"
)

// Storage 文件存储实现（仿照BlessingSkin表结构）
type Storage struct {
	dataDir       string                // 数据目录
	textureConfig *config.TextureConfig // 材质配置
	mu            sync.RWMutex          // 读写锁

	// 数据文件（仿照BlessingSkin表结构）
	users    map[string]*FileUser    // 用户数据 (users.json)
	players  map[string]*FilePlayer  // 角色数据 (players.json)
	textures map[string]*FileTexture // 材质数据 (textures.json)

	// 缓存映射
	userProfiles map[string][]string // 用户角色映射缓存
}

// FileUser 文件存储的用户结构（对应BlessingSkin的users表）
type FileUser struct {
	UID        int    `json:"uid"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Nickname   string `json:"nickname"`
	Score      int    `json:"score"`
	Permission int    `json:"permission"`
	Verified   bool   `json:"verified"`
	RegisterAt string `json:"register_at"`
	LastSignAt string `json:"last_sign_at"`
}

// FilePlayer 文件存储的角色结构（对应BlessingSkin的players表）
type FilePlayer struct {
	PID        int    `json:"pid"`
	UID        int    `json:"uid"`
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	SkinTID    int    `json:"tid_skin"`
	CapeTID    int    `json:"tid_cape"`
	LastModify string `json:"last_modified"`
}

// FileTexture 文件存储的材质结构（对应BlessingSkin的textures表）
type FileTexture struct {
	TID      int    `json:"tid"`
	Name     string `json:"name"`
	Type     string `json:"type"` // "steve", "alex", "cape"
	Hash     string `json:"hash"`
	Size     int    `json:"size"`
	Uploader int    `json:"uploader"`
	Public   bool   `json:"public"`
	UploadAt string `json:"upload_at"`
}

// convertFileUserToYggdrasilUser 将FileUser转换为yggdrasil.User
func (s *Storage) convertFileUserToYggdrasilUser(fileUser *FileUser) (*yggdrasil.User, error) {
	// 获取用户的角色
	var profiles []yggdrasil.Profile
	for _, player := range s.players {
		if player.UID == fileUser.UID {
			profiles = append(profiles, yggdrasil.Profile{
				ID:   player.UUID,
				Name: player.Name,
			})
		}
	}

	return &yggdrasil.User{
		ID:       fmt.Sprintf("%d", fileUser.UID),
		Email:    fileUser.Email,
		Password: fileUser.Password,
		Profiles: profiles,
	}, nil
}

// convertYggdrasilUserToFileUser 将yggdrasil.User转换为FileUser
func (s *Storage) convertYggdrasilUserToFileUser(user *yggdrasil.User) (*FileUser, error) {
	uid := 1
	if user.ID != "" {
		if parsedUID, err := strconv.Atoi(user.ID); err == nil {
			uid = parsedUID
		}
	}

	return &FileUser{
		UID:        uid,
		Email:      user.Email,
		Password:   user.Password,
		Nickname:   user.Email, // 默认使用邮箱作为昵称
		Score:      1000,       // 默认积分
		Permission: 0,          // 默认权限
		Verified:   true,       // 默认已验证
		RegisterAt: time.Now().Format("2006-01-02 15:04:05"),
		LastSignAt: time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}

// NewStorage 创建新的文件存储实例
func NewStorage(options map[string]any, textureConfig *config.TextureConfig) (*Storage, error) {
	dataDir := "data"
	if dir, ok := options["data_dir"].(string); ok && dir != "" {
		dataDir = dir
	}

	storage := &Storage{
		dataDir:       dataDir,
		textureConfig: textureConfig,
		users:         make(map[string]*FileUser),
		players:       make(map[string]*FilePlayer),
		textures:      make(map[string]*FileTexture),
		userProfiles:  make(map[string][]string),
	}

	// 创建必要的目录
	if err := storage.initDirectories(); err != nil {
		return nil, fmt.Errorf("failed to initialize directories: %w", err)
	}

	// 加载数据到缓存
	if err := storage.loadData(); err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}

	return storage, nil
}

// initDirectories 初始化目录结构
func (s *Storage) initDirectories() error {
	dirs := []string{
		s.dataDir,
		filepath.Join(s.dataDir, "textures", "skins"),
		filepath.Join(s.dataDir, "textures", "capes"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// loadData 加载数据到缓存
func (s *Storage) loadData() error {
	// 加载用户数据
	if err := s.loadUsers(); err != nil {
		return err
	}

	// 加载角色数据
	if err := s.loadPlayers(); err != nil {
		return err
	}

	// 加载材质数据
	if err := s.loadTexturesData(); err != nil {
		return err
	}

	return nil
}

// loadTexturesData 加载材质数据
func (s *Storage) loadTexturesData() error {
	texturesFile := filepath.Join(s.dataDir, "textures.json")

	// 如果文件不存在，创建空的材质数据
	if _, err := os.Stat(texturesFile); os.IsNotExist(err) {
		return s.createDefaultTexturesData()
	}

	data, err := os.ReadFile(texturesFile)
	if err != nil {
		return err
	}

	var textures []*FileTexture
	if err := sonic.Unmarshal(data, &textures); err != nil {
		return err
	}

	// 加载到缓存
	for _, texture := range textures {
		s.textures[texture.Hash] = texture
	}

	return nil
}

// createDefaultTexturesData 创建默认材质数据
func (s *Storage) createDefaultTexturesData() error {
	// 创建空的材质数据
	return s.saveTexturesData()
}

// saveTexturesData 保存材质数据
func (s *Storage) saveTexturesData() error {
	var textures []*FileTexture
	for _, texture := range s.textures {
		textures = append(textures, texture)
	}

	data, err := sonic.MarshalIndent(textures, "", "  ")
	if err != nil {
		return err
	}

	texturesFile := filepath.Join(s.dataDir, "textures.json")
	return os.WriteFile(texturesFile, data, 0644)
}

// getHashPath 获取哈希分桶路径
func (s *Storage) getHashPath(baseDir, key, extension string) string {
	hash := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	// 两级分桶：前2个字符/后2个字符/完整哈希.扩展名
	level1 := hashStr[:2]
	level2 := hashStr[2:4]
	filename := hashStr + extension

	return filepath.Join(s.dataDir, baseDir, level1, level2, filename)
}

// ensureDir 确保目录存在
func (s *Storage) ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
}

// Close 关闭存储连接
func (s *Storage) Close() error {
	// 文件存储无需特殊关闭操作
	return nil
}

// Ping 检查存储连接
func (s *Storage) Ping() error {
	// 检查数据目录是否可访问
	_, err := os.Stat(s.dataDir)
	return err
}

// GetStorageType 获取存储类型
func (s *Storage) GetStorageType() string {
	return "file"
}

// GetUserByUUID 根据UUID获取用户
func (s *Storage) GetUserByUUID(uuid string) (*yggdrasil.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 先通过UUID找到对应的角色
	var targetPlayer *FilePlayer
	for _, player := range s.players {
		if player.UUID == uuid {
			targetPlayer = player
			break
		}
	}

	if targetPlayer == nil {
		return nil, fmt.Errorf("player not found")
	}

	// 通过角色的UID找到对应的用户
	for _, user := range s.users {
		if user.UID == targetPlayer.UID {
			return s.convertFileUserToYggdrasilUser(user)
		}
	}

	return nil, fmt.Errorf("user not found")
}

// AuthenticateUser 用户认证
func (s *Storage) AuthenticateUser(username, password string) (*yggdrasil.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.Email == username && user.Password == password {
			return s.convertFileUserToYggdrasilUser(user)
		}
	}
	return nil, fmt.Errorf("authentication failed")
}

// GetUserProfiles 根据用户UUID获取角色
func (s *Storage) GetUserProfiles(userUUID string) ([]*yggdrasil.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 先通过UUID找到对应的角色，获取UID
	var targetUID int
	for _, player := range s.players {
		if player.UUID == userUUID {
			targetUID = player.UID
			break
		}
	}

	if targetUID == 0 {
		return nil, fmt.Errorf("player not found")
	}

	// 获取该用户的所有角色
	var profiles []*yggdrasil.Profile
	for _, player := range s.players {
		if player.UID == targetUID {
			profiles = append(profiles, &yggdrasil.Profile{
				ID:         player.UUID,
				Name:       player.Name,
				Properties: []yggdrasil.ProfileProperty{}, // 初始化为空数组而不是nil
			})
		}
	}

	return profiles, nil
}

// GetPlayerTextures 获取角色的所有材质
func (s *Storage) GetPlayerTextures(playerUUID string) (map[storage.TextureType]*storage.TextureInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	textures := make(map[storage.TextureType]*storage.TextureInfo)

	// 查找角色
	player, exists := s.players[playerUUID]
	if !exists {
		return textures, nil // 角色不存在，返回空材质
	}

	// 获取皮肤材质
	if player.SkinTID > 0 {
		for _, texture := range s.textures {
			if texture.TID == player.SkinTID {
				textures[storage.TextureTypeSkin] = &storage.TextureInfo{
					Type: storage.TextureTypeSkin,
					URL:  s.textureConfig.BaseURL + "skin/" + texture.Hash + ".png",
					Metadata: &storage.TextureMetadata{
						Hash:       texture.Hash,
						FileSize:   int64(texture.Size),
						UploadedAt: parseTime(texture.UploadAt),
					},
				}
				break
			}
		}
	}

	// 获取披风材质
	if player.CapeTID > 0 {
		for _, texture := range s.textures {
			if texture.TID == player.CapeTID {
				textures[storage.TextureTypeCape] = &storage.TextureInfo{
					Type: storage.TextureTypeCape,
					URL:  s.textureConfig.BaseURL + "cape/" + texture.Hash + ".png",
					Metadata: &storage.TextureMetadata{
						Hash:       texture.Hash,
						FileSize:   int64(texture.Size),
						UploadedAt: parseTime(texture.UploadAt),
					},
				}
				break
			}
		}
	}

	return textures, nil
}

// parseTime 解析时间字符串
func parseTime(timeStr string) time.Time {
	if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
		return t
	}
	return time.Now()
}

// GetSignatureKeyPair 获取签名用的密钥对（文件存储不支持密钥管理）
func (s *Storage) GetSignatureKeyPair() (privateKey string, publicKey string, err error) {
	return "", "", fmt.Errorf("signature key pair not available in file storage, use config file")
}
